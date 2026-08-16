// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImageGenerationValidationMatrix(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.ImageGeneratorProvider = "automatic1111"
	cfg.ImageGeneratorURL = "http://127.0.0.1:7860/api/ignored?x=1#fragment"

	cases := []struct {
		name    string
		cfg     Config
		path    string
		prompt  string
		width   int
		height  int
		wantErr string
	}{
		{"empty prompt", cfg, "out.png", " ", 0, 0, "non-empty prompt"},
		{"long prompt", cfg, "out.png", strings.Repeat("x", (16<<10)+1), 0, 0, "16 KiB"},
		{"disabled", func() Config { c := cfg; c.ImageGeneratorProvider = "disabled"; return c }(), "out.png", "cat", 0, 0, "disabled"},
		{"unsupported provider", func() Config { c := cfg; c.ImageGeneratorProvider = "other"; return c }(), "out.png", "cat", 0, 0, "unsupported"},
		{"bad scheme", func() Config { c := cfg; c.ImageGeneratorURL = "file:///tmp/x"; return c }(), "out.png", "cat", 0, 0, "http or https"},
		{"credentials", func() Config { c := cfg; c.ImageGeneratorURL = "http://user:pass@127.0.0.1:7860"; return c }(), "out.png", "cat", 0, 0, "credentials"},
		{"remote host", func() Config { c := cfg; c.ImageGeneratorURL = "http://192.168.1.10:7860"; return c }(), "out.png", "cat", 0, 0, "localhost"},
		{"bad extension", cfg, "out.txt", "cat", 0, 0, "destination"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateImageGenerationRequest(project, tc.cfg, tc.path, tc.prompt, tc.width, tc.height)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err=%v want marker %q", err, tc.wantErr)
			}
		})
	}

	defaultCfg := cfg
	defaultCfg.ImageGeneratorProvider = ""
	defaultCfg.ImageGeneratorURL = ""
	plan, err := validateImageGenerationRequest(project, defaultCfg, "images/out.webp", "  local cat  ", 1, 5000)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Provider != "automatic1111" || plan.Endpoint != "http://127.0.0.1:7860/sdapi/v1/txt2img" || plan.Width != 64 || plan.Height != 2048 || plan.Format != "webp" || plan.Prompt != "local cat" {
		t.Fatalf("unexpected image plan: %#v", plan)
	}
	plan, err = validateImageGenerationRequest(project, cfg, "out.jpeg", "cat", 0, 0)
	if err != nil || plan.Width != 512 || plan.Height != 512 || plan.Format != "jpeg" || strings.Contains(plan.Endpoint, "?") || strings.Contains(plan.Endpoint, "#") {
		t.Fatalf("normalized image plan=%#v err=%v", plan, err)
	}

	for _, tc := range []struct {
		w, h       int
		wantW, wantH int
	}{
		{0, 0, 512, 512},
		{1, 2, 64, 64},
		{4096, 4096, 2048, 2048},
		{640, 480, 640, 480},
	} {
		w, h := normalizeGeneratedImageDimensions(tc.w, tc.h)
		if w != tc.wantW || h != tc.wantH {
			t.Fatalf("normalizeGeneratedImageDimensions(%d,%d)=%d,%d want %d,%d", tc.w, tc.h, w, h, tc.wantW, tc.wantH)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLoopbackHost(host) {
			t.Fatalf("expected loopback host %q", host)
		}
	}
	for _, host := range []string{"", "example.com", "192.168.1.2"} {
		if isLoopbackHost(host) {
			t.Fatalf("unexpected loopback host %q", host)
		}
	}
}

func TestToolCommandRewritePureBranches(t *testing.T) {
	for _, tc := range []struct {
		in        string
		head, rest string
		ok        bool
	}{
		{"", "", "", false},
		{"go\ntest", "", "", false},
		{"& go test", "", "", false},
		{"| go", "", "", false},
		{". script.ps1", "", "", false},
		{`"C:\Program Files\Go\bin\go.exe" test ./...`, `C:\Program Files\Go\bin\go.exe`, "test ./...", true},
		{`'C:\Program Files\Git\bin\git.exe' status`, `C:\Program Files\Git\bin\git.exe`, "status", true},
		{`"unterminated`, "", "", false},
		{"go test ./...", "go", "test ./...", true},
		{"go", "go", "", true},
		{"go;rm", "", "", false},
	} {
		head, rest, ok := splitCommandHead(tc.in)
		if head != tc.head || rest != tc.rest || ok != tc.ok {
			t.Fatalf("splitCommandHead(%q)=(%q,%q,%v) want (%q,%q,%v)", tc.in, head, rest, ok, tc.head, tc.rest, tc.ok)
		}
	}

	project := t.TempDir()
	cfg := defaultConfig()
	cfg.AutoDiscoverTools = false
	if got, detail, err := rewriteKnownToolCommand(project, "go test ./...", cfg, "powershell"); err != nil || got != "go test ./..." || detail != "" {
		t.Fatalf("disabled rewrite got=%q detail=%q err=%v", got, detail, err)
	}
	cfg.AutoDiscoverTools = true
	for _, command := range []string{"go; echo bad", `C:\go\bin\go.exe test`, "unknown-tool arg"} {
		got, detail, err := rewriteKnownToolCommand(project, command, cfg, "powershell")
		if err != nil || got != command || detail != "" {
			t.Fatalf("unchanged rewrite %q -> %q detail=%q err=%v", command, got, detail, err)
		}
	}

	tools := t.TempDir()
	fakeGo := filepath.Join(tools, "go-fixture.exe")
	if err := os.WriteFile(fakeGo, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.ToolOverrides = map[string]string{"go": fakeGo}
	for _, shell := range []string{"powershell", "cmd", "wsl"} {
		got, detail, err := rewriteKnownToolCommand(project, "go test ./...", cfg, shell)
		if err != nil || !strings.Contains(got, fakeGo) || !strings.Contains(got, "test ./...") || !strings.Contains(detail, "go") {
			t.Fatalf("shell=%s got=%q detail=%q err=%v", shell, got, detail, err)
		}
	}

	missingCfg := cfg
	missingCfg.ToolOverrides = map[string]string{"go": filepath.Join(tools, "missing-go.exe")}
	_, detail, err := rewriteKnownToolCommand(project, "go test ./...", missingCfg, "powershell")
	if err == nil || !strings.Contains(strings.ToLower(detail), "nicht gefunden") {
		t.Fatalf("missing tool detail=%q err=%v", detail, err)
	}
}

func TestSkillConflictAndActivationPolicyBranches(t *testing.T) {
	base := localSkillSummary{Name: "x", Path: "z", RootTier: 2, Priority: 1, Relevant: false, RootRank: 2}
	cases := []struct {
		candidate localSkillSummary
		want      bool
	}{
		{localSkillSummary{Name: "x", Path: "a", RootTier: 1, Priority: 0, RootRank: 9}, true},
		{localSkillSummary{Name: "x", Path: "a", RootTier: 3, Priority: 9, RootRank: 0}, false},
		{localSkillSummary{Name: "x", Path: "a", RootTier: 2, Priority: 2, RootRank: 9}, true},
		{localSkillSummary{Name: "x", Path: "a", RootTier: 2, Priority: 0, RootRank: 0}, false},
		{localSkillSummary{Name: "x", Path: "a", RootTier: 2, Priority: 1, Relevant: true, RootRank: 9}, true},
		{localSkillSummary{Name: "x", Path: "a", RootTier: 2, Priority: 1, Relevant: false, RootRank: 1}, true},
		{localSkillSummary{Name: "x", Path: "zz", RootTier: 2, Priority: 1, Relevant: false, RootRank: 2}, false},
		{localSkillSummary{Name: "x", Path: "a", RootTier: 2, Priority: 1, Relevant: false, RootRank: 2}, true},
	}
	for _, tc := range cases {
		if got := skillConflictWinner(tc.candidate, base); got != tc.want {
			t.Fatalf("skillConflictWinner(%#v,%#v)=%v want %v", tc.candidate, base, got, tc.want)
		}
	}
	resolved := resolveSkillConflicts([]localSkillSummary{
		{Name: "Build", Path: "later", RootTier: 2, Priority: 1},
		{Name: "build", Path: "winner", RootTier: 1, Priority: 1},
		{Name: "Other", Path: "other", RootTier: 1},
	})
	if len(resolved) != 2 {
		t.Fatalf("resolved skills=%#v", resolved)
	}
	var foundWinner bool
	for _, skill := range resolved {
		if strings.EqualFold(skill.Name, "build") && skill.Path == "winner" {
			foundWinner = true
		}
	}
	if !foundWinner {
		t.Fatalf("expected conflict winner in %#v", resolved)
	}

	if !activationMatchesTask("Fix the Android Gradle build", []string{"", "android build"}) {
		t.Fatal("direct activation match expected")
	}
	if !activationMatchesTask("Investigate authentication parser", []string{"security authentication workflow"}) {
		t.Fatal("significant-word activation match expected")
	}
	if activationMatchesTask("Update CSS colors", []string{"database migration", ""}) {
		t.Fatal("unexpected activation match")
	}
}
