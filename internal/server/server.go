// Package server implements the HTTP allowlist server and periodic refresh logic.
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"git.stellar.study/stellar-study/iplist/internal/metrics"
)

// Source fetches IPv4 CIDR prefixes for a single allowlist.
type Source interface {
	// Name identifies the allowlist (e.g. "github", "azure").
	Name() string
	// Fetch returns IPv4 CIDR prefixes. Errors leave the previous cache intact.
	Fetch(ctx context.Context) ([]string, error)
}

// cached holds the last good fetch result for a source.
type cached struct {
	mu          sync.RWMutex
	prefixes    []string
	lastSuccess time.Time
	lastError   error
}

func (c *cached) set(prefixes []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prefixes = prefixes
	c.lastSuccess = time.Now()
	c.lastError = nil
}

func (c *cached) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err
}

func (c *cached) get() ([]string, time.Time, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.prefixes, c.lastSuccess, c.lastError
}

// Server serves allowlists over HTTP and refreshes them periodically.
type Server struct {
	sources    map[string]Source
	caches     map[string]*cached
	aliases    map[string]string
	addr       string
	interval   time.Duration
	httpServer *http.Server
}

// New creates a Server.
func New(addr string, interval time.Duration, sources []Source) *Server {
	s := &Server{
		sources:  make(map[string]Source, len(sources)),
		caches:   make(map[string]*cached, len(sources)),
		aliases:  make(map[string]string),
		addr:     addr,
		interval: interval,
	}

	for _, src := range sources {
		name := src.Name()
		s.sources[name] = src
		s.caches[name] = &cached{}
	}

	// Path aliases. Both /azure/teams and /azure/microsoftteams serve the Azure Bot Service allowlist.
	s.aliases["/github"] = "github"
	s.aliases["/azure/teams"] = "azure"
	s.aliases["/azure/microsoftteams"] = "azure"

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", metrics.Handler)
	mux.HandleFunc("/", s.handleIndex)
	for path, name := range s.aliases {
		log.Printf("registered endpoint: %s -> %s", path, name)
		mux.HandleFunc(path, s.handleAllowlist(name))
	}

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	return s
}

// Run starts the periodic refresh loop and the HTTP server. It blocks until the server stops.
func (s *Server) Run(ctx context.Context) error {
	// Seed caches at startup.
	s.refreshAll(ctx)

	// Periodic refresh.
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshAll(ctx)
			}
		}
	}()

	// Keep staleness metrics updated between refreshes.
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.updateStaleness()
			}
		}
	}()

	log.Printf("iplist server listening on %s", s.addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) refreshAll(ctx context.Context) {
	var wg sync.WaitGroup
	for name, src := range s.sources {
		wg.Add(1)
		go func(name string, src Source) {
			defer wg.Done()
			s.refreshOne(ctx, name, src)
		}(name, src)
	}
	wg.Wait()
}

func (s *Server) refreshOne(ctx context.Context, name string, src Source) {
	prefixes, err := src.Fetch(ctx)
	if err != nil {
		log.Printf("fetch failed for %s: %v", name, err)
		s.caches[name].setError(err)
		metrics.FetchTotal.WithLabelValues(name, "failure").Inc()
		return
	}

	s.caches[name].set(prefixes)
	metrics.FetchTotal.WithLabelValues(name, "success").Inc()
	metrics.PrefixesTotal.WithLabelValues(name).Set(float64(len(prefixes)))
	metrics.LastSuccessTimestamp.WithLabelValues(name).Set(float64(time.Now().Unix()))
	metrics.StalenessSeconds.WithLabelValues(name).Set(0)
	log.Printf("refreshed %s: %d prefixes", name, len(prefixes))
}

func (s *Server) updateStaleness() {
	for name := range s.sources {
		_, lastSuccess, _ := s.caches[name].get()
		if !lastSuccess.IsZero() {
			metrics.StalenessSeconds.WithLabelValues(name).Set(time.Since(lastSuccess).Seconds())
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var b strings.Builder
	b.WriteString("iplist allowlist server\n\nendpoints:\n")
	paths := make([]string, 0, len(s.aliases))
	for p := range s.aliases {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "  GET %s\n", p)
	}
	b.WriteString("  GET /metrics\n")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleAllowlist(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		prefixes, lastSuccess, lastErr := s.caches[name].get()
		if len(prefixes) == 0 && lastSuccess.IsZero() {
			msg := "allowlist not yet available"
			if lastErr != nil {
				msg = fmt.Sprintf("allowlist unavailable: %v", lastErr)
			}
			http.Error(w, msg, http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		for _, p := range prefixes {
			_, _ = fmt.Fprintln(w, p)
		}
	}
}
