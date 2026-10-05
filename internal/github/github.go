// Package github fetches IPv4 prefixes from the GitHub meta API.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"time"
)

var (
	apiURL = "https://api.github.com/meta"
	keys   = []string{"actions", "dependabot", "git", "hooks", "packages", "web", "api"}
)

// Source fetches GitHub meta IPv4 prefixes.
type Source struct {
	client *http.Client
}

// NewSource returns a new GitHub allowlist source.
func NewSource(timeout time.Duration) *Source {
	return &Source{
		client: &http.Client{Timeout: timeout},
	}
}

// Name returns the allowlist name.
func (s *Source) Name() string { return "github" }

// Fetch retrieves the GitHub meta API and returns relevant IPv4 CIDR prefixes.
func (s *Source) Fetch(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "iplist/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, apiURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode github meta: %w", err)
	}

	prefixes := make(map[string]struct{})
	for _, key := range keys {
		raw, ok := doc[key]
		if !ok {
			continue
		}
		var list []string
		if err := json.Unmarshal(raw, &list); err != nil {
			// GitHub may add non-array fields; ignore them.
			continue
		}
		for _, p := range list {
			if isIPv4CIDR(p) {
				prefixes[p] = struct{}{}
			}
		}
	}

	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no IPv4 prefixes found in GitHub meta response")
	}

	return sortedKeys(prefixes), nil
}

func isIPv4CIDR(s string) bool {
	ip, _, err := net.ParseCIDR(s)
	if err != nil {
		return false
	}
	return ip.To4() != nil
}

func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
