// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoverageBoostWebToolsHelpers(t *testing.T) {
	ua := localCodeUserAgent()
	if !strings.HasPrefix(ua, "Mozilla/5.0 LocalCode/") {
		t.Fatalf("unexpected user agent: %s", ua)
	}

	html := `<html><body><h1>Hello World</h1><p>Text</p></body></html>`
	cleaned := cleanHTMLText(html)
	if !strings.Contains(cleaned, "Hello World") || !strings.Contains(cleaned, "Text") || strings.Contains(cleaned, "<h1>") {
		t.Fatalf("unexpected cleaned html: %s", cleaned)
	}

	if !isForbiddenIP(net.ParseIP("127.0.0.1")) {
		t.Fatal("expected 127.0.0.1 to be forbidden")
	}
	if !isForbiddenIP(net.ParseIP("10.0.0.1")) {
		t.Fatal("expected 10.0.0.1 to be forbidden")
	}
	if !isForbiddenIP(net.ParseIP("192.168.1.1")) {
		t.Fatal("expected 192.168.1.1 to be forbidden")
	}
	if !isForbiddenIP(net.ParseIP("169.254.1.1")) {
		t.Fatal("expected link-local IP to be forbidden")
	}
	if !isForbiddenIP(net.ParseIP("::1")) {
		t.Fatal("expected ::1 to be forbidden")
	}
	if isForbiddenIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("expected public IP 8.8.8.8 not to be forbidden")
	}

	if _, err := validatePublicURL("ftp://example.com"); err == nil {
		t.Fatal("expected non-http(s) scheme to fail")
	}
	if _, err := validatePublicURL("http:///no-host"); err == nil {
		t.Fatal("expected empty host to fail")
	}
	if _, err := validatePublicURL("http://127.0.0.1:8080"); err == nil {
		t.Fatal("expected private IP url to fail")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := publicOnlyDialContext(ctx, "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected dial to 127.0.0.1 to fail")
	}

	client := publicHTTPClient(5*time.Second, 3)
	if client == nil || client.Timeout == 0 {
		t.Fatal("expected valid public http client")
	}

	formatted := formatWebResults([]WebResult{{Title: "Test", URL: "https://example.com", Content: "Sample text"}})
	if !strings.Contains(formatted, "Test") || !strings.Contains(formatted, "https://example.com") {
		t.Fatalf("unexpected format: %s", formatted)
	}
}

func TestCoverageBoostRemoteServerEdgeCases(t *testing.T) {
	state := newRemoteTestState(t)
	server := NewRemoteServer(state)

	// Equal host port tests
	if !equalHostPort("http", "localhost:80", "localhost") {
		t.Fatal("expected default port 80 match")
	}
	if !equalHostPort("https", "localhost:443", "localhost") {
		t.Fatal("expected default port 443 match")
	}
	if equalHostPort("http", "localhost:8080", "localhost:9090") {
		t.Fatal("expected port mismatch")
	}
	if equalHostPort("http", "a.com:80", "b.com:80") {
		t.Fatal("expected host mismatch")
	}

	// sameRequestOrigin tests
	if !sameRequestOrigin("http://127.0.0.1:32146", "127.0.0.1:32146") {
		t.Fatal("expected same origin match")
	}
	if sameRequestOrigin("ftp://127.0.0.1", "127.0.0.1") {
		t.Fatal("expected invalid scheme mismatch")
	}

	// Ping handler
	req := httptest.NewRequest(http.MethodGet, "/remote/api/ping", nil)
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ping status=%d", rr.Code)
	}

	// Ping method not allowed
	reqPost := httptest.NewRequest(http.MethodPost, "/remote/api/ping", nil)
	rrPost := httptest.NewRecorder()
	server.ServeHTTP(rrPost, reqPost)
	if rrPost.Code != http.StatusMethodNotAllowed {
		t.Fatalf("ping POST status=%d", rrPost.Code)
	}

	// Remote page HEAD
	reqHead := httptest.NewRequest(http.MethodHead, "/remote", nil)
	rrHead := httptest.NewRecorder()
	server.ServeHTTP(rrHead, reqHead)
	if rrHead.Code != http.StatusOK {
		t.Fatalf("remote HEAD status=%d", rrHead.Code)
	}

	// Remote page 404
	req404 := httptest.NewRequest(http.MethodGet, "/remote/not-found", nil)
	rr404 := httptest.NewRecorder()
	server.ServeHTTP(rr404, req404)
	if rr404.Code != http.StatusNotFound {
		t.Fatalf("remote 404 status=%d", rr404.Code)
	}

	// Unpair method not allowed
	reqUnpairGet := httptest.NewRequest(http.MethodGet, "/remote/api/unpair", nil)
	rrUnpairGet := httptest.NewRecorder()
	server.ServeHTTP(rrUnpairGet, reqUnpairGet)
	if rrUnpairGet.Code != http.StatusUnauthorized { // auth checks first
		t.Fatalf("unpair GET without auth status=%d", rrUnpairGet.Code)
	}
}

func TestCoverageBoostAppStateAndToolRegistry(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{RootProjectDir: dir}
	if err := saveConfig(cfg); err != nil {
		t.Logf("saveConfig note: %v", err)
	}

	state := NewAppState(cfg, NewOllamaClient())
	if state == nil {
		t.Fatal("NewAppState returned nil")
	}

	// AddEvent and Close
	state.AddEvent(UIEvent{ThreadID: "test-thread", Type: "user", Message: "hello"})
	state.Close()

	// Tool registry helpers
	if canonicalToolName("adb.exe") != "adb" {
		t.Fatalf("unexpected canonical name: %s", canonicalToolName("adb.exe"))
	}
	if executableName("git") == "" {
		t.Fatal("expected non-empty executable name")
	}
	if scriptName("build.ps1") == "" {
		t.Fatal("expected non-empty script name")
	}

	p := profileForTool("git")
	if p.Name != "git" {
		t.Fatalf("unexpected profile: %+v", p)
	}

	roots := androidSDKRoots(dir)
	_ = roots

	u := uniquePaths([]string{"C:\\a", "C:\\a", "C:\\b", ""})
	if len(u) != 2 {
		t.Fatalf("unexpected unique paths: %v", u)
	}

	args := quoteArgs([]string{"a", "b with space", "\"quoted\""})
	if !strings.Contains(args, `"b with space"`) {
		t.Fatalf("unexpected quoted args: %s", args)
	}

	cmdLine := buildWindowsCommandLine("git", []string{"status", "--short"})
	if !strings.Contains(cmdLine, "git") || !strings.Contains(cmdLine, "status") {
		t.Fatalf("unexpected cmdLine: %s", cmdLine)
	}

	// Normalize questions
	if !sameQuestion("Is this ok?", "is this ok?") {
		t.Fatal("expected same normalized question")
	}

	head, rest, ok := splitCommandHead("git commit -m \"test\"")
	if !ok || head != "git" || rest != "commit -m \"test\"" {
		t.Fatalf("unexpected split head=%s rest=%s ok=%v", head, rest, ok)
	}
}

func TestCoverageBoostFileTools(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if !isProbablyText([]byte("Hello world\nThis is plain text\twith tabs.")) {
		t.Fatal("expected plain text to be recognized")
	}
	if isProbablyText([]byte{0x00, 0x01, 0x02, 0xFF, 0xFE}) {
		t.Fatal("expected binary data not to be recognized as plain text")
	}

	content, err := readProjectFile(dir, "hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "line1") || !strings.Contains(content, "line2") {
		t.Fatalf("unexpected content: %s", content)
	}

	diff := simpleDiff("old content\nunchanged\n", "new content\nunchanged\n")
	if !strings.Contains(diff, "-old content") || !strings.Contains(diff, "+new content") {
		t.Fatalf("unexpected diff: %s", diff)
	}
}
