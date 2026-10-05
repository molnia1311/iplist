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

// Source fetches IPv4 CIDR prefixes grouped by list name.
type Source interface {
	// Name identifies the source (e.g. "github", "azure").
	Name() string
	// Fetch returns IPv4 CIDR prefixes keyed by list name.
	// Errors leave the previous cache intact.
	Fetch(ctx context.Context) (map[string][]string, error)
}

// cached holds the last good fetch result for a list.
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
	addr       string
	interval   time.Duration
	httpServer *http.Server
}

// New creates a Server.
func New(addr string, interval time.Duration, sources []Source) *Server {
	s := &Server{
		sources:  make(map[string]Source, len(sources)),
		caches:   make(map[string]*cached),
		addr:     addr,
		interval: interval,
	}

	for _, src := range sources {
		name := src.Name()
		s.sources[name] = src
		// GitHub exposes a single known list; pre-create its cache so the
		// endpoint returns 503 before the first fetch instead of 404.
		if name == "github" {
			s.caches["github:github"] = &cached{}
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", metrics.Handler)
	mux.HandleFunc("/github", s.handleAllowlist("github:github"))
	mux.HandleFunc("/azure/", s.handleAzure)
	mux.HandleFunc("/", s.handleIndex)

	log.Printf("registered endpoint: GET /github -> github:github")
	log.Printf("registered endpoint: GET /azure/{tag}")

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
	result, err := src.Fetch(ctx)
	if err != nil {
		log.Printf("fetch failed for %s: %v", name, err)
		// Record the error against every list already cached for this source.
		for key := range s.caches {
			if strings.HasPrefix(key, name+":") {
				s.caches[key].setError(err)
				metrics.FetchTotal.WithLabelValues(key, "failure").Inc()
			}
		}
		return
	}

	for listName, prefixes := range result {
		key := name + ":" + listName
		if _, ok := s.caches[key]; !ok {
			s.caches[key] = &cached{}
		}
		s.caches[key].set(prefixes)
		metrics.FetchTotal.WithLabelValues(key, "success").Inc()
		metrics.PrefixesTotal.WithLabelValues(key).Set(float64(len(prefixes)))
		metrics.LastSuccessTimestamp.WithLabelValues(key).Set(float64(time.Now().Unix()))
		metrics.StalenessSeconds.WithLabelValues(key).Set(0)
		log.Printf("refreshed %s: %d prefixes", key, len(prefixes))
	}
}

func (s *Server) updateStaleness() {
	for key := range s.caches {
		_, lastSuccess, _ := s.caches[key].get()
		if !lastSuccess.IsZero() {
			metrics.StalenessSeconds.WithLabelValues(key).Set(time.Since(lastSuccess).Seconds())
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
	paths := make([]string, 0, len(s.caches)+2)
	paths = append(paths, "/github")
	for key := range s.caches {
		if strings.HasPrefix(key, "azure:") {
			paths = append(paths, "/azure/"+strings.TrimPrefix(key, "azure:"))
		}
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

func (s *Server) handleAzure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tag := strings.TrimPrefix(r.URL.Path, "/azure/")
	if tag == "" || strings.Contains(tag, "/") {
		http.NotFound(w, r)
		return
	}

	s.handleAllowlist("azure:"+tag).ServeHTTP(w, r)
}

func (s *Server) handleAllowlist(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		c, ok := s.caches[key]
		if !ok {
			http.NotFound(w, r)
			return
		}

		prefixes, lastSuccess, lastErr := c.get()
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
