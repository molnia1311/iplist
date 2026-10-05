package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	name     string
	prefixes []string
	err      error
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Fetch(ctx context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.prefixes, nil
}

func TestHandleAllowlist(t *testing.T) {
	src := &fakeSource{name: "github", prefixes: []string{"1.2.3.4/32", "5.6.7.0/24"}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	// Seed cache manually.
	srv.caches["github"].set([]string{"1.2.3.4/32", "5.6.7.0/24"})

	req := httptest.NewRequest(http.MethodGet, "/github", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(rec.Body)
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), string(body))
	}
	if lines[0] != "1.2.3.4/32" || lines[1] != "5.6.7.0/24" {
		t.Errorf("unexpected body: %q", string(body))
	}
}

func TestHandleAllowlistNotReady(t *testing.T) {
	src := &fakeSource{name: "github", prefixes: []string{"1.2.3.4/32"}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	req := httptest.NewRequest(http.MethodGet, "/github", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleIndex(t *testing.T) {
	src := &fakeSource{name: "github", prefixes: []string{"1.2.3.4/32"}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusOK)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "/github") {
		t.Errorf("index did not list /github: %q", string(body))
	}
}

func TestAlias(t *testing.T) {
	src := &fakeSource{name: "azure", prefixes: []string{"13.107.42.0/24"}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})
	srv.caches["azure"].set([]string{"13.107.42.0/24"})

	for _, path := range []string{"/azure/teams", "/azure/microsoftteams"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want %d", path, rec.Code, http.StatusOK)
		}
		body, _ := io.ReadAll(rec.Body)
		if !strings.Contains(string(body), "13.107.42.0/24") {
			t.Errorf("%s: unexpected body: %q", path, string(body))
		}
	}
}
