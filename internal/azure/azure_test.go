package azure

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchReturnsAllTags(t *testing.T) {
	jsonBody := `{
		"changeNumber": 1,
		"cloud": "Public",
		"values": [
			{
				"name": "AzureBotService",
				"id": "AzureBotService",
				"properties": {
					"changeNumber": 1,
					"region": "",
					"regionId": 0,
					"platform": "Azure",
					"systemService": "AzureBotService",
					"addressPrefixes": ["13.107.42.0/24", "2606:50c0::/32", "40.126.0.0/18"]
				}
			},
			{
				"name": "Storage",
				"properties": {
					"addressPrefixes": ["10.0.0.0/16"]
				}
			},
			{
				"name": "IPv6Only",
				"properties": {
					"addressPrefixes": ["2606:50c0::/32"]
				}
			}
		]
	}`

	mux := http.NewServeMux()
	mux.HandleFunc("/details.aspx", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		url := "http://" + r.Host + "/data.json"
		_, _ = w.Write([]byte(`window.__DLCDetails__={"downloadFile":[{"url":"` + url + `"}]}`))
	})
	mux.HandleFunc("/data.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jsonBody))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	orig := detailsURL
	defer func() { detailsURL = orig }()
	detailsURL = ts.URL + "/details.aspx?id=56519"

	src := NewSource(5 * time.Second)
	result, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("got %d tags, want 2: %v", len(result), result)
	}

	wantAzureBotService := []string{"13.107.42.0/24", "40.126.0.0/18"}
	if got, ok := result["AzureBotService"]; !ok {
		t.Fatalf("missing AzureBotService tag")
	} else {
		if len(got) != len(wantAzureBotService) {
			t.Fatalf("AzureBotService: got %d prefixes, want %d: %v", len(got), len(wantAzureBotService), got)
		}
		for i, p := range wantAzureBotService {
			if got[i] != p {
				t.Errorf("AzureBotService prefix %d: got %s, want %s", i, got[i], p)
			}
		}
	}

	wantStorage := []string{"10.0.0.0/16"}
	if got, ok := result["Storage"]; !ok {
		t.Fatalf("missing Storage tag")
	} else {
		if len(got) != len(wantStorage) {
			t.Fatalf("Storage: got %d prefixes, want %d: %v", len(got), len(wantStorage), got)
		}
		for i, p := range wantStorage {
			if got[i] != p {
				t.Errorf("Storage prefix %d: got %s, want %s", i, got[i], p)
			}
		}
	}

	if _, ok := result["IPv6Only"]; ok {
		t.Fatalf("IPv6Only should have been skipped")
	}
}

func TestFetchResolveFails(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("no json link here"))
	}))
	defer ts.Close()

	orig := detailsURL
	defer func() { detailsURL = orig }()
	detailsURL = ts.URL

	src := NewSource(5 * time.Second)
	_, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error when download link is missing")
	}
}
