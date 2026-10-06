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
	name   string
	result map[string][]string
	err    error
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Fetch(ctx context.Context) (map[string][]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestHandleAllowlist(t *testing.T) {
	src := &fakeSource{name: "github", result: map[string][]string{"github": {"1.2.3.4/32", "5.6.7.0/24"}}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	// Seed cache manually.
	srv.caches["github:github"].set([]string{"1.2.3.4/32", "5.6.7.0/24"})

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
	src := &fakeSource{name: "github", result: map[string][]string{"github": {"1.2.3.4/32"}}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	req := httptest.NewRequest(http.MethodGet, "/github", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleIndex(t *testing.T) {
	src := &fakeSource{name: "github", result: map[string][]string{"github": {"1.2.3.4/32"}}}
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
	if !strings.Contains(string(body), "/metrics") {
		t.Errorf("index did not list /metrics: %q", string(body))
	}
}

func TestAzureTag(t *testing.T) {
	src := &fakeSource{name: "azure", result: map[string][]string{
		"AzureBotService": {"13.107.42.0/24"},
		"MicrosoftTeams":  {"52.113.74.0/24"},
	}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})
	srv.caches["azure:azurebotservice"] = &cached{}
	srv.caches["azure:azurebotservice"].set([]string{"13.107.42.0/24"})
	srv.caches["azure:microsoftteams"] = &cached{}
	srv.caches["azure:microsoftteams"].set([]string{"52.113.74.0/24"})

	var wantBody string
	for _, path := range []string{"/azure/AzureBotService", "/azure/azurebotservice", "/azure/azureBotService"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		srv.httpServer.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want %d", path, rec.Code, http.StatusOK)
		}
		body, _ := io.ReadAll(rec.Body)
		if wantBody == "" {
			wantBody = string(body)
		} else if string(body) != wantBody {
			t.Errorf("%s: body mismatch\nwant: %q\ngot:  %q", path, wantBody, string(body))
		}
	}

	// Index should list the cached Azure tags.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "/azure/azurebotservice") {
		t.Errorf("index did not list /azure/azurebotservice: %q", string(body))
	}
	if !strings.Contains(string(body), "/azure/microsoftteams") {
		t.Errorf("index did not list /azure/microsoftteams: %q", string(body))
	}
}

func TestAzureUnknownTag(t *testing.T) {
	src := &fakeSource{name: "azure", result: map[string][]string{"AzureBotService": {"13.107.42.0/24"}}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	req := httptest.NewRequest(http.MethodGet, "/azure/NoSuchTag", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAzureTagInvalidPath(t *testing.T) {
	src := &fakeSource{name: "azure", result: map[string][]string{"AzureBotService": {"13.107.42.0/24"}}}
	srv := New("127.0.0.1:0", time.Hour, []Source{src})

	req := httptest.NewRequest(http.MethodGet, "/azure/AzureBotService/extra", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusNotFound)
	}
}
