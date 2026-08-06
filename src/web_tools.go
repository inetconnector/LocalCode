// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type WebResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

var ddgResultRE = regexp.MustCompile(`(?is)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>.*?<a[^>]+class="result__snippet"[^>]*>(.*?)</a>`)
var tagRE = regexp.MustCompile(`(?is)<[^>]+>`)
var spaceRE = regexp.MustCompile(`\s+`)

// Indirections keep network code deterministic in tests while production uses
// the hardened resolver and HTTP client below.
var webLookupIP = net.LookupIP
var webHTTPClient = publicHTTPClient
var ollamaWebSearchEndpoint = "https://ollama.com/api/web_search"
var duckDuckGoSearchEndpoint = "https://html.duckduckgo.com/html/"
var bingRSSSearchEndpoint = "https://www.bing.com/search"

func cleanHTMLText(s string) string {
	s = tagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(spaceRE.ReplaceAllString(s, " "))
}

func webSearch(ctx context.Context, cfg Config, query string, maxResults int) ([]WebResult, error) {
	if !cfg.NetworkEnabled {
		return nil, errors.New("network access is disabled in settings")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	if maxResults <= 0 {
		maxResults = cfg.WebSearchMaxResults
	}
	if maxResults > 10 {
		maxResults = 10
	}
	switch cfg.WebSearchProvider {
	case "ollama":
		return ollamaWebSearch(ctx, cfg, query, maxResults)
	case "duckduckgo":
		results, err := duckDuckGoSearch(ctx, query, maxResults)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		fallback, fallbackErr := bingRSSSearch(ctx, query, maxResults)
		if fallbackErr == nil && len(fallback) > 0 {
			return fallback, nil
		}
		if err != nil && fallbackErr != nil {
			return nil, fmt.Errorf("web search failed: DuckDuckGo: %v; Bing RSS: %v", err, fallbackErr)
		}
		return fallback, fallbackErr
	default:
		return nil, errors.New("web search provider is disabled")
	}
}

func ollamaWebSearch(ctx context.Context, cfg Config, query string, maxResults int) ([]WebResult, error) {
	key := strings.TrimSpace(os.Getenv(cfg.WebSearchAPIKeyEnv))
	if key == "" {
		return nil, fmt.Errorf("environment variable %s is not set", cfg.WebSearchAPIKeyEnv)
	}
	payload, _ := json.Marshal(map[string]any{"query": query, "max_results": maxResults})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaWebSearchEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := webHTTPClient(45*time.Second, 8).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama web search HTTP %d: %s", resp.StatusCode, truncateText(string(body), 1200))
	}
	var out struct {
		Results []WebResult `json:"results"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Results, nil
}

func duckDuckGoSearch(ctx context.Context, query string, maxResults int) ([]WebResult, error) {
	endpoint := duckDuckGoSearchEndpoint + "?q=" + url.QueryEscape(query)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 LocalCode/6.4.1")
	resp, err := webHTTPClient(35*time.Second, 8).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo HTTP %d", resp.StatusCode)
	}
	matches := ddgResultRE.FindAllStringSubmatch(string(body), maxResults)
	results := make([]WebResult, 0, len(matches))
	for _, m := range matches {
		target := html.UnescapeString(m[1])
		if u, err := url.Parse(target); err == nil {
			if redirect := u.Query().Get("uddg"); redirect != "" {
				target = redirect
			}
		}
		results = append(results, WebResult{Title: cleanHTMLText(m[2]), URL: target, Content: cleanHTMLText(m[3])})
	}
	if len(results) == 0 {
		return nil, errors.New("search returned no parseable results")
	}
	return results, nil
}

func isForbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

func validatePublicURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("only http and https URLs are allowed")
	}
	if u.Hostname() == "" {
		return nil, errors.New("URL has no hostname")
	}
	ips, err := webLookupIP(u.Hostname())
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isForbiddenIP(ip) {
			return nil, fmt.Errorf("private or local address is blocked: %s", ip.String())
		}
	}
	return u, nil
}

func publicOnlyDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if parsed := net.ParseIP(host); parsed != nil {
		if isForbiddenIP(parsed) {
			return nil, fmt.Errorf("private or local address is blocked: %s", parsed.String())
		}
		dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
		return dialer.DialContext(ctx, network, net.JoinHostPort(parsed.String(), port))
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var public []net.IP
	for _, address := range addresses {
		if isForbiddenIP(address.IP) {
			return nil, fmt.Errorf("private or local address is blocked: %s", address.IP.String())
		}
		public = append(public, address.IP)
	}
	if len(public) == 0 {
		return nil, errors.New("hostname did not resolve to a public address")
	}
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, ip := range public {
		conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}
	return nil, lastErr
}

func publicHTTPClient(timeout time.Duration, maxRedirects int) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Do not delegate target resolution to an HTTP proxy. The custom dialer
	// resolves once, validates every returned IP and dials that exact IP, which
	// prevents DNS rebinding between validation and connection establishment.
	transport.Proxy = nil
	transport.DialContext = publicOnlyDialContext
	client := &http.Client{Timeout: timeout, Transport: transport}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > maxRedirects {
			return errors.New("too many redirects")
		}
		_, err := validatePublicURL(req.URL.String())
		return err
	}
	return client
}

func webFetch(ctx context.Context, cfg Config, rawURL string) (string, error) {
	if !cfg.NetworkEnabled {
		return "", errors.New("network access is disabled in settings")
	}
	u, err := validatePublicURL(rawURL)
	if err != nil {
		return "", err
	}
	maxBytes := cfg.WebFetchMaxBytes
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	client := webHTTPClient(45*time.Second, 8)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 LocalCode/6.4.1")
	req.Header.Set("Accept", "text/html,application/json,text/plain;q=0.9,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > maxBytes {
		return "", fmt.Errorf("response exceeded %d bytes", maxBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateText(string(body), 1200))
	}
	ctype := strings.ToLower(resp.Header.Get("Content-Type"))
	text := string(body)
	if strings.Contains(ctype, "html") {
		text = cleanHTMLText(text)
	}
	return "URL: " + u.String() + "\nContent-Type: " + ctype + "\n\n" + truncateText(text, 120000), nil
}

func formatWebResults(results []WebResult) string {
	var b strings.Builder
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n%s\n%s\n\n", i+1, r.Title, r.URL, r.Content)
	}
	return strings.TrimSpace(b.String())
}

type rssSearchFeed struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			PubDate     string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

func bingRSSSearch(ctx context.Context, query string, maxResults int) ([]WebResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	endpoint := bingRSSSearchEndpoint + "?format=rss&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 LocalCode/6.4.1")
	resp, err := webHTTPClient(35*time.Second, 8).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bing RSS HTTP %d", resp.StatusCode)
	}
	var feed rssSearchFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	results := make([]WebResult, 0, minInt(maxResults, len(feed.Channel.Items)))
	for _, item := range feed.Channel.Items {
		if len(results) >= maxResults {
			break
		}
		content := cleanHTMLText(item.Description)
		if strings.TrimSpace(item.PubDate) != "" {
			content = strings.TrimSpace(item.PubDate) + " — " + content
		}
		results = append(results, WebResult{Title: cleanHTMLText(item.Title), URL: strings.TrimSpace(item.Link), Content: content})
	}
	if len(results) == 0 {
		return nil, errors.New("Bing RSS returned no results")
	}
	return results, nil
}
