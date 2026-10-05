package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchFiltersIPv4(t *testing.T) {
	jsonBody := `{
		"hooks": ["192.30.252.0/22", "185.199.108.0/22"],
		"web": ["140.82.112.0/20", "2001:db8::/32"],
		"api": ["1.2.3.4/32"],
		"git": [],
		"packages": ["13.107.42.0/24"],
		"actions": ["4.148.0.0/16", "2606:50c0::/32"],
		"dependabot": ["3.217.79.0/24"]
	}`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	}))
	defer ts.Close()

	// Temporarily point the API at the test server.
	orig := apiURL
	defer func() { apiURL = orig }()
	apiURL = ts.URL

	src := NewSource(5 * time.Second)
	prefixes, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	want := []string{
		"1.2.3.4/32",
		"13.107.42.0/24",
		"140.82.112.0/20",
		"185.199.108.0/22",
		"192.30.252.0/22",
		"3.217.79.0/24",
		"4.148.0.0/16",
	}

	if len(prefixes) != len(want) {
		t.Fatalf("got %d prefixes, want %d: %v", len(prefixes), len(want), prefixes)
	}
	for i, p := range want {
		if prefixes[i] != p {
			t.Errorf("prefix %d: got %s, want %s", i, prefixes[i], p)
		}
	}
}

func TestFetchHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer ts.Close()

	orig := apiURL
	defer func() { apiURL = orig }()
	apiURL = ts.URL

	src := NewSource(5 * time.Second)
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}
