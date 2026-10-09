package m365

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchGroupsAndFilters(t *testing.T) {
	versionBody := `{"instance":"Worldwide","latest":"2023100101"}`
	endpointsBody := `[
		{
			"id": 1,
			"serviceArea": "Skype",
			"serviceAreaDisplayName": "Skype and Teams",
			"ips": ["52.112.0.0/14", "2001:db8::/32", "52.122.0.0/15"]
		},
		{
			"id": 2,
			"serviceArea": "Skype",
			"serviceAreaDisplayName": "Skype and Teams",
			"ips": ["52.122.0.0/15", "13.107.64.0/18"]
		},
		{
			"id": 3,
			"serviceArea": "Common",
			"serviceAreaDisplayName": "Common",
			"ips": ["23.103.160.0/20"]
		},
		{
			"id": 4,
			"serviceArea": "Exchange",
			"serviceAreaDisplayName": "Exchange Online",
			"urls": ["outlook.office365.com"]
		}
	]`

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versionBody))
	})
	mux.HandleFunc("/endpoints", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(endpointsBody))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	origVersion := versionURL
	origEndpoints := endpointsURL
	defer func() {
		versionURL = origVersion
		endpointsURL = origEndpoints
	}()
	versionURL = ts.URL + "/version"
	endpointsURL = ts.URL + "/endpoints"

	src := NewSource(5 * time.Second)
	result, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d areas, want 2: %v", len(result), result)
	}

	skype, ok := result["skype"]
	if !ok {
		t.Fatalf("missing skype area")
	}
	wantSkype := []string{"13.107.64.0/18", "52.112.0.0/14", "52.122.0.0/15"}
	if len(skype) != len(wantSkype) {
		t.Fatalf("skype: got %d prefixes, want %d: %v", len(skype), len(wantSkype), skype)
	}
	for i, p := range wantSkype {
		if skype[i] != p {
			t.Errorf("skype prefix %d: got %s, want %s", i, skype[i], p)
		}
	}

	common, ok := result["common"]
	if !ok {
		t.Fatalf("missing common area")
	}
	wantCommon := []string{"23.103.160.0/20"}
	if len(common) != len(wantCommon) {
		t.Fatalf("common: got %d prefixes, want %d: %v", len(common), len(wantCommon), common)
	}

	if _, ok := result["exchange"]; ok {
		t.Fatalf("exchange should have been skipped (URL-only)")
	}
}

func TestFetchUnchangedVersionSkipsEndpoints(t *testing.T) {
	versionBody := `{"instance":"Worldwide","latest":"2023100101"}`
	endpointsBody := `[{"id":1,"serviceArea":"Skype","ips":["52.112.0.0/14"]}]`

	callCount := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versionBody))
	})
	mux.HandleFunc("/endpoints", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(endpointsBody))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	origVersion := versionURL
	origEndpoints := endpointsURL
	defer func() {
		versionURL = origVersion
		endpointsURL = origEndpoints
	}()
	versionURL = ts.URL + "/version"
	endpointsURL = ts.URL + "/endpoints"

	src := NewSource(5 * time.Second)
	if _, err := src.Fetch(context.Background()); err != nil {
		t.Fatalf("first Fetch failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("first fetch: endpoints called %d times, want 1", callCount)
	}

	result, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("second Fetch failed: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("second fetch: endpoints called %d times, want 1", callCount)
	}
	if got, ok := result["skype"]; !ok || len(got) != 1 || got[0] != "52.112.0.0/14" {
		t.Fatalf("unexpected result on unchanged version: %v", result)
	}
}

func TestFetchErrorRetainsPreviousResult(t *testing.T) {
	versionBody := `{"instance":"Worldwide","latest":"2023100101"}`
	errorVersionBody := `{"instance":"Worldwide","latest":"2023100102"}`
	endpointsBody := `[{"id":1,"serviceArea":"Skype","ips":["52.112.0.0/14"]}]`

	currentVersion := versionBody
	endpointsStatus := http.StatusOK
	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(currentVersion))
	})
	mux.HandleFunc("/endpoints", func(w http.ResponseWriter, r *http.Request) {
		if endpointsStatus != http.StatusOK {
			http.Error(w, "too many requests", endpointsStatus)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(endpointsBody))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	origVersion := versionURL
	origEndpoints := endpointsURL
	defer func() {
		versionURL = origVersion
		endpointsURL = origEndpoints
	}()
	versionURL = ts.URL + "/version"
	endpointsURL = ts.URL + "/endpoints"

	src := NewSource(5 * time.Second)
	result, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("first Fetch failed: %v", err)
	}

	// Force Fetch to call /endpoints and surface a 429 error.
	currentVersion = errorVersionBody
	endpointsStatus = http.StatusTooManyRequests

	_, err = src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for 429 response")
	}

	// Restore the working endpoint and original version; the previous result
	// should still be available via the short-circuit.
	currentVersion = versionBody
	endpointsStatus = http.StatusOK
	got, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch after error failed: %v", err)
	}
	if len(got) != len(result) {
		t.Fatalf("result was not retained after error: got %v, want %v", got, result)
	}
}
