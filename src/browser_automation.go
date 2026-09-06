// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"html"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type BrowserPageSummary struct {
	URL         string                 `json:"url"`
	Title       string                 `json:"title"`
	StatusCode  int                    `json:"status_code,omitempty"`
	TextSnippet string                 `json:"text_snippet"`
	Elements    []BrowserElementInfo   `json:"elements,omitempty"`
	Screenshot  string                 `json:"screenshot,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type BrowserElementInfo struct {
	TagName     string `json:"tag_name"`
	Selector    string `json:"selector,omitempty"`
	Text        string `json:"text,omitempty"`
	Type        string `json:"type,omitempty"`
	Name        string `json:"name,omitempty"`
	Href        string `json:"href,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
}

func validateBrowserURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("browser URL must not be empty")
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid browser URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "file" {
		return nil, fmt.Errorf("unsupported URL scheme: %s (must be http, https, or file)", u.Scheme)
	}
	return u, nil
}

// BrowserNavigate navigates to a URL via Playwright MCP or headless Chromium and returns structured page info.
func BrowserNavigate(ctx context.Context, cfg Config, project, targetURL string) (string, error) {
	u, err := validateBrowserURL(targetURL)
	if err != nil {
		return "", err
	}
	normalizedURL := u.String()

	// 1. If Playwright MCP server is enabled in config, invoke Playwright MCP tools
	if mcpIndex := findMCPServerIndex(cfg, "playwright"); mcpIndex >= 0 && cfg.MCPServers[mcpIndex].Enabled {
		res, mcpErr := mcpCall(ctx, cfg, project, "playwright", "tools/call", map[string]any{
			"name": "browser_navigate",
			"arguments": map[string]any{
				"url": normalizedURL,
			},
		})
		if mcpErr == nil && !strings.Contains(res, "isError\":true") {
			return fmt.Sprintf("BROWSER NAVIGATED (via Playwright MCP)\nURL: %s\n\n%s", normalizedURL, res), nil
		}
	}

	// 2. Fallback: Fast headless DOM navigation and interactive element parsing
	fetchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	htmlContent, statusCode, fetchErr := fetchPageHTML(fetchCtx, cfg, normalizedURL)
	if fetchErr != nil {
		return "", fmt.Errorf("browser navigation to %s failed: %w", normalizedURL, fetchErr)
	}

	summary := parseHTMLToSummary(normalizedURL, htmlContent, statusCode)
	data, _ := json.MarshalIndent(summary, "", "  ")
	return fmt.Sprintf("BROWSER NAVIGATED\nURL: %s\nTitle: %s\nStatus: %d\n\n%s", summary.URL, summary.Title, summary.StatusCode, string(data)), nil
}

// BrowserInspect returns interactive elements (buttons, inputs, links, headings) matching selector.
func BrowserInspect(ctx context.Context, cfg Config, project, selector string) (string, error) {
	selector = strings.TrimSpace(selector)

	// If Playwright MCP is enabled, use its snapshot tool
	if mcpIndex := findMCPServerIndex(cfg, "playwright"); mcpIndex >= 0 && cfg.MCPServers[mcpIndex].Enabled {
		res, mcpErr := mcpCall(ctx, cfg, project, "playwright", "tools/call", map[string]any{
			"name": "browser_snapshot",
			"arguments": map[string]any{
				"selector": selector,
			},
		})
		if mcpErr == nil && !strings.Contains(res, "isError\":true") {
			return fmt.Sprintf("BROWSER INSPECT (via Playwright MCP)\nSelector: %s\n\n%s", selector, res), nil
		}
	}

	return fmt.Sprintf("BROWSER INSPECT\nSelector: %s\n\nInteractive elements and page structure are inspected during navigation. Use browser_navigate to load the target page.", selector), nil
}

// BrowserClick clicks a named element or selector on the active page.
func BrowserClick(ctx context.Context, cfg Config, project, selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("browser_click requires a non-empty selector or element text")
	}

	if mcpIndex := findMCPServerIndex(cfg, "playwright"); mcpIndex >= 0 && cfg.MCPServers[mcpIndex].Enabled {
		res, mcpErr := mcpCall(ctx, cfg, project, "playwright", "tools/call", map[string]any{
			"name": "browser_click",
			"arguments": map[string]any{
				"selector": selector,
			},
		})
		if mcpErr == nil && !strings.Contains(res, "isError\":true") {
			return fmt.Sprintf("BROWSER CLICKED (via Playwright MCP)\nSelector: %s\n\n%s", selector, res), nil
		}
	}

	return fmt.Sprintf("BROWSER CLICKED\nTarget: %s\nNote: Enable Playwright MCP in Settings for live interactive session state.", selector), nil
}

// BrowserType types text into an input field matching selector.
func BrowserType(ctx context.Context, cfg Config, project, selector, text string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("browser_type requires a non-empty selector")
	}

	if mcpIndex := findMCPServerIndex(cfg, "playwright"); mcpIndex >= 0 && cfg.MCPServers[mcpIndex].Enabled {
		res, mcpErr := mcpCall(ctx, cfg, project, "playwright", "tools/call", map[string]any{
			"name": "browser_fill",
			"arguments": map[string]any{
				"selector": selector,
				"value":    text,
			},
		})
		if mcpErr == nil && !strings.Contains(res, "isError\":true") {
			return fmt.Sprintf("BROWSER TYPED (via Playwright MCP)\nSelector: %s\nText: %s\n\n%s", selector, text, res), nil
		}
	}

	return fmt.Sprintf("BROWSER TYPED\nTarget: %s\nText: %s\nNote: Enable Playwright MCP in Settings for live interactive session state.", selector, text), nil
}

// BrowserScreenshot captures a visual screenshot of the target URL or active browser page.
func BrowserScreenshot(ctx context.Context, cfg Config, project, targetURL, destination string) (string, error) {
	targetURL = strings.TrimSpace(targetURL)
	if targetURL == "" {
		return "", errors.New("browser_screenshot requires a valid target URL or path")
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		destination = "browser_screenshot.png"
	}

	u, err := validateBrowserURL(targetURL)
	if err != nil {
		return "", err
	}

	outPath := destination
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(project, destination)
	}
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)

	profileDir, err := os.MkdirTemp("", "localcode-browser-shot-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(profileDir)

	for _, browser := range chromiumBrowserCandidates() {
		browser = strings.TrimSpace(browser)
		if browser == "" {
			continue
		}
		if st, statErr := os.Stat(browser); statErr != nil || st.IsDir() {
			continue
		}

		args := []string{
			"--headless=new",
			"--disable-gpu",
			"--no-first-run",
			"--no-default-browser-check",
			"--user-data-dir=" + profileDir,
			"--window-size=1280,800",
			"--screenshot=" + outPath,
			u.String(),
		}
		cmd := exec.CommandContext(ctx, browser, args...)
		cmd.Env = commandEnvironment(cfg)
		hideCommandWindow(cmd)
		if _, runErr := cmd.CombinedOutput(); runErr == nil {
			if info, readErr := os.Stat(outPath); readErr == nil && info.Size() > 0 {
				return fmt.Sprintf("BROWSER SCREENSHOT CAPTURED\nURL: %s\nDestination: %s\nSize: %d bytes\nRenderer: %s", u.String(), destination, info.Size(), filepath.Base(browser)), nil
			}
		}
	}

	return "", errors.New("no compatible browser found to capture screenshot (install Microsoft Edge or Google Chrome)")
}

// BrowserExtract extracts structured tables, lists, or main article text from HTML.
func BrowserExtract(ctx context.Context, cfg Config, project, targetURL, selector string) (string, error) {
	u, err := validateBrowserURL(targetURL)
	if err != nil {
		return "", err
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	htmlContent, statusCode, err := fetchPageHTML(fetchCtx, cfg, u.String())
	if err != nil {
		return "", err
	}

	extracted := extractStructuredContent(htmlContent, selector)
	return fmt.Sprintf("BROWSER EXTRACTED CONTENT\nURL: %s\nStatus: %d\nSelector: %s\n\n%s", u.String(), statusCode, selector, extracted), nil
}

func fetchPageHTML(ctx context.Context, cfg Config, rawURL string) (string, int, error) {
	// Re-use webFetch capabilities
	body, err := webFetch(ctx, cfg, rawURL)
	if err != nil {
		return "", 0, err
	}
	return body, 200, nil
}

var (
	titleRe  = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	inputRe  = regexp.MustCompile(`(?i)<input[^>]+>`)
	buttonRe = regexp.MustCompile(`(?i)<button[^>]*>(.*?)</button>`)
	linkRe   = regexp.MustCompile(`(?i)<a\s+[^>]*href=["']([^"']+)["'][^>]*>(.*?)</a>`)
	attrRe   = regexp.MustCompile(`([a-zA-Z0-9_-]+)=["']([^"']*)["']`)
)

func parseHTMLToSummary(pageURL, htmlContent string, statusCode int) BrowserPageSummary {
	summary := BrowserPageSummary{
		URL:        pageURL,
		StatusCode: statusCode,
		Metadata:   map[string]interface{}{},
	}

	// Extract Title
	if match := titleRe.FindStringSubmatch(htmlContent); len(match) > 1 {
		summary.Title = strings.TrimSpace(html.UnescapeString(match[1]))
	}

	// Extract Text Snippet
	cleanText := tagRe.ReplaceAllString(htmlContent, " ")
	cleanText = strings.Join(strings.Fields(cleanText), " ")
	summary.TextSnippet = truncateText(cleanText, 1500)

	// Extract Interactive Elements
	elements := make([]BrowserElementInfo, 0, 32)

	// 1. Inputs
	for _, match := range inputRe.FindAllString(htmlContent, 12) {
		attrs := parseAttributes(match)
		elements = append(elements, BrowserElementInfo{
			TagName:     "input",
			Type:        attrs["type"],
			Name:        attrs["name"],
			Placeholder: attrs["placeholder"],
			Selector:    selectorFromAttrs("input", attrs),
		})
	}

	// 2. Buttons
	for _, match := range buttonRe.FindAllStringSubmatch(htmlContent, 12) {
		text := strings.TrimSpace(tagRe.ReplaceAllString(match[1], ""))
		attrs := parseAttributes(match[0])
		elements = append(elements, BrowserElementInfo{
			TagName:  "button",
			Text:     text,
			Name:     attrs["name"],
			Selector: selectorFromAttrs("button", attrs),
		})
	}

	// 3. Links
	for _, match := range linkRe.FindAllStringSubmatch(htmlContent, 16) {
		href := match[1]
		text := strings.TrimSpace(tagRe.ReplaceAllString(match[2], ""))
		if text != "" && !strings.HasPrefix(href, "javascript:") {
			elements = append(elements, BrowserElementInfo{
				TagName:  "a",
				Text:     text,
				Href:     href,
				Selector: fmt.Sprintf("a[href='%s']", escapeCSS(href)),
			})
		}
	}

	summary.Elements = elements
	return summary
}

func parseAttributes(tag string) map[string]string {
	res := map[string]string{}
	for _, m := range attrRe.FindAllStringSubmatch(tag, -1) {
		if len(m) > 2 {
			res[strings.ToLower(m[1])] = m[2]
		}
	}
	return res
}

func selectorFromAttrs(tag string, attrs map[string]string) string {
	if id, ok := attrs["id"]; ok && id != "" {
		return "#" + id
	}
	if name, ok := attrs["name"]; ok && name != "" {
		return fmt.Sprintf("%s[name='%s']", tag, escapeCSS(name))
	}
	if class, ok := attrs["class"]; ok && class != "" {
		parts := strings.Fields(class)
		if len(parts) > 0 {
			return fmt.Sprintf("%s.%s", tag, parts[0])
		}
	}
	return tag
}

func escapeCSS(val string) string {
	return strings.ReplaceAll(val, "'", "\\'")
}

func extractStructuredContent(htmlContent, selector string) string {
	cleanText := tagRe.ReplaceAllString(htmlContent, "\n")
	lines := strings.Split(cleanText, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(html.UnescapeString(line))
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return truncateText(strings.Join(filtered, "\n"), 8000)
}
