// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseHTMLToSummary(t *testing.T) {
	htmlSample := `<!DOCTYPE html>
<html>
<head><title>LocalCode Test Portal</title></head>
<body>
  <h1>Welcome to LocalCode</h1>
  <p>Autonomous AI software development workstation.</p>
  <form id="loginForm">
    <input type="text" name="username" placeholder="Username" id="userInp">
    <input type="password" name="password" placeholder="Password">
    <button type="submit" name="loginBtn">Login</button>
  </form>
  <div class="navigation">
    <a href="/docs/api">API Documentation</a>
    <a href="https://example.com/support">Support Link</a>
  </div>
</body>
</html>`

	summary := parseHTMLToSummary("https://localcode.dev", htmlSample, 200)

	if summary.Title != "LocalCode Test Portal" {
		t.Errorf("expected title 'LocalCode Test Portal', got %q", summary.Title)
	}
	if summary.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", summary.StatusCode)
	}
	if !strings.Contains(summary.TextSnippet, "Autonomous AI software development") {
		t.Errorf("expected text snippet to contain text, got %q", summary.TextSnippet)
	}

	if len(summary.Elements) < 4 {
		t.Fatalf("expected at least 4 interactive elements, got %d", len(summary.Elements))
	}

	foundInput := false
	foundButton := false
	foundLink := false
	for _, el := range summary.Elements {
		if el.TagName == "input" && el.Name == "username" {
			foundInput = true
			if el.Selector != "#userInp" {
				t.Errorf("expected input selector #userInp, got %s", el.Selector)
			}
		}
		if el.TagName == "button" && el.Text == "Login" {
			foundButton = true
		}
		if el.TagName == "a" && strings.Contains(el.Href, "api") {
			foundLink = true
		}
	}

	if !foundInput {
		t.Error("expected username input element in summary")
	}
	if !foundButton {
		t.Error("expected login button in summary")
	}
	if !foundLink {
		t.Error("expected api link in summary")
	}
}

func TestValidateBrowserURL(t *testing.T) {
	u, err := validateBrowserURL("https://example.com")
	if err != nil || u.Host != "example.com" {
		t.Fatalf("unexpected validation error: %v", err)
	}

	u, err = validateBrowserURL("localhost:8080/test")
	if err != nil || !strings.Contains(u.String(), "localhost:8080/test") {
		t.Fatalf("auto-prefix https failed: %v, url: %v", err, u)
	}

	_, err = validateBrowserURL("ftp://malicious.com")
	if err == nil {
		t.Fatal("expected error on unsupported scheme ftp://")
	}
}

func TestBrowserAndDesktopApprovalRules(t *testing.T) {
	cfg := Config{ApprovalMode: "normal"}

	readOnlyActions := []AgentAction{
		{Action: "browser_inspect", Selector: "#btn"},
		{Action: "desktop_list_windows"},
		{Action: "desktop_inspect", WindowTitle: "Notepad"},
	}

	for _, a := range readOnlyActions {
		if actionNeedsApproval(cfg, "project", a) {
			t.Errorf("action %s should not require approval in normal mode", a.Action)
		}
	}

	mutatingActions := []AgentAction{
		{Action: "browser_click", Selector: "#btn"},
		{Action: "browser_type", Selector: "#input", Text: "hello"},
		{Action: "desktop_click", WindowTitle: "Notepad", ControlName: "Save"},
		{Action: "desktop_type", WindowTitle: "Notepad", ControlName: "Edit", Text: "content"},
	}

	for _, a := range mutatingActions {
		if !actionNeedsApproval(cfg, "project", a) {
			t.Errorf("action %s must require approval in normal mode", a.Action)
		}
	}

	// In dangerous mode, all actions are allowed without approval
	dangerousCfg := Config{ApprovalMode: "dangerous"}
	for _, a := range mutatingActions {
		if actionNeedsApproval(dangerousCfg, "project", a) {
			t.Errorf("action %s should not require approval in dangerous mode", a.Action)
		}
	}
}

func TestPreviewActionBrowserAndDesktop(t *testing.T) {
	cfg := Config{}
	project := t.TempDir()

	tests := []struct {
		action AgentAction
		want   string
	}{
		{
			action: AgentAction{Action: "browser_navigate", URL: "https://example.com"},
			want:   "Browser navigation preview",
		},
		{
			action: AgentAction{Action: "browser_click", Selector: "#submit"},
			want:   "Browser click preview",
		},
		{
			action: AgentAction{Action: "browser_type", Selector: "#user", Text: "admin"},
			want:   "Browser type preview",
		},
		{
			action: AgentAction{Action: "desktop_click", WindowTitle: "Calculator", ControlName: "Five"},
			want:   "Desktop click preview",
		},
		{
			action: AgentAction{Action: "desktop_type", WindowTitle: "Notepad", ControlName: "Edit", Text: "Notes"},
			want:   "Desktop type preview",
		},
		{
			action: AgentAction{Action: "desktop_list_windows"},
			want:   "Desktop list windows preview",
		},
	}

	for _, tc := range tests {
		preview, err := previewAction(project, cfg, tc.action)
		if err != nil {
			t.Fatalf("previewAction for %s returned error: %v", tc.action.Action, err)
		}
		if !strings.Contains(preview, tc.want) {
			t.Errorf("expected preview for %s to contain %q, got %q", tc.action.Action, tc.want, preview)
		}
	}
}

func TestBlockedDesktopWindowFilter(t *testing.T) {
	if !isBlockedDesktopWindow("Windows Security", "") {
		t.Error("expected 'Windows Security' to be blocked")
	}
	if !isBlockedDesktopWindow("Task Manager", "Taskmgr") {
		t.Error("expected 'Task Manager' to be blocked")
	}
	if isBlockedDesktopWindow("Visual Studio Code", "Code.exe") {
		t.Error("expected 'Visual Studio Code' to not be blocked")
	}
	if isBlockedDesktopWindow("Microsoft Edge", "msedge.exe") {
		t.Error("expected 'Microsoft Edge' to not be blocked")
	}
}

func TestDoctorDiagnosticsBrowserAndDesktop(t *testing.T) {
	cfg := Config{}
	report := RunDoctorDiagnostics(context.Background(), cfg)

	foundBrowser := false
	foundDesktop := false
	for _, item := range report.Items {
		if item.Category == "browser_automation" {
			foundBrowser = true
			if item.Name != "Autonomous Browser Automation" {
				t.Errorf("unexpected browser diag name: %s", item.Name)
			}
		}
		if item.Category == "desktop_automation" {
			foundDesktop = true
			if item.Name != "Windows Desktop & UI Automation" {
				t.Errorf("unexpected desktop diag name: %s", item.Name)
			}
		}
	}

	if !foundBrowser {
		t.Error("missing browser_automation item in Doctor report")
	}
	if !foundDesktop {
		t.Error("missing desktop_automation item in Doctor report")
	}
}

func TestBrowserFallbackScreenshot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("skipping browser screenshot test on non-windows")
	}
	project := t.TempDir()
	sampleHTML := filepath.Join(project, "sample.html")
	_ = os.WriteFile(sampleHTML, []byte("<html><body><h1>Test Page</h1></body></html>"), 0o644)

	cfg := Config{}
	dest := filepath.Join(project, "out.png")
	u := "file:///" + filepath.ToSlash(sampleHTML)
	_, err := BrowserScreenshot(context.Background(), cfg, project, u, dest)
	// If Edge or Chrome is installed on the machine, it generates the screenshot.
	// If not installed, it returns a clean diagnostic error.
	if err == nil {
		if _, statErr := os.Stat(dest); statErr != nil {
			t.Errorf("screenshot file not found at %s", dest)
		}
	}
}
