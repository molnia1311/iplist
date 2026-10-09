// Package m365 fetches Microsoft 365 IPv4 prefixes from the Office 365 endpoints API.
package m365

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const clientRequestID = "f1f201f2-ea1b-4427-b54c-82bed65ca111"

var (
	versionURL   = "https://endpoints.office.com/version/Worldwide?ClientRequestId=" + clientRequestID
	endpointsURL = "https://endpoints.office.com/endpoints/Worldwide?ClientRequestId=" + clientRequestID
)

// endpointSet represents one object in the Microsoft 365 endpoints array.
type endpointSet struct {
	ID                     int      `json:"id"`
	ServiceArea            string   `json:"serviceArea"`
	ServiceAreaDisplayName string   `json:"serviceAreaDisplayName"`
	IPs                    []string `json:"ips"`
	URLs                   []string `json:"urls"`
	TCPPorts               string   `json:"tcpPorts"`
	UDPPorts               string   `json:"udpPorts"`
	Category               string   `json:"category"`
	Required               bool     `json:"required"`
	ExpressRoute           bool     `json:"expressRoute"`
	Notes                  string   `json:"notes"`
}

// Source fetches Microsoft 365 IPv4 prefixes.
type Source struct {
	client      *http.Client
	mu          sync.Mutex
	lastVersion string
	lastResult  map[string][]string
}

// NewSource returns a new Microsoft 365 allowlist source.
func NewSource(timeout time.Duration) *Source {
	return &Source{
		client: &http.Client{Timeout: timeout},
	}
}

// Name returns the allowlist name.
func (s *Source) Name() string { return "m365" }

// Fetch retrieves the current Microsoft 365 endpoints and returns IPv4 prefixes
// grouped by lowercase service area.
func (s *Source) Fetch(ctx context.Context) (map[string][]string, error) {
	latest, err := s.fetchVersion(ctx)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if latest == s.lastVersion && s.lastResult != nil {
		result := s.lastResult
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()

	sets, err := s.fetchEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[string]struct{})
	for _, set := range sets {
		if len(set.IPs) == 0 {
			continue
		}
		area := strings.ToLower(set.ServiceArea)
		if result[area] == nil {
			result[area] = make(map[string]struct{})
		}
		for _, p := range set.IPs {
			if isIPv4CIDR(p) {
				result[area][p] = struct{}{}
			}
		}
	}

	out := make(map[string][]string)
	for area, prefixes := range result {
		if len(prefixes) == 0 {
			continue
		}
		out[area] = sortedKeys(prefixes)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no IPv4 prefixes found in Microsoft 365 endpoints")
	}

	s.mu.Lock()
	s.lastVersion = latest
	s.lastResult = out
	s.mu.Unlock()

	return out, nil
}

func (s *Source) fetchVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "iplist/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, versionURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var doc struct {
		Instance string `json:"instance"`
		Latest   string `json:"latest"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("decode m365 version: %w", err)
	}
	if doc.Latest == "" {
		return "", fmt.Errorf("m365 version response missing latest field")
	}
	return doc.Latest, nil
}

func (s *Source) fetchEndpoints(ctx context.Context) ([]endpointSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "iplist/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, endpointsURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var sets []endpointSet
	if err := json.Unmarshal(body, &sets); err != nil {
		return nil, fmt.Errorf("decode m365 endpoints: %w", err)
	}
	return sets, nil
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
