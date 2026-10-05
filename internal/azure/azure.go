// Package azure fetches Azure Service Tag IPv4 prefixes for the allowlist.
package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"time"
)

var detailsURL = "https://www.microsoft.com/en-us/download/details.aspx?id=56519"

const tagName = "AzureBotService"

var jsonURLRE = regexp.MustCompile(`https?://[^"\s<>]+?\.json`)

// Source fetches the AzureBotService service tag IPv4 prefixes.
type Source struct {
	client *http.Client
}

// NewSource returns a new Azure allowlist source.
func NewSource(timeout time.Duration) *Source {
	return &Source{
		client: &http.Client{Timeout: timeout},
	}
}

// Name returns the allowlist name.
func (s *Source) Name() string { return "azure" }

// Fetch retrieves the current Azure Service Tags JSON and returns IPv4 prefixes for AzureBotService.
func (s *Source) Fetch(ctx context.Context) ([]string, error) {
	jsonURL, err := s.resolveJSONURL(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve azure json url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jsonURL, nil)
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
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, jsonURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var doc struct {
		Values []struct {
			Name       string `json:"name"`
			Properties struct {
				AddressPrefixes []string `json:"addressPrefixes"`
			} `json:"properties"`
		} `json:"values"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode azure json: %w", err)
	}

	prefixes := make(map[string]struct{})
	for _, v := range doc.Values {
		if v.Name != tagName {
			continue
		}
		for _, p := range v.Properties.AddressPrefixes {
			if isIPv4CIDR(p) {
				prefixes[p] = struct{}{}
			}
		}
	}

	if len(prefixes) == 0 {
		return nil, fmt.Errorf("no IPv4 prefixes found for service tag %s", tagName)
	}

	return sortedKeys(prefixes), nil
}

func (s *Source) resolveJSONURL(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, detailsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "iplist/1.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// The JSON download URL is embedded in the page. Unescape HTML entities first,
	// then look for the first .json download link.
	unescaped := html.UnescapeString(string(body))
	matches := jsonURLRE.FindAllString(unescaped, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no json download link found on %s", detailsURL)
	}

	return matches[0], nil
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
