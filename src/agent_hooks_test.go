// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHooksV2CommandReceivesStructuredJSONStdin(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.CommandTimeout = 15

	var command string
	if runtime.GOOS == "windows" {
		command = `$raw = [Console]::In.ReadToEnd(); $j = $raw | ConvertFrom-Json; Write-Output ("EVENT=" + $j.event + ";ACTION=" + $j.action.action + ";PATH=" + $j.action.path)`
	} else {
		command = `cat`
	}

	def := HookDefinition{
		Name:    "stdin-inspector",
		Event:   HookEventOnEdit,
		Type:    "command",
		Command: command,
	}
	payload := HookPayload{
		Event:   HookEventOnEdit,
		Project: project,
		Action:  AgentAction{Action: "write_file", Path: "src/app.go", Message: "update app"},
	}

	res, err := RunHookV2(context.Background(), project, cfg, def, payload)
	if err != nil {
		t.Fatalf("RunHookV2 failed: %v (output=%q)", err, res.Output)
	}
	if res.Blocked {
		t.Fatalf("expected hook not to block, got %+v", res)
	}
	if runtime.GOOS == "windows" {
		want := "EVENT=OnEdit;ACTION=write_file;PATH=src/app.go"
		if !strings.Contains(res.Output, want) {
			t.Fatalf("expected %q in hook stdout, got %q", want, res.Output)
		}
	} else {
		if !strings.Contains(res.Output, `"event":"OnEdit"`) || !strings.Contains(res.Output, `"path":"src/app.go"`) {
			t.Fatalf("expected JSON payload on stdin, got %q", res.Output)
		}
	}
}

func TestHooksV2ExitCode2VetoBlocksExecution(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.CommandTimeout = 15

	var vetoCmd string
	var markerCmd string
	markerFile := filepath.Join(project, "second-hook-ran.txt")
	if runtime.GOOS == "windows" {
		vetoCmd = `[Console]::Error.WriteLine('policy veto: direct edits to lockfile forbidden'); exit 2`
		markerCmd = `Set-Content -LiteralPath '` + strings.ReplaceAll(markerFile, "'", "''") + `' -Value 'ran'`
	} else {
		vetoCmd = `echo 'policy veto: direct edits to lockfile forbidden' >&2; exit 2`
		markerCmd = `echo ran > "` + markerFile + `"`
	}

	defs := []HookDefinition{
		{
			Name:        "lockfile-guard",
			Event:       HookEventPreToolUse,
			Type:        "command",
			Command:     vetoCmd,
			ToolMatcher: "write_file",
		},
		{
			Name:    "should-not-run-after-veto",
			Event:   HookEventPreToolUse,
			Type:    "command",
			Command: markerCmd,
		},
	}

	payload := HookPayload{
		Event:   HookEventPreToolUse,
		Project: project,
		Action:  AgentAction{Action: "write_file", Path: "go.sum", Message: "edit lockfile"},
	}

	results, err := RunHooksV2(context.Background(), project, cfg, defs, payload)
	if err == nil {
		t.Fatalf("expected veto error on exit code 2, got nil (results=%+v)", results)
	}
	vetoErr, ok := IsHookVetoError(err)
	if !ok {
		t.Fatalf("expected *HookVetoError, got %T: %v", err, err)
	}
	if vetoErr.ExitCode != HookVetoExitCode {
		t.Fatalf("expected ExitCode=%d, got %d", HookVetoExitCode, vetoErr.ExitCode)
	}
	if !strings.Contains(vetoErr.Reason, "lockfile forbidden") {
		t.Fatalf("expected veto reason to include stderr message, got %q", vetoErr.Reason)
	}
	if len(results) != 1 || !results[0].Blocked {
		t.Fatalf("expected RunHooksV2 to stop on first veto with 1 blocked result, got %+v", results)
	}
	if _, statErr := os.Stat(markerFile); statErr == nil {
		t.Fatal("second hook must not run after first hook vetoes with exit code 2")
	}

	// Verify backwards-compatible runAgentToolHook also surfaces exit-code-2 veto!
	cfg.HookBeforeTool = vetoCmd
	_, legacyErr := runAgentToolHook(context.Background(), project, cfg, "before", payload.Action)
	if _, isVeto := IsHookVetoError(legacyErr); !isVeto {
		t.Fatalf("expected legacy runAgentToolHook to return *HookVetoError on exit code 2, got %v", legacyErr)
	}
}

func TestHooksV2LifecycleEventsAllowNonToolPayloads(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.CommandTimeout = 15

	var receivedEvents []HookEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p HookPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedEvents = append(receivedEvents, p.Event)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"decision":"allow"}`)
	}))
	defer srv.Close()

	events := []HookEvent{
		HookEventOnTestFail,
		HookEventStop,
		HookEventSubagentStop,
	}
	for _, ev := range events {
		def := HookDefinition{
			Name:  "lifecycle-" + string(ev),
			Event: ev,
			Type:  "http",
			URL:   srv.URL,
		}
		res, err := RunHookV2(context.Background(), project, cfg, def, HookPayload{
			Event:      ev,
			Project:    project,
			Action:     AgentAction{Action: "finish", Message: "done"},
			SubagentID: "sub-42",
		})
		if err != nil {
			t.Fatalf("event %s failed: %v", ev, err)
		}
		if res.Blocked {
			t.Fatalf("event %s unexpectedly blocked: %+v", ev, res)
		}
	}
	if len(receivedEvents) != len(events) {
		t.Fatalf("expected %d lifecycle events delivered, got %d: %v", len(events), len(receivedEvents), receivedEvents)
	}
}

func TestHooksV2LoopbackHTTPHookAllowVetoAndSSRF(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.CommandTimeout = 15

	var allowHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p HookPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/allow":
			allowHits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"decision":"allow","reason":"ok"}`)
		case "/veto-json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"decision":"block","reason":"tests must pass before commit"}`)
		case "/veto-403":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"reason":"forbidden by local policy server"}`)
		case "/redirect-ssrf":
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	action := AgentAction{Action: "write_file", Path: "main.go", Message: "edit"}

	// 1. Loopback allow.
	res, err := RunHookV2(context.Background(), project, cfg, HookDefinition{
		Name:  "http-allow",
		Event: HookEventPreToolUse,
		Type:  "http",
		URL:   srv.URL + "/allow",
	}, HookPayload{Event: HookEventPreToolUse, Project: project, Action: action})
	if err != nil || res.Blocked || allowHits.Load() != 1 {
		t.Fatalf("expected loopback HTTP allow, got res=%+v err=%v hits=%d", res, err, allowHits.Load())
	}

	// 2. Loopback JSON veto.
	res, err = RunHookV2(context.Background(), project, cfg, HookDefinition{
		Name:  "http-veto-json",
		Event: HookEventPreToolUse,
		Type:  "http",
		URL:   srv.URL + "/veto-json",
	}, HookPayload{Event: HookEventPreToolUse, Project: project, Action: action})
	veto, ok := IsHookVetoError(err)
	if !ok || !res.Blocked || !strings.Contains(veto.Reason, "tests must pass") {
		t.Fatalf("expected JSON veto error, got res=%+v err=%v", res, err)
	}

	// 3. Loopback HTTP 403 veto.
	res, err = RunHookV2(context.Background(), project, cfg, HookDefinition{
		Name:  "http-veto-403",
		Event: HookEventPreToolUse,
		Type:  "http",
		URL:   srv.URL + "/veto-403",
	}, HookPayload{Event: HookEventPreToolUse, Project: project, Action: action})
	veto, ok = IsHookVetoError(err)
	if !ok || !res.Blocked || !strings.Contains(veto.Reason, "forbidden by local policy server") {
		t.Fatalf("expected 403 veto error, got res=%+v err=%v", res, err)
	}

	// 4. Redirect to metadata IP must be blocked.
	_, err = RunHookV2(context.Background(), project, cfg, HookDefinition{
		Name:  "http-redirect",
		Event: HookEventPreToolUse,
		Type:  "http",
		URL:   srv.URL + "/redirect-ssrf",
	}, HookPayload{Event: HookEventPreToolUse, Project: project, Action: action})
	if err == nil || !strings.Contains(err.Error(), "SSRF") {
		t.Fatalf("expected SSRF redirect block, got err=%v", err)
	}

	// 5. Non-loopback URLs must be rejected immediately by SSRF validation.
	forbiddenURLs := []string{
		"http://example.com/hook",
		"https://evil.attacker.org/exfil",
		"http://10.0.0.5:8080/hook",
		"http://192.168.1.1/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://0.0.0.0:8080/hook",
		"http://user:pass@127.0.0.1:8080/hook",
		"file:///etc/passwd",
	}
	for _, bad := range forbiddenURLs {
		if _, err := ValidateLoopbackHookURL(bad); err == nil {
			t.Fatalf("expected ValidateLoopbackHookURL to reject %q", bad)
		}
		if _, err := RunHookV2(context.Background(), project, cfg, HookDefinition{
			Name:  "ssrf-bad",
			Event: HookEventPreToolUse,
			Type:  "http",
			URL:   bad,
		}, HookPayload{Event: HookEventPreToolUse, Project: project, Action: action}); err == nil {
			t.Fatalf("expected RunHookV2 to reject non-loopback URL %q", bad)
		}
	}
}

func TestLoadProjectHooksV2(t *testing.T) {
	project := t.TempDir()
	hookDir := filepath.Join(project, ".localcode")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"hooks":[{"name":"lint-on-edit","event":"OnEdit","type":"command","command":"echo lint","tool_matcher":"write_*"}]}`
	if err := os.WriteFile(filepath.Join(hookDir, "hooks.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	defs, err := LoadProjectHooksV2(project)
	if err != nil {
		t.Fatalf("LoadProjectHooksV2 failed: %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "lint-on-edit" || defs[0].Event != HookEventOnEdit {
		t.Fatalf("unexpected loaded hooks: %+v", defs)
	}
}
