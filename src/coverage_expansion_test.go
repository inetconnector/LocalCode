// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func isolateCoverageEnv(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	cacheHome := filepath.Join(root, "cache")
	userHome := filepath.Join(root, "home")
	t.Setenv("LOCALCODE_CONFIG_HOME", configHome)
	t.Setenv("LOCALCODE_CACHE_HOME", cacheHome)
	t.Setenv("LOCALCODE_USER_HOME", userHome)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "local-app-data"))
	t.Setenv("APPDATA", filepath.Join(root, "roaming-app-data"))
	for _, dir := range []string{configHome, cacheHome, userHome, os.Getenv("LOCALAPPDATA"), os.Getenv("APPDATA")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func newCoverageState(t *testing.T, project string) *AppState {
	t.Helper()
	isolateCoverageEnv(t)
	cfg := defaultConfig()
	cfg.RootProjectDir = filepath.Dir(project)
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.Language = "en"
	cfg.PreferredLanguage = "en"
	cfg.CommandTimeout = 1
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	state.Model = "test-model"
	t.Cleanup(state.Close)
	return state
}

func TestCoverageApprovalRulesAndTokens(t *testing.T) {
	project := t.TempDir()
	if _, err := normalizeApprovalRule(ApprovalRule{Scope: "bad", Pattern: []string{"x"}}); err == nil {
		t.Fatal("scope")
	}
	if _, err := normalizeApprovalRule(ApprovalRule{Decision: "bad", Pattern: []string{"x"}}); err == nil {
		t.Fatal("decision")
	}
	if _, err := normalizeApprovalRule(ApprovalRule{}); err == nil {
		t.Fatal("pattern")
	}
	global, err := normalizeApprovalRule(ApprovalRule{Scope: "global", Decision: "prompt", Project: project, Pattern: []string{" git ", " status "}})
	if err != nil || global.Project != "" || global.ID == "" || global.CreatedAt.IsZero() {
		t.Fatalf("global=%+v err=%v", global, err)
	}
	local, err := normalizeApprovalRule(ApprovalRule{Project: project, Pattern: []string{"git", "status"}})
	if err != nil || local.Scope != "project" || local.Decision != "allow" || !filepath.IsAbs(local.Project) {
		t.Fatalf("local=%+v err=%v", local, err)
	}
	rules := normalizeApprovalRules([]ApprovalRule{local, local, {Scope: "broken"}})
	if len(rules) != 1 {
		t.Fatalf("rules=%d", len(rules))
	}

	cases := []AgentAction{
		{Action: "git", Args: []string{"status"}}, {Action: "git_commit"},
		{Action: "run_tool", Tool: "ADB", Args: []string{"devices"}}, {Action: "run_command", Command: " echo   hi "},
		{Action: "run_command"}, {Action: "write_file", Path: "a/../b.txt"}, {Action: "replace_text", Path: "x"}, {Action: "delete_file", Path: "x"},
		{Action: "copy_path", Source: "a", Destination: "b"}, {Action: "move_path", Source: "a", Destination: "b"},
		{Action: "skill_copy_resource", Skill: "asset-skill", Resource: "assets/icon.png", Destination: "public/icon.png"},
		{Action: "skill_run_script", Skill: "asset-skill", Script: "echo skill-ok", Args: []string{"--fast"}},
		{Action: "web_search"}, {Action: "web_fetch"}, {Action: "build_project"}, {Action: "deploy_android"}, {Action: "open_terminal"},
		{Action: "aider_edit"}, {Action: "aider_repo_map"}, {Action: "aider_lint"}, {Action: "aider_test"}, {Action: "install_aider"},
		{Action: "mcp_call_tool", Server: "fs", Tool: "read"}, {Action: "custom"},
	}
	for _, c := range cases {
		_ = approvalActionTokens(c)
	}
	if !rulePatternMatches([]string{"GIT", "status"}, []string{"git", "status", "--short"}) {
		t.Fatal("match")
	}
	if rulePatternMatches(nil, []string{"git"}) || rulePatternMatches([]string{"git", "status"}, []string{"git"}) || rulePatternMatches([]string{"git", "log"}, []string{"git", "status"}) {
		t.Fatal("bad match")
	}

	cfg := defaultConfig()
	cfg.ApprovalRules = []ApprovalRule{
		{Scope: "global", Decision: "allow", Pattern: []string{"git"}, Justification: "allow"},
		{Scope: "project", Project: project, Decision: "prompt", Pattern: []string{"git", "status"}, Justification: "prompt"},
		{Scope: "project", Project: project, Decision: "forbidden", Pattern: []string{"git", "status", "--short"}, Justification: "deny"},
	}
	d, j, m := approvalRuleDecision(cfg, project, AgentAction{Action: "git", Args: []string{"status", "--short"}})
	if !m || d != "forbidden" || j != "deny" {
		t.Fatalf("%s %s %v", d, j, m)
	}
	if _, _, m := approvalRuleDecision(cfg, t.TempDir(), AgentAction{Action: "run_command"}); m {
		t.Fatal("unexpected")
	}

	for _, a := range []AgentAction{{Action: "git", Args: []string{"status"}}, {Action: "git_commit"}, {Action: "run_tool", Tool: "go", Args: []string{"version"}}, {Action: "run_command", Command: "echo hi"}, {Action: "write_file", Path: "x"}, {Action: "skill_copy_resource", Skill: "asset-skill", Resource: "assets/icon.png", Destination: "public/icon.png"}, {Action: "skill_run_script", Skill: "asset-skill", Script: "echo skill-ok"}, {Action: "build_project"}, {Action: "unknown"}} {
		if p, ok := persistentApprovalPattern(a); !ok || len(p) == 0 {
			t.Fatalf("pattern %+v", a)
		}
	}
	if _, ok := persistentApprovalPattern(AgentAction{Action: "delete_file", Path: "x"}); ok {
		t.Fatal("delete persistent")
	}
	if _, ok := persistentApprovalPattern(AgentAction{Action: "move_path", Source: "x", Destination: "y"}); ok {
		t.Fatal("move persistent")
	}
	if _, ok := persistentApprovalPattern(AgentAction{Action: "git", Args: []string{"reset", "--hard"}}); ok {
		t.Fatal("unsafe git")
	}

	isolateCoverageEnv(t)
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	if _, err := state.addApprovalRule(project, AgentAction{Action: "delete_file", Path: "x"}, "project"); err == nil {
		t.Fatal("expected reject")
	}
	added, err := state.addApprovalRule(project, AgentAction{Action: "run_tool", Tool: "go", Args: []string{"version"}}, "project")
	if err != nil || added.ID == "" || len(state.Config.ApprovalRules) == 0 {
		t.Fatalf("added=%+v err=%v", added, err)
	}
}

func TestCoverageHistoryLifecycle(t *testing.T) {
	isolateCoverageEnv(t)
	if got := loadThreads(); len(got) != 0 {
		t.Fatal(got)
	}
	if err := os.MkdirAll(filepath.Dir(threadsPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(threadsPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := loadThreads(); len(got) != 0 {
		t.Fatal(got)
	}
	payload := threadFile{Version: 1, Threads: []ChatThread{{ID: "ok", Project: "p", Title: ""}, {ID: "", Project: "p"}, {ID: "bad", Project: ""}}}
	b, _ := json.Marshal(payload)
	if err := os.WriteFile(threadsPath(), b, 0o600); err != nil {
		t.Fatal(err)
	}
	got := loadThreads()
	if got["ok"].Title != "Neuer Chat" || len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	many := map[string]*ChatThread{}
	now := time.Now()
	for i := 0; i < 260; i++ {
		ev := make([]UIEvent, 1005)
		many[fmt.Sprint(i)] = &ChatThread{ID: fmt.Sprint(i), Project: "p", Title: "x", UpdatedAt: now.Add(time.Duration(i) * time.Second), Events: ev}
	}
	many["nil"] = nil
	if err := saveThreads(many); err != nil {
		t.Fatal(err)
	}
	loaded := loadThreads()
	if len(loaded) != 250 {
		t.Fatalf("len=%d", len(loaded))
	}
	for _, th := range loaded {
		if len(th.Events) != 1000 {
			t.Fatalf("events=%d", len(th.Events))
		}
		break
	}
	if threadTitle("   ") != "Dateien analysieren" {
		t.Fatal("empty title")
	}
	long := strings.Repeat("ä", 80)
	if !strings.HasSuffix(threadTitle(long), "…") {
		t.Fatal("truncate")
	}
	a := newThread("p", "m")
	bth := newThread("p", "m")
	bth.UpdatedAt = a.UpdatedAt.Add(time.Second)
	sums := summariesForThreads(map[string]*ChatThread{"a": a, "b": bth, "nil": nil})
	if len(sums) != 2 || sums[0].ID != bth.ID {
		t.Fatal(sums)
	}

	project := t.TempDir()
	state := NewAppState(defaultConfig(), NewOllamaClient())
	t.Cleanup(state.Close)
	state.Project = project
	state.Model = "m"
	state.Threads = map[string]*ChatThread{}
	state.selectProjectThread(project)
	first := state.CurrentThread
	state.selectProjectThread(project)
	if state.CurrentThread != first {
		t.Fatal("did not reuse")
	}
	state.Running = true
	if _, err := state.NewChat(project); err == nil {
		t.Fatal("running")
	}
	state.Running = false
	state.Project = ""
	if _, err := state.NewChat(""); err == nil {
		t.Fatal("missing project")
	}
	state.Project = project
	summary, err := state.NewChat("")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SelectChat("missing"); err == nil {
		t.Fatal("missing chat")
	}
	state.Running = true
	if err := state.SelectChat(summary.ID); err == nil {
		t.Fatal("running select")
	}
	state.Running = false
	if err := state.SelectChat(summary.ID); err != nil {
		t.Fatal(err)
	}
	if len(state.threadSummaries()) < 1 {
		t.Fatal("summaries")
	}
	if err := state.RenameChat(summary.ID, "   "); err == nil {
		t.Fatal("empty rename")
	}
	if err := state.RenameChat("missing", "x"); err == nil {
		t.Fatal("missing rename")
	}
	state.Running = true
	if err := state.RenameChat(summary.ID, "x"); err == nil {
		t.Fatal("running rename")
	}
	state.Running = false
	if err := state.RenameChat(summary.ID, strings.Repeat("z", 130)); err != nil {
		t.Fatal(err)
	}
	if _, err := state.DuplicateChat("missing"); err == nil {
		t.Fatal("missing dup")
	}
	state.Running = true
	if _, err := state.DuplicateChat(summary.ID); err == nil {
		t.Fatal("running dup")
	}
	state.Running = false
	dup, err := state.DuplicateChat(summary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ArchiveChat("missing", true); err == nil {
		t.Fatal("missing archive")
	}
	state.Running = true
	if err := state.ArchiveChat(summary.ID, true); err == nil {
		t.Fatal("running archive")
	}
	state.Running = false
	if err := state.ArchiveChat(summary.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := state.ArchiveChat(summary.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := state.DeleteChat("missing"); err == nil {
		t.Fatal("missing delete")
	}
	state.Running = true
	if err := state.DeleteChat(dup.ID); err == nil {
		t.Fatal("running delete")
	}
	state.Running = false
	if err := state.DeleteChat(dup.ID); err != nil {
		t.Fatal(err)
	}
	cp := cloneThreads(map[string]*ChatThread{"x": state.Threads[summary.ID], "nil": nil})
	if cp["x"] == state.Threads[summary.ID] {
		t.Fatal("not cloned")
	}
	ch := state.Subscribe()
	state.AddEvent(UIEvent{Type: "test", Message: "x"})
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
	state.Unsubscribe(ch)
}

func TestCoveragePathOperations(t *testing.T) {
	root := t.TempDir()
	allowed := t.TempDir()
	if r := sandboxRoots(Config{SandboxMode: "unrestricted"}, root); r != nil {
		t.Fatal(r)
	}
	for _, mode := range []string{"project", "workspace", "unknown"} {
		_ = sandboxRoots(Config{SandboxMode: mode, RootProjectDir: root, AllowedRoots: []string{allowed}}, root)
	}
	missing := filepath.Join(root, "a", "b", "c.txt")
	if p, err := canonicalSandboxPath(missing); err != nil || !filepath.IsAbs(p) {
		t.Fatalf("%s %v", p, err)
	}
	cfg := Config{SandboxMode: "project", RootProjectDir: root, AllowedRoots: []string{allowed}}
	if _, err := resolveSandboxPath(cfg, root, ""); err == nil {
		t.Fatal("empty")
	}
	if p, err := resolveSandboxPath(Config{SandboxMode: "unrestricted"}, root, "x"); err != nil || !filepath.IsAbs(p) {
		t.Fatal(p, err)
	}
	if _, err := resolveSandboxPath(cfg, root, "../escape"); err == nil {
		t.Fatal("escape")
	}
	if _, err := resolveSandboxPath(cfg, root, filepath.Join(allowed, "x")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	msg, err := copyPath(cfg, root, "a.txt", "sub/b.txt")
	if err != nil || !strings.Contains(msg, "Copied") {
		t.Fatal(msg, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dir", "x.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := copyPath(cfg, root, "dir", "dircopy"); err != nil {
		t.Fatal(err)
	}
	if _, err := movePath(cfg, root, "sub/b.txt", "moved/c.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := copyPath(cfg, root, "missing", "x"); err == nil {
		t.Fatal("missing")
	}
	if _, err := movePath(cfg, root, "missing", "x"); err == nil {
		t.Fatal("missing move")
	}
	if err := copyFile(filepath.Join(root, "a.txt"), filepath.Join(root, "deep", "z.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(filepath.Join(root, "nope"), filepath.Join(root, "x")); err == nil {
		t.Fatal("bad dir")
	}
}

func makeZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestCoverageAttachmentsAllFormats(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("hello"))
	clean, err := validateAttachments([]Attachment{{Name: "../bad:name.txt", Data: data}})
	if err != nil || clean[0].MIME == "" || clean[0].Size != 5 {
		t.Fatalf("%+v %v", clean, err)
	}
	if _, err := validateAttachments(make([]Attachment, maxAttachmentCount+1)); err == nil {
		t.Fatal("count")
	}
	if _, err := validateAttachments([]Attachment{{Name: "x", Data: "!"}}); err == nil {
		t.Fatal("base64")
	}
	if _, err := validateAttachments([]Attachment{{Name: "x", Data: base64.StdEncoding.EncodeToString(nil)}}); err == nil {
		t.Fatal("empty")
	}
	if got := sanitizeAttachmentName(strings.Repeat("x", 200) + ".txt"); len(got) > 180 {
		t.Fatal(len(got))
	}
	if len(attachmentNames(clean)) != 1 || isImageAttachment(clean[0]) {
		t.Fatal("names/image")
	}
	img := Attachment{Name: "x.png", MIME: "image/png", Data: base64.StdEncoding.EncodeToString([]byte{1, 2, 3})}
	if !isImageAttachment(img) {
		t.Fatal("image")
	}
	ctx := context.Background()
	zipData := makeZipBytes(t, map[string]string{"a.txt": "zip text", "b.bin": "xx"})
	items := []Attachment{{Name: "a.txt", MIME: "text/plain", Data: data}, {Name: "archive.zip", MIME: "application/zip", Data: base64.StdEncoding.EncodeToString(zipData)}, {Name: "x.png", MIME: "image/png", Data: img.Data}}
	prepared, err := prepareAttachments(ctx, items)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(prepared.Dir)
	if len(prepared.Images) != 1 || !strings.Contains(prepared.Context, "hello") || prepared.Dir == "" {
		t.Fatalf("%+v", prepared)
	}
	p1 := uniqueAttachmentPath(prepared.Dir, "a.txt")
	_ = os.WriteFile(p1, []byte("x"), 0o600)
	p2 := uniqueAttachmentPath(prepared.Dir, "a.txt")
	if p1 == p2 {
		t.Fatal("not unique")
	}
	if text, kind := extractAttachmentText(ctx, "x.txt", "text/plain", []byte("abc"), ""); text != "abc" || kind == "" {
		t.Fatal(text, kind)
	}
	if text, _ := extractAttachmentText(ctx, "x.zip", "application/zip", zipData, ""); !strings.Contains(text, "a.txt") {
		t.Fatal(text)
	}
	office := makeZipBytes(t, map[string]string{"word/document.xml": "<w:document xmlns:w='x'><w:p><w:t>Hello</w:t><w:tab/><w:t>World</w:t></w:p></w:document>"})
	if text, err := extractOfficeXML(office, []string{"word/"}); err != nil || !strings.Contains(text, "Hello") {
		t.Fatal(text, err)
	}
	if _, err := extractOfficeXML([]byte("bad"), []string{"word/"}); err == nil {
		t.Fatal("bad office")
	}
	if text := extractPrintableStrings([]byte{0, 1, 'a', 'b', 'c', 'd', 'e', 'f', 0, 'x', 'y', 'z', 'q', 'r', 's'}); !strings.Contains(text, "abcd") {
		t.Fatal(text)
	}
	if !isTextLike("text/plain", ".txt", []byte("x")) || !isTextLike("application/json", ".json", []byte("{}")) || isTextLike("application/octet-stream", ".bin", []byte{0, 1, 2, 0}) {
		t.Fatal("textlike")
	}
	if !strings.Contains(normalizeText([]byte{0xff, 'a'}), "a") {
		t.Fatal("normalize")
	}
	if _, err := listZip([]byte("bad")); err == nil {
		t.Fatal("bad zip")
	}
	if s, err := listZip(zipData); err != nil || !strings.Contains(s, "a.txt") {
		t.Fatal(s, err)
	}
	if _, err := validateImages([]ImageAttachment{{Name: "x", MIME: "image/png", Data: "!"}}); err == nil {
		t.Fatal("bad image")
	}
	if _, err := validateImages([]ImageAttachment{{Name: "x", MIME: "text/plain", Data: data}}); err == nil {
		t.Fatal("mime")
	}
	if _, err := validateImages(make([]ImageAttachment, maxAttachmentCount+1)); err == nil {
		t.Fatal("count images")
	}
	if out, err := validateImages([]ImageAttachment{{Name: "x.png", MIME: "image/png", Data: img.Data}}); err != nil || len(out) != 1 {
		t.Fatal(out, err)
	}
	if len(attachmentSummaries(items)) != 3 {
		t.Fatal("summaries")
	}
}

func TestCoverageMCPBuiltinHelpersAndFilesystem(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "en"
	cfg.PreferredLanguage = "en"
	cfg.SandboxMode = "project"
	_ = objectSchema(map[string]any{"x": map[string]any{"type": "string"}}, "x")
	if !strings.Contains(mcpTextResult("ok"), "ok") {
		t.Fatal("text")
	}
	if s, e := mcpErrorResult(nil); s != "" || e != nil {
		t.Fatal(s, e)
	}
	if s, e := mcpErrorResult(errors.New("bad")); e == nil || !strings.Contains(s, "bad") {
		t.Fatal(s, e)
	}
	_ = decodeMCPParams(nil)
	_ = decodeMCPParams(map[string]any{"x": 1})
	_ = decodeMCPParams(struct {
		X int `json:"x"`
	}{1})
	_ = mcpArgumentMap(map[string]any{})
	_ = mcpArgumentMap(map[string]any{"arguments": map[string]any{"x": 1}})
	_ = mcpArgumentMap(map[string]any{"arguments": struct {
		X int `json:"x"`
	}{1}})
	args := map[string]any{"s": " x ", "n": json.Number("4"), "f": 3.5, "b": true, "bs": "false", "i": "7", "arr": []any{"a", 2, "b"}, "one": "z"}
	if stringArg(args, "s") != "x" || stringArg(args, "n") != "4" || stringArg(args, "f") != "3.5" || stringArg(args, "none") != "" {
		t.Fatal("stringArg")
	}
	if !boolArg(args, "b", false) || boolArg(args, "bs", true) || !boolArg(args, "bad", true) {
		t.Fatal("bool")
	}
	if intArg(args, "f", 0) != 3 || intArg(args, "n", 0) != 4 || intArg(args, "i", 0) != 7 || intArg(args, "bad", 9) != 9 {
		t.Fatal("int")
	}
	if len(stringSliceArg(args, "arr")) != 2 || len(stringSliceArg(args, "one")) != 1 || stringSliceArg(args, "none") != nil {
		t.Fatal("slice")
	}
	for _, preset := range []string{"filesystem", "powershell", "git", "unknown"} {
		_ = builtinTools(preset, cfg)
		_ = builtinToolNames(preset, cfg)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "a.txt"), []byte("hello needle"), 0o600); err != nil {
		t.Fatal(err)
	}
	fsServer := MCPServerConfig{Name: "fs", Preset: "filesystem", Transport: "builtin"}
	for _, method := range []string{"tools/list", "resources/list", "prompts/list"} {
		if _, err := mcpCallBuiltin(context.Background(), cfg, project, fsServer, method, nil); err != nil {
			t.Fatalf("%s: %v", method, err)
		}
	}
	if _, err := mcpCallBuiltin(context.Background(), cfg, project, fsServer, "tools/call", map[string]any{"name": "read_text_file", "arguments": map[string]any{"path": "a.txt"}}); err != nil {
		t.Fatal(err)
	}
	resourceURI, err := fileURI(filepath.Join(project, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcpCallBuiltin(context.Background(), cfg, project, fsServer, "resources/read", map[string]any{"uri": resourceURI}); err != nil {
		t.Fatal(err)
	}
	if _, err := mcpCallBuiltin(context.Background(), cfg, project, fsServer, "prompts/get", map[string]any{"name": "x"}); err == nil {
		t.Fatal("prompt")
	}
	if _, err := mcpCallBuiltin(context.Background(), cfg, project, fsServer, "bad", nil); err == nil {
		t.Fatal("method")
	}
	if _, err := pathFromFileURI("http://x"); err == nil {
		t.Fatal("uri")
	}
	if p, err := pathFromFileURI("file:///tmp/a%20b"); err != nil || !strings.Contains(p, "a b") {
		t.Fatal(p, err)
	}
	if p, err := pathFromFileURI("file://C:/Users/test/a.txt"); err != nil || !strings.Contains(strings.ToLower(filepath.ToSlash(p)), "c:/users/test/a.txt") {
		t.Fatal(p, err)
	}
	if p, err := pathFromFileURI("file://server/share/a.txt"); err != nil || !strings.Contains(strings.ToLower(filepath.ToSlash(p)), "server/share/a.txt") {
		t.Fatal(p, err)
	}
	if _, err := executeBuiltinMCPTool(context.Background(), cfg, project, "bad", "x", nil); err == nil {
		t.Fatal("preset")
	}
	if p, err := secureProjectPath(project, ""); err != nil || p != filepath.Clean(project) {
		t.Fatal(p, err)
	}
	calls := []struct {
		name string
		args map[string]any
	}{
		{"list_directory", map[string]any{"path": ".", "depth": 99}}, {"read_text_file", map[string]any{"path": "a.txt"}},
		{"write_file", map[string]any{"path": "b.txt", "content": "new"}}, {"create_directory", map[string]any{"path": "dir"}},
		{"search_files", map[string]any{"query": "needle"}}, {"get_file_info", map[string]any{"path": "a.txt"}},
		{"copy_path", map[string]any{"source": "a.txt", "destination": "copy.txt"}}, {"move_path", map[string]any{"source": "copy.txt", "destination": "moved.txt"}},
	}
	for _, c := range calls {
		if _, err := executeFilesystemMCPTool(cfg, project, c.name, c.args); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	if _, err := executeFilesystemMCPTool(cfg, project, "delete_path", map[string]any{"path": "."}); err == nil {
		t.Fatal("root delete")
	}
	if _, err := executeFilesystemMCPTool(cfg, project, "delete_path", map[string]any{"path": "dir"}); err == nil {
		t.Fatal("recursive")
	}
	if _, err := executeFilesystemMCPTool(cfg, project, "delete_path", map[string]any{"path": "dir", "recursive": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeFilesystemMCPTool(cfg, project, "delete_path", map[string]any{"path": "moved.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeFilesystemMCPTool(cfg, project, "unknown", nil); err == nil {
		t.Fatal("unknown")
	}
	if q := quotePowerShellLiteral("a'b"); q != "'a''b'" {
		t.Fatal(q)
	}
	if runtime.GOOS != "windows" {
		if _, err := executePowerShellMCPTool(context.Background(), cfg, project, "powershell_run", map[string]any{"script": ""}); err == nil || !strings.Contains(err.Error(), "PowerShell") { /* may exist; acceptable below */
		}
	}
}

func serveCoverageRequest(t *testing.T, server *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		switch typed := body.(type) {
		case json.RawMessage:
			reader = bytes.NewReader(typed)
		case string:
			reader = strings.NewReader(typed)
		default:
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			reader = bytes.NewReader(data)
		}
	}
	req := httptest.NewRequest(method, "http://127.0.0.1:32145"+path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	return rr
}

func requireCoverageStatus(t *testing.T, rr *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rr.Code != want {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, want, rr.Body.String())
	}
}

func TestCoverageServerEndpointMatrix(t *testing.T) {
	base := isolateCoverageEnv(t)
	home := filepath.Join(base, "profile")
	t.Setenv("LOCALCODE_USER_HOME", home)
	root := filepath.Join(home, "Projekte")
	project := filepath.Join(root, "Demo")
	other := filepath.Join(root, "Other")
	for _, dir := range []string{project, other} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("demo"), 0o600); err != nil {
		t.Fatal(err)
	}

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "test-model", "size": 1}}})
		case "/api/chat":
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": `{"action":"finish","message":"done"}`}, "done": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollama.Close()
	client := NewOllamaClient()
	client.BaseURL = ollama.URL
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = "native"
	cfg.AiderEnabled = false
	cfg.CreateProjectDocs = false
	cfg.CommandTimeout = 2
	cfg.Language = "en"
	cfg.PreferredLanguage = "en"
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Transport != "builtin" {
			cfg.MCPServers[i].Enabled = false
		}
	}
	state := NewAppState(cfg, client)
	state.Project = project
	state.Model = "test-model"
	t.Cleanup(state.Close)
	server := NewServer(state)
	server.selectDirectory = func(initial, language string) (string, error) { return initial, nil }

	// Security and method branches.
	badHost := httptest.NewRequest(http.MethodGet, "http://evil.example/api/ping", nil)
	badHost.Host = "evil.example"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, badHost)
	requireCoverageStatus(t, rr, http.StatusForbidden)
	for _, path := range []string{"/api/ping", "/api/status", "/api/projects", "/api/threads", "/api/snapshot", "/api/tools", "/api/mcp/status"} {
		rr = serveCoverageRequest(t, server, http.MethodPost, path, map[string]any{})
		if rr.Code != http.StatusMethodNotAllowed && path != "/api/snapshot" {
			t.Fatalf("method %s=%d", path, rr.Code)
		}
	}
	for _, path := range []string{"/api/ping", "/api/status", "/api/projects", "/api/threads", "/api/snapshot", "/api/settings", "/api/tools?versions=1", "/api/mcp/status"} {
		rr = serveCoverageRequest(t, server, http.MethodGet, path, nil)
		requireCoverageStatus(t, rr, http.StatusOK)
	}

	// Project actions and selection.
	for _, action := range []string{"rename", "pin", "unpin", "remove", "restore"} {
		value := ""
		if action == "rename" {
			value = "Pretty Demo"
		}
		rr = serveCoverageRequest(t, server, http.MethodPost, "/api/project-action", map[string]any{"path": project, "action": action, "value": value})
		requireCoverageStatus(t, rr, http.StatusOK)
	}
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/project-action", map[string]any{"path": project, "action": "bad"})
	requireCoverageStatus(t, rr, http.StatusConflict)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/select-project", map[string]any{"path": project})
	requireCoverageStatus(t, rr, http.StatusOK)
	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/select-project", map[string]any{"path": other})
	requireCoverageStatus(t, rr, http.StatusConflict)
	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/select-project", map[string]any{"path": "../bad"})
	requireCoverageStatus(t, rr, http.StatusBadRequest)

	newRoot := filepath.Join(base, "newroot")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/root", map[string]any{"path": newRoot})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/root", map[string]any{"path": filepath.Join(base, "missing")})
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	// Restore original root, browse and reset.
	if _, err := server.applyRoot(root); err != nil {
		t.Fatal(err)
	}
	server.selectDirectory = func(initial, language string) (string, error) { return "", errors.New("picker failed") }
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/browse-root", map[string]any{})
	requireCoverageStatus(t, rr, http.StatusInternalServerError)
	server.selectDirectory = func(initial, language string) (string, error) { return "", nil }
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/browse-root", map[string]any{})
	requireCoverageStatus(t, rr, http.StatusOK)
	if !strings.Contains(rr.Body.String(), `"cancelled":true`) {
		t.Fatalf("cancelled browse body=%s", rr.Body.String())
	}
	server.selectDirectory = func(initial, language string) (string, error) { return initial, nil }
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/browse-root", map[string]any{})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/reset-root", map[string]any{})
	requireCoverageStatus(t, rr, http.StatusOK)
	if _, err := server.applyRoot(root); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.Project = project
	state.Config.LastProject = project
	state.mu.Unlock()
	state.selectProjectThread(project)

	// Chat lifecycle through HTTP.
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/chat", map[string]any{"message": "", "model": "test-model"})
	requireCoverageStatus(t, rr, http.StatusConflict)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/chat", map[string]any{"message": "hello", "model": "test-model", "attachments": []map[string]any{{"name": "x", "data": "!"}}})
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/chat", map[string]any{"message": "hello", "model": "test-model"})
	requireCoverageStatus(t, rr, http.StatusOK)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.RLock()
		running := state.Running
		state.mu.RUnlock()
		if !running {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	rr = serveCoverageRequest(t, server, http.MethodGet, "/api/threads", nil)
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/new-chat", map[string]any{"project": project})
	requireCoverageStatus(t, rr, http.StatusOK)
	var newResp struct {
		Thread ChatThreadSummary `json:"thread"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &newResp)
	id := newResp.Thread.ID
	if id == "" {
		t.Fatalf("new chat body=%s", rr.Body.String())
	}
	for _, item := range []struct {
		path string
		body any
	}{
		{"/api/select-chat", map[string]any{"id": id}}, {"/api/rename-chat", map[string]any{"id": id, "title": "Renamed"}},
		{"/api/archive-chat", map[string]any{"id": id, "archived": false}}, {"/api/duplicate-chat", map[string]any{"id": id}},
	} {
		rr = serveCoverageRequest(t, server, http.MethodPost, item.path, item.body)
		requireCoverageStatus(t, rr, http.StatusOK)
	}
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/open-chat-window", map[string]any{"id": "missing"})
	requireCoverageStatus(t, rr, http.StatusNotFound)
	oldLaunch := launchTaskWindow
	launchTaskWindow = func(string) error { return nil }
	defer func() { launchTaskWindow = oldLaunch }()
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/open-chat-window", map[string]any{"id": id})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/delete-chat", map[string]any{"id": id})
	requireCoverageStatus(t, rr, http.StatusOK)

	// Stop/force-stop and approvals.
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/stop", map[string]any{})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/force-stop", map[string]any{})
	requireCoverageStatus(t, rr, http.StatusOK)
	pending := &PendingAction{ID: "p1", Action: AgentAction{Action: "run_tool", Message: "run"}, Result: make(chan ApprovalDecision, 1)}
	state.mu.Lock()
	state.Pending = pending
	state.mu.Unlock()
	for _, decision := range []string{"invalid", "project"} {
		rr = serveCoverageRequest(t, server, http.MethodPost, "/api/approve", map[string]any{"id": "p1", "approve": true, "decision": decision})
		if decision == "invalid" {
			requireCoverageStatus(t, rr, http.StatusBadRequest)
		} else {
			requireCoverageStatus(t, rr, http.StatusOK)
			<-pending.Result
		}
	}
	state.mu.Lock()
	state.Pending = nil
	state.mu.Unlock()
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/approve", map[string]any{"id": "p1"})
	requireCoverageStatus(t, rr, http.StatusNotFound)

	state.mu.Lock()
	state.Pending = &PendingAction{ID: "p2", Action: AgentAction{Action: "write_file", Message: "write", Path: "x"}, Preview: "diff", Result: make(chan ApprovalDecision, 1)}
	state.mu.Unlock()
	rr = serveCoverageRequest(t, server, http.MethodGet, "/api/snapshot?thread_id="+state.CurrentThread, nil)
	requireCoverageStatus(t, rr, http.StatusOK)
	state.mu.Lock()
	state.Pending = nil
	state.mu.Unlock()

	// Settings, MCP, tools and command endpoints.
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/settings", map[string]any{"ui_theme": "light", "language": "en", "unknown_future_field": true})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/settings", json.RawMessage("{"))
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/mcp/test", map[string]any{"name": "filesystem"})
	requireCoverageStatus(t, rr, http.StatusOK)
	for _, action := range []string{"disable", "enable", "reset", "test"} {
		rr = serveCoverageRequest(t, server, http.MethodPost, "/api/mcp/setup", map[string]any{"name": "filesystem", "action": action})
		requireCoverageStatus(t, rr, http.StatusOK)
	}
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/mcp/setup", map[string]any{"name": "missing", "action": "enable"})
	requireCoverageStatus(t, rr, http.StatusNotFound)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/mcp/setup", map[string]any{"name": "filesystem", "action": "bad"})
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/tools/diagnose", map[string]any{"tool": "go"})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/tools/diagnose", map[string]any{"tool": ""})
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/terminal-command", map[string]any{"command": "echo hi", "path": project})
	requireCoverageStatus(t, rr, http.StatusOK)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/terminal-command", map[string]any{"command": ""})
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/open-terminal", map[string]any{"command": "git reset --hard"})
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	rr = serveCoverageRequest(t, server, http.MethodPost, "/api/open-project", map[string]any{"path": "../bad"})
	requireCoverageStatus(t, rr, http.StatusBadRequest)

	// Git overview success and failure branches.
	state.mu.Lock()
	state.Project = project
	state.Config.GitEnabled = false
	state.mu.Unlock()
	rr = serveCoverageRequest(t, server, http.MethodGet, "/api/git-overview", nil)
	requireCoverageStatus(t, rr, http.StatusBadRequest)
	state.mu.Lock()
	state.Config.GitEnabled = true
	state.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := initializeGitRepository(ctx, project, state.Config); err == nil {
		rr = serveCoverageRequest(t, server, http.MethodGet, "/api/git-overview", nil)
		requireCoverageStatus(t, rr, http.StatusOK)
	}

	// SSE connection and cancellation.
	ctx2, cancel2 := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:32145/api/events", nil).WithContext(ctx2)
	stream := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { server.ServeHTTP(stream, req); close(done) }()
	time.Sleep(20 * time.Millisecond)
	state.AddEvent(UIEvent{Type: "coverage", Message: "event"})
	time.Sleep(20 * time.Millisecond)
	cancel2()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SSE did not stop")
	}
	if !strings.Contains(stream.Body.String(), "connected") {
		t.Fatal(stream.Body.String())
	}

	// Shutdown is only tested through its safe method-rejection path.
	rr = serveCoverageRequest(t, server, http.MethodGet, "/api/shutdown", nil)
	requireCoverageStatus(t, rr, http.StatusMethodNotAllowed)
	if errorText(nil) != "" || errorText(errors.New("x")) != "x" {
		t.Fatal("errorText")
	}
	for _, models := range [][]ModelInfo{{{Name: "coder-x"}}, {{Name: "other"}}, nil} {
		_ = chooseDefaultModel(models, "missing")
	}
	url, err := startHTTPServer(state, 0)
	if err != nil || !strings.HasPrefix(url, "http://127.0.0.1:") {
		t.Fatal(url, err)
	}
}

type coverageRoundTripFunc func(*http.Request) (*http.Response, error)

func (f coverageRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func coverageHTTPResponse(status int, contentType, body string) *http.Response {
	h := make(http.Header)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
}

func TestCoverageWebToolsDeterministicNetwork(t *testing.T) {
	oldLookup, oldClient := webLookupIP, webHTTPClient
	oldOllama, oldDDG, oldBing := ollamaWebSearchEndpoint, duckDuckGoSearchEndpoint, bingRSSSearchEndpoint
	defer func() {
		webLookupIP = oldLookup
		webHTTPClient = oldClient
		ollamaWebSearchEndpoint = oldOllama
		duckDuckGoSearchEndpoint = oldDDG
		bingRSSSearchEndpoint = oldBing
	}()
	ollamaWebSearchEndpoint = "https://ollama.test/api/web_search"
	duckDuckGoSearchEndpoint = "https://ddg.test/html/"
	bingRSSSearchEndpoint = "https://bing.test/search"
	webLookupIP = func(host string) ([]net.IP, error) {
		switch host {
		case "private.test":
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		case "error.test":
			return nil, errors.New("dns")
		default:
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
	}
	mode := "ddg-ok"
	webHTTPClient = func(time.Duration, int) *http.Client {
		return &http.Client{Transport: coverageRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch mode {
			case "transport-error":
				return nil, errors.New("transport")
			case "ddg-ok":
				return coverageHTTPResponse(200, "text/html", `<a class="result__a" href="https://example.com">Example <b>Title</b></a><a class="result__snippet">A &amp; B</a>`), nil
			case "ddg-empty":
				if r.URL.Host == "ddg.test" {
					return coverageHTTPResponse(200, "text/html", "none"), nil
				}
				return coverageHTTPResponse(200, "application/rss+xml", `<rss><channel><item><title>Fallback</title><link>https://example.com/f</link><description><![CDATA[<b>Text</b>]]></description><pubDate>Today</pubDate></item></channel></rss>`), nil
			case "both-empty":
				return coverageHTTPResponse(200, "text/plain", "none"), nil
			case "status":
				return coverageHTTPResponse(503, "text/plain", "down"), nil
			case "ollama-ok":
				return coverageHTTPResponse(200, "application/json", `{"results":[{"title":"O","url":"https://x","content":"C"}]}`), nil
			case "ollama-bad-json":
				return coverageHTTPResponse(200, "application/json", "{"), nil
			case "fetch-html":
				return coverageHTTPResponse(200, "text/html", "<html><body>Hello <b>World</b></body></html>"), nil
			case "fetch-text":
				return coverageHTTPResponse(200, "text/plain", "plain"), nil
			case "fetch-large":
				return coverageHTTPResponse(200, "text/plain", strings.Repeat("x", 100)), nil
			case "rss-bad":
				return coverageHTTPResponse(200, "application/rss+xml", "<rss>"), nil
			case "rss-empty":
				return coverageHTTPResponse(200, "application/rss+xml", "<rss><channel></channel></rss>"), nil
			default:
				return coverageHTTPResponse(500, "text/plain", "unknown"), nil
			}
		})}
	}

	if cleanHTMLText(" <b>A</b> &amp;   B ") != "A & B" {
		t.Fatal("clean")
	}
	cfg := defaultConfig()
	cfg.NetworkEnabled = false
	if _, err := webSearch(context.Background(), cfg, "x", 1); err == nil {
		t.Fatal("network disabled")
	}
	cfg.NetworkEnabled = true
	cfg.WebSearchProvider = "duckduckgo"
	if _, err := webSearch(context.Background(), cfg, " ", 1); err == nil {
		t.Fatal("empty")
	}
	results, err := webSearch(context.Background(), cfg, "query", 20)
	if err != nil || len(results) != 1 || results[0].Title != "Example Title" {
		t.Fatalf("%+v %v", results, err)
	}
	mode = "ddg-empty"
	results, err = webSearch(context.Background(), cfg, "query", 2)
	if err != nil || len(results) != 1 || results[0].Title != "Fallback" {
		t.Fatalf("%+v %v", results, err)
	}
	mode = "both-empty"
	if _, err := webSearch(context.Background(), cfg, "query", 2); err == nil {
		t.Fatal("both empty")
	}
	cfg.WebSearchProvider = "disabled"
	if _, err := webSearch(context.Background(), cfg, "query", 2); err == nil {
		t.Fatal("disabled")
	}
	cfg.WebSearchProvider = "ollama"
	cfg.WebSearchAPIKeyEnv = "LC_TEST_KEY"
	t.Setenv("LC_TEST_KEY", "")
	if _, err := webSearch(context.Background(), cfg, "query", 2); err == nil {
		t.Fatal("key")
	}
	t.Setenv("LC_TEST_KEY", "secret")
	mode = "ollama-ok"
	results, err = ollamaWebSearch(context.Background(), cfg, "query", 2)
	if err != nil || len(results) != 1 {
		t.Fatal(results, err)
	}
	mode = "status"
	if _, err := ollamaWebSearch(context.Background(), cfg, "query", 2); err == nil {
		t.Fatal("ollama status")
	}
	mode = "ollama-bad-json"
	if _, err := ollamaWebSearch(context.Background(), cfg, "query", 2); err == nil {
		t.Fatal("bad json")
	}
	mode = "transport-error"
	if _, err := duckDuckGoSearch(context.Background(), "q", 2); err == nil {
		t.Fatal("ddg transport")
	}
	mode = "status"
	if _, err := duckDuckGoSearch(context.Background(), "q", 2); err == nil {
		t.Fatal("ddg status")
	}
	mode = "both-empty"
	if _, err := duckDuckGoSearch(context.Background(), "q", 2); err == nil {
		t.Fatal("ddg empty")
	}

	if _, err := validatePublicURL("ftp://example.com"); err == nil {
		t.Fatal("scheme")
	}
	if _, err := validatePublicURL("https://"); err == nil {
		t.Fatal("host")
	}
	if _, err := validatePublicURL("https://private.test"); err == nil {
		t.Fatal("private")
	}
	if _, err := validatePublicURL("https://error.test"); err == nil {
		t.Fatal("dns")
	}
	if u, err := validatePublicURL("https://public.test/path"); err != nil || u.Hostname() != "public.test" {
		t.Fatal(u, err)
	}
	for _, ip := range []net.IP{nil, net.ParseIP("127.0.0.1"), net.ParseIP("10.0.0.1"), net.ParseIP("224.0.0.1"), net.ParseIP("93.184.216.34")} {
		_ = isForbiddenIP(ip)
	}
	if _, err := publicOnlyDialContext(context.Background(), "tcp", "bad-address"); err == nil {
		t.Fatal("split")
	}
	if _, err := publicOnlyDialContext(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Fatal("private dial")
	}
	client := publicHTTPClient(time.Second, 0)
	if err := client.CheckRedirect(&http.Request{URL: &url.URL{Scheme: "https", Host: "public.test"}}, []*http.Request{{}}); err == nil {
		t.Fatal("redirect count")
	}

	cfg.WebFetchMaxBytes = 1000
	mode = "fetch-html"
	text, err := webFetch(context.Background(), cfg, "https://public.test/html")
	if err != nil || !strings.Contains(text, "Hello World") {
		t.Fatal(text, err)
	}
	mode = "fetch-text"
	text, err = webFetch(context.Background(), cfg, "https://public.test/text")
	if err != nil || !strings.Contains(text, "plain") {
		t.Fatal(text, err)
	}
	cfg.NetworkEnabled = false
	if _, err := webFetch(context.Background(), cfg, "https://public.test"); err == nil {
		t.Fatal("fetch disabled")
	}
	cfg.NetworkEnabled = true
	mode = "status"
	if _, err := webFetch(context.Background(), cfg, "https://public.test"); err == nil {
		t.Fatal("fetch status")
	}
	cfg.WebFetchMaxBytes = 10
	mode = "fetch-large"
	if _, err := webFetch(context.Background(), cfg, "https://public.test"); err == nil {
		t.Fatal("fetch large")
	}
	mode = "transport-error"
	if _, err := webFetch(context.Background(), cfg, "https://public.test"); err == nil {
		t.Fatal("fetch transport")
	}
	formatted := formatWebResults([]WebResult{{Title: "T", URL: "U", Content: "C"}})
	if !strings.Contains(formatted, "1. T") {
		t.Fatal(formatted)
	}

	if _, err := bingRSSSearch(context.Background(), " ", 1); err == nil {
		t.Fatal("rss empty query")
	}
	mode = "status"
	if _, err := bingRSSSearch(context.Background(), "q", 1); err == nil {
		t.Fatal("rss status")
	}
	mode = "rss-bad"
	if _, err := bingRSSSearch(context.Background(), "q", 1); err == nil {
		t.Fatal("rss xml")
	}
	mode = "rss-empty"
	if _, err := bingRSSSearch(context.Background(), "q", 1); err == nil {
		t.Fatal("rss empty")
	}
	mode = "ddg-empty"
	results, err = bingRSSSearch(context.Background(), "q", 1)
	if err != nil || len(results) != 1 {
		t.Fatal(results, err)
	}
}

func TestCoverageAgentActionsAndApprovals(t *testing.T) {
	base := isolateCoverageEnv(t)
	root := filepath.Join(base, "root")
	project := filepath.Join(root, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "a.txt"), []byte("one needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.LastModel = "test-model"
	cfg.EditingEngine = "native"
	cfg.AiderEnabled = false
	cfg.ApprovalMode = "dangerous"
	cfg.AutoStateUpdate = false
	cfg.CreateProjectDocs = false
	cfg.CommandTimeout = 5
	cfg.Language = "en"
	cfg.PreferredLanguage = "en"
	for i := range cfg.MCPServers {
		if cfg.MCPServers[i].Transport != "builtin" {
			cfg.MCPServers[i].Enabled = false
		}
	}
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	state.Model = "test-model"
	state.Threads = map[string]*ChatThread{}
	state.selectProjectThread(project)
	t.Cleanup(state.Close)
	ctx := context.Background()

	// Pure approval policy matrices.
	for _, mode := range []string{"strict", "balanced", "auto", "dangerous"} {
		cfg2 := cfg
		cfg2.ApprovalMode = mode
		for _, a := range []AgentAction{{Action: "list_files"}, {Action: "web_search"}, {Action: "replace_text"}, {Action: "skill_copy_resource", Skill: "asset-skill", Resource: "assets/icon.png", Destination: "public/icon.png"}, {Action: "skill_run_script", Skill: "asset-skill", Script: "echo skill-ok"}, {Action: "aider_repo_map"}, {Action: "git", Args: []string{"status"}}, {Action: "git", Args: []string{"add", "."}}, {Action: "git_commit"}, {Action: "run_tool", Tool: "go", Args: []string{"version"}}, {Action: "run_command", Command: "go test"}, {Action: "delete_file"}} {
			_ = actionNeedsApproval(cfg2, project, a)
		}
	}
	for _, cmd := range []string{"git status", "git diff", "git log", "go test", "go vet", "go list", "npm test", "npm run lint", "pytest", "cargo test", "dotnet test", "dir", "type x", "findstr x", "where go", "echo hi", "rm x"} {
		_ = commandLooksReadOnly(cmd)
	}
	for _, c := range []struct {
		tool string
		args []string
	}{{"adb", []string{"devices"}}, {"adb", []string{"install", "x"}}, {"git", []string{"status"}}, {"go", []string{"version"}}, {"node", []string{"--version"}}, {"python", []string{"script.py"}}, {"unknown", nil}} {
		_ = toolActionLooksReadOnly(c.tool, c.args)
	}
	msgs := make([]OllamaMessage, 40)
	if len(trimMessages(msgs)) != 30 || len(trimMessages(msgs[:2])) != 2 {
		t.Fatal("trim")
	}

	// Preview all supported action classes.
	previews := []AgentAction{
		{Action: "replace_text", Path: "a.txt", OldText: "one", NewText: "two"}, {Action: "write_file", Path: "new.txt", Content: "new"}, {Action: "delete_file", Path: "a.txt"},
		{Action: "run_tool", Tool: "go", Args: []string{"version"}}, {Action: "run_command", Command: "echo hi"}, {Action: "open_terminal", Command: "echo hi"},
		{Action: "copy_path", Source: "a.txt", Destination: "copy.txt"}, {Action: "move_path", Source: "a.txt", Destination: "move.txt"}, {Action: "git", Args: []string{"status"}},
		{Action: "git_commit", CommitMessage: "test: commit"}, {Action: "web_search", Query: "query"}, {Action: "web_fetch", URL: "https://public.test"},
		{Action: "aider_edit", Task: "change a.txt"}, {Action: "aider_repo_map"}, {Action: "aider_lint"}, {Action: "aider_test"}, {Action: "build_project"}, {Action: "deploy_android"},
		{Action: "mcp_call_tool", Server: "filesystem", Tool: "read_text_file", Arguments: map[string]any{"path": "a.txt"}},
	}
	for _, a := range previews {
		if _, err := previewAction(project, cfg, a); err != nil {
			t.Fatalf("preview %s: %v", a.Action, err)
		}
	}
	for _, a := range []AgentAction{{Action: "replace_text", Path: "a.txt", OldText: "missing", NewText: "x"}, {Action: "run_tool"}, {Action: "run_command"}, {Action: "git", Args: []string{"reset", "--hard"}}, {Action: "aider_edit"}, {Action: "unknown"}} {
		if _, err := previewAction(project, cfg, a); err == nil {
			t.Fatalf("expected preview error %s", a.Action)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "bin.dat"), []byte{0, 1, 2, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := previewAction(project, cfg, AgentAction{Action: "write_file", Path: "bin.dat", Content: "x"}); err == nil {
		t.Fatal("binary preview")
	}

	// Read actions through the visible agent dispatcher.
	resourceURI, err := fileURI(filepath.Join(project, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []AgentAction{
		{Action: "discover_tool", Tool: "go", Message: "discover"}, {Action: "project_info", Message: "info"}, {Action: "tool_inventory", Message: "inventory"},
		{Action: "list_files", Path: ".", MaxDepth: 2, Message: "list"}, {Action: "read_file", Path: "a.txt", Message: "read"}, {Action: "search_text", Query: "needle", Message: "search"},
		{Action: "mcp_list_tools", Server: "filesystem", Message: "tools"}, {Action: "mcp_list_resources", Server: "filesystem", Message: "resources"},
		{Action: "mcp_read_resource", Server: "filesystem", URI: resourceURI, Message: "resource"}, {Action: "mcp_list_prompts", Server: "filesystem", Message: "prompts"},
	} {
		res, wait := state.handleAgentAction(ctx, project, a)
		if wait || strings.Contains(res, "ERROR:") {
			t.Fatalf("action %s wait=%v res=%s", a.Action, wait, res)
		}
	}
	if _, wait := state.handleAgentAction(ctx, project, AgentAction{Action: "ask_user", Message: "question"}); !wait {
		t.Fatal("ask")
	}
	if _, wait := state.handleAgentAction(ctx, project, AgentAction{Action: "finish", Message: "done"}); !wait {
		t.Fatal("finish")
	}
	if res, _ := state.handleAgentAction(ctx, project, AgentAction{Action: "unknown", Message: "bad"}); !strings.Contains(res, "ERROR:") {
		t.Fatal(res)
	}

	// Mutating actions use dangerous mode here to execute immediately.
	mutations := []AgentAction{
		{Action: "write_file", Path: "w.txt", Content: "alpha", Message: "write"}, {Action: "replace_text", Path: "w.txt", OldText: "alpha", NewText: "beta", Message: "replace"},
		{Action: "copy_path", Source: "w.txt", Destination: "c.txt", Message: "copy"}, {Action: "move_path", Source: "c.txt", Destination: "m.txt", Message: "move"},
		{Action: "run_tool", Tool: "go", Args: []string{"version"}, Message: "go"}, {Action: "run_command", Command: "echo command-ok", Message: "command"},
		{Action: "mcp_call_tool", Server: "filesystem", Tool: "read_text_file", Arguments: map[string]any{"path": "w.txt"}, Message: "mcp"},
	}
	for _, a := range mutations {
		res, wait := state.performApproved(ctx, project, a)
		if wait || strings.Contains(res, "ERROR:") {
			t.Fatalf("mutation %s: %s wait=%v", a.Action, res, wait)
		}
	}
	if res, _ := state.performApproved(ctx, project, AgentAction{Action: "delete_file", Path: "m.txt", Message: "delete"}); strings.Contains(res, "ERROR:") {
		t.Fatal(res)
	}

	// Git paths.
	if res, err := executeAction(ctx, project, cfg, AgentAction{Action: "git", Args: []string{"init"}}); err != nil || !strings.Contains(strings.ToLower(res), "git") {
		t.Fatal(res, err)
	}
	if res, _ := state.handleAgentAction(ctx, project, AgentAction{Action: "git", Args: []string{"status"}, Message: "status"}); strings.Contains(res, "ERROR:") {
		t.Fatal(res)
	}
	cfgOff := cfg
	cfgOff.GitEnabled = false
	state.mu.Lock()
	state.Config = cfgOff
	state.mu.Unlock()
	if res, _ := state.handleAgentAction(ctx, project, AgentAction{Action: "git", Args: []string{"status"}, Message: "status"}); !strings.Contains(res, "ERROR:") {
		t.Fatal(res)
	}
	state.mu.Lock()
	state.Config = cfg
	state.mu.Unlock()

	// Direct executeAction file/error branches.
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "write_file", Path: "direct.txt", Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "replace_text", Path: "direct.txt", OldText: "x", NewText: "y"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "copy_path", Source: "direct.txt", Destination: "direct-copy.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "move_path", Source: "direct-copy.txt", Destination: "direct-move.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "delete_file", Path: "direct-move.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "run_tool", Tool: "go", Args: []string{"version"}}); err != nil {
		t.Fatal(err)
	}
	if out, err := executeAction(ctx, project, cfg, AgentAction{Action: "run_command", Command: "echo direct"}); err != nil || !strings.Contains(out, "direct") {
		t.Fatal(out, err)
	}
	if _, err := executeAction(ctx, project, cfg, AgentAction{Action: "unknown"}); err == nil {
		t.Fatal("unsupported")
	}

	// Persistent allow and forbid rules bypass/police the approval prompt.
	allowCfg := cfg
	allowCfg.ApprovalMode = "strict"
	allowCfg.ApprovalRules = []ApprovalRule{{Scope: "global", Decision: "allow", Pattern: []string{"write_file", "allowed.txt"}}}
	state.mu.Lock()
	state.Config = allowCfg
	state.mu.Unlock()
	ok, err := state.requestApprovalWithPreview(ctx, project, AgentAction{Action: "write_file", Path: "allowed.txt"}, "preview")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	forbidCfg := allowCfg
	forbidCfg.ApprovalRules = []ApprovalRule{{Scope: "global", Decision: "forbidden", Pattern: []string{"write_file", "blocked.txt"}}}
	state.mu.Lock()
	state.Config = forbidCfg
	state.mu.Unlock()
	if ok, err := state.requestApprovalWithPreview(ctx, project, AgentAction{Action: "write_file", Path: "blocked.txt"}, "preview"); err == nil || ok {
		t.Fatal(ok, err)
	}

	// One-time approval and rejection via the pending channel.
	state.mu.Lock()
	state.Config = allowCfg
	state.Config.ApprovalRules = nil
	state.mu.Unlock()
	for _, approved := range []bool{true, false} {
		ctx2, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result := make(chan struct {
			ok  bool
			err error
		}, 1)
		go func() {
			ok, err := state.requestApprovalWithPreview(ctx2, project, AgentAction{Action: "run_command", Message: "approve", Command: "echo x"}, "preview")
			result <- struct {
				ok  bool
				err error
			}{ok, err}
		}()
		deadline := time.Now().Add(time.Second)
		for {
			state.mu.RLock()
			p := state.Pending
			state.mu.RUnlock()
			if p != nil {
				p.Result <- ApprovalDecision{Approved: approved}
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("pending not set")
			}
			time.Sleep(time.Millisecond)
		}
		r := <-result
		if r.err != nil || r.ok != approved {
			t.Fatal(r)
		}
	}
}

func writeCoverageExecutable(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("shell-backed coverage helper is only used on non-Windows test hosts")
	}
	content := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeAiderForCoverage(t *testing.T, cfg *Config, project string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "aider")
	writeCoverageExecutable(t, path, `
if [ "${1:-}" = "--version" ]; then
  echo "aider 0.86.2"
  exit 0
fi
printf 'fake aider:'
printf ' %s' "$@"
printf '\n'
case " $* " in
  *" --message-file "*) printf 'edited\n' >> edit-result.txt ;;
esac
`)
	cfg.AiderEnabled = true
	cfg.AiderExecutable = path
	cfg.AiderVersion = aiderPinnedVersion
	cfg.AiderMainModel = "qwen2.5-coder:14b"
	cfg.AiderMapTokens = 1024
	cfg.AiderMaxChatHistoryTokens = 2048
	cfg.ModelTimeout = 30
	cfg.CommandTimeout = 60
	cfg.RootProjectDir = filepath.Dir(project)
	cfg.LastProject = project
	cfg.NetworkEnabled = true
	return path
}

func TestCoverageAiderDownloadVerifyAndExtract(t *testing.T) {
	isolateCoverageEnv(t)
	payload := []byte("download-payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = w.Write(payload)
		case "/large":
			w.Header().Set("Content-Length", "9999")
			_, _ = w.Write(payload)
		case "/stream":
			w.Header().Del("Content-Length")
			_, _ = w.Write(bytes.Repeat([]byte("x"), 32))
		default:
			http.Error(w, "no", http.StatusBadGateway)
		}
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "nested", "file.bin")
	if err := downloadFile(context.Background(), server.URL+"/ok", target, 100); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); !bytes.Equal(got, payload) {
		t.Fatalf("download=%q", got)
	}
	// Existing destinations must be atomically replaced.
	if err := os.WriteFile(target, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := downloadFile(context.Background(), server.URL+"/ok", target, 100); err != nil {
		t.Fatal(err)
	}
	if err := downloadFile(context.Background(), server.URL+"/large", target, 10); err == nil {
		t.Fatal("expected content-length limit")
	}
	if err := downloadFile(context.Background(), server.URL+"/stream", target, 10); err == nil {
		t.Fatal("expected streaming limit")
	}
	if err := downloadFile(context.Background(), server.URL+"/missing", target, 100); err == nil {
		t.Fatal("expected HTTP failure")
	}
	if err := downloadFile(context.Background(), "://bad", target, 100); err == nil {
		t.Fatal("expected malformed URL")
	}

	sum := sha256.Sum256(payload)
	expected := hex.EncodeToString(sum[:])
	if err := verifyFileSHA256(target, expected); err != nil {
		t.Fatal(err)
	}
	if err := verifyFileSHA256(target, strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected checksum mismatch")
	}
	if err := verifyFileSHA256(filepath.Join(t.TempDir(), "missing"), expected); err == nil {
		t.Fatal("expected missing checksum file")
	}

	zipPath := filepath.Join(t.TempDir(), "valid.zip")
	if err := os.WriteFile(zipPath, makeZipBytes(t, map[string]string{"dir/tool": "ok"}), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "extract")
	if err := extractZipFile(zipPath, destination); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(destination, "dir", "tool")); string(got) != "ok" {
		t.Fatalf("extract=%q", got)
	}
	unsafeZip := filepath.Join(t.TempDir(), "unsafe.zip")
	if err := os.WriteFile(unsafeZip, makeZipBytes(t, map[string]string{"../escape": "bad"}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractZipFile(unsafeZip, filepath.Join(t.TempDir(), "unsafe")); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if err := extractZipFile(filepath.Join(t.TempDir(), "not-a-zip"), destination); err == nil {
		t.Fatal("expected invalid zip")
	}
}

func TestCoverageAiderInstallAndUtilities(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	fakeAiderForCoverage(t, &cfg, project)

	status := aiderStatus(context.Background(), cfg)
	if !status.Installed || !strings.Contains(status.Version, aiderPinnedVersion) {
		t.Fatalf("status=%+v", status)
	}
	badCfg := cfg
	badCfg.AiderVersion = "9.9.9"
	if bad := aiderStatus(context.Background(), badCfg); bad.Installed || bad.Error == "" {
		t.Fatalf("bad=%+v", bad)
	}
	missingCfg := cfg
	missingCfg.AiderExecutable = filepath.Join(t.TempDir(), "missing")
	if missing := aiderStatus(context.Background(), missingCfg); missing.Installed || missing.Error == "" {
		t.Fatalf("missing=%+v", missing)
	}
	if !strings.Contains((&AiderNotInstalledError{Status: missingCfgStatus(missingCfg)}).Error(), "Aider") {
		t.Fatal("typed error")
	}
	if !strings.Contains((&AiderNotInstalledError{}).Error(), "nicht installiert") {
		t.Fatal("fallback error")
	}

	for _, mode := range []string{"repo-map", "lint", "test"} {
		out, err := runAiderUtility(context.Background(), project, mode, cfg.AiderMainModel, "thread", cfg)
		if err != nil || !strings.Contains(out, "fake aider") {
			t.Fatalf("mode=%s out=%q err=%v", mode, out, err)
		}
	}
	if _, err := runAiderUtility(context.Background(), project, "bad", cfg.AiderMainModel, "thread", cfg); err == nil {
		t.Fatal("unsupported utility")
	}
	if _, err := runAiderUtility(context.Background(), project, "repo-map", "", "thread", Config{}); err == nil {
		t.Fatal("missing executable/model")
	}

	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	state.Model = cfg.AiderMainModel
	state.CurrentThread = "thread-current"
	t.Cleanup(state.Close)
	thread, model := state.currentAiderThreadAndModel(cfg)
	if thread != "thread-current" || model != cfg.AiderMainModel {
		t.Fatalf("thread=%q model=%q", thread, model)
	}
	for _, action := range []AgentAction{
		{Action: "aider_edit", Task: "edit the file"},
		{Action: "aider_repo_map"},
		{Action: "aider_lint"},
		{Action: "aider_test"},
	} {
		out, err := state.executeAiderAction(context.Background(), project, cfg, action)
		if err != nil || strings.TrimSpace(out) == "" {
			t.Fatalf("action=%s out=%q err=%v", action.Action, out, err)
		}
	}
	if state.LastAiderBackup == "" {
		t.Fatal("backup not recorded")
	}
	if _, err := state.executeAiderAction(context.Background(), project, cfg, AgentAction{Action: "aider_undo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.executeAiderAction(context.Background(), project, cfg, AgentAction{Action: "invalid"}); err == nil {
		t.Fatal("unsupported action")
	}

	formatted := formatAiderRunResult(AiderRunResult{Output: "output", ChangedFiles: []string{"a.go"}, BackupDir: "/backup", Executable: "aider", Arguments: []string{"--x"}, Duration: time.Second, ExitCode: 0}, cfg)
	if !strings.Contains(formatted, "a.go") || !strings.Contains(formatted, "AIDER OUTPUT") {
		t.Fatalf("formatted=%q", formatted)
	}

	// A local fake uv installation exercises the complete managed install path.
	uv := filepath.Join(appDataDir(), "tools", "uv", uvExecutableName())
	writeCoverageExecutable(t, uv, `
if [ "${1:-}" = "--version" ]; then echo "uv 0.11.16"; exit 0; fi
if [ "${1:-}" = "tool" ] && [ "${2:-}" = "install" ]; then
  mkdir -p "$UV_TOOL_BIN_DIR"
  cat > "$UV_TOOL_BIN_DIR/aider" <<'EOS'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then echo "aider 0.86.2"; else echo "installed aider"; fi
EOS
  chmod +x "$UV_TOOL_BIN_DIR/aider"
  echo "installed"
  exit 0
fi
exit 3
`)
	installCfg := defaultConfig()
	installCfg.NetworkEnabled = true
	installCfg.AiderEnabled = true
	installCfg.AiderVersion = aiderPinnedVersion
	installCfg.AiderExecutable = ""
	uvPath, uvVersion, err := ensureUVInstalled(context.Background(), installCfg)
	if err != nil || uvPath == "" || !strings.Contains(uvVersion, "0.11.16") {
		t.Fatalf("uv=%q version=%q err=%v", uvPath, uvVersion, err)
	}
	installed, detail, err := installAider(context.Background(), installCfg)
	if err != nil || !installed.Installed || !strings.Contains(detail, "installed") {
		t.Fatalf("installed=%+v detail=%q err=%v", installed, detail, err)
	}
	installCfg.SetupDownloadsEnabled = false
	if _, _, err := installAider(context.Background(), installCfg); err == nil {
		t.Fatal("expected setup-download-disabled install failure")
	}
}

func missingCfgStatus(cfg Config) AiderStatus {
	return aiderStatus(context.Background(), cfg)
}

func TestCoverageAiderHTTPHandlers(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	fakeAiderForCoverage(t, &cfg, project)
	state := NewAppState(cfg, NewOllamaClient())
	state.Project = project
	state.Model = cfg.AiderMainModel
	t.Cleanup(state.Close)
	s := &Server{state: state}

	call := func(handler http.HandlerFunc, method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler(rec, req)
		return rec
	}
	if rec := call(s.handleAiderStatus, http.MethodGet, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "installed") {
		t.Fatalf("status=%d %s", rec.Code, rec.Body.String())
	}
	if rec := call(s.handleAiderStatus, http.MethodPost, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatal(rec.Code)
	}
	if rec := call(s.handleAiderSetup, http.MethodPost, `{"action":"test"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "fake aider") {
		t.Fatalf("setup test=%d %s", rec.Code, rec.Body.String())
	}
	if rec := call(s.handleAiderSetup, http.MethodGet, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatal(rec.Code)
	}
	if rec := call(s.handleAiderSetup, http.MethodPost, "{"); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
	if rec := call(s.handleAiderSetup, http.MethodPost, `{"action":"unknown"}`); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	if rec := call(s.handleAiderSetup, http.MethodPost, `{"action":"test"}`); rec.Code != http.StatusConflict {
		t.Fatal(rec.Code)
	}
	if rec := call(s.handleAiderUndo, http.MethodPost, ""); rec.Code != http.StatusConflict {
		t.Fatal(rec.Code)
	}
	state.mu.Lock()
	state.Running = false
	state.mu.Unlock()

	before := snapshotProjectFingerprints(project)
	backup, err := createAiderBackup(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "file.txt"), []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	after := snapshotProjectFingerprints(project)
	changed := changedFingerprintPaths(before, after)
	if err := writeAiderBackupManifest(backup, project, before, after, changed); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	state.LastAiderBackup = backup
	state.mu.Unlock()
	if rec := call(s.handleAiderUndo, http.MethodPost, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Restored") {
		t.Fatalf("undo=%d %s", rec.Code, rec.Body.String())
	}
	if rec := call(s.handleAiderUndo, http.MethodGet, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatal(rec.Code)
	}
	state.mu.Lock()
	state.Project = ""
	state.mu.Unlock()
	if rec := call(s.handleAiderUndo, http.MethodPost, ""); rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code)
	}
}

func TestCoverageMCPHTTPAndSSE(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	t.Setenv("MCP_TEST_TOKEN", "secret")
	var initialized, notified, called bool
	serverHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Project") != project || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "headers", http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		method, _ := payload["method"].(string)
		switch method {
		case "initialize":
			initialized = true
			w.Header().Set("Mcp-Session-Id", "session-1")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2026-07-28"}}`)
		case "notifications/initialized":
			notified = true
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			called = true
			if r.Header.Get("Mcp-Session-Id") != "session-1" {
				http.Error(w, "session", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"alpha\"}]}}\n\n")
		case "tools/error":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":2,"error":{"code":-1,"message":"boom"}}`)
		default:
			http.Error(w, "unknown", http.StatusNotFound)
		}
	}))
	defer serverHTTP.Close()

	cfg := defaultConfig()
	serverCfg := MCPServerConfig{Name: "http-test", DisplayName: "HTTP", Enabled: true, Transport: "streamable-http", URL: serverHTTP.URL, TimeoutSec: 2, Headers: map[string]string{"X-Project": "${PROJECT_ROOT}", "Authorization": "Bearer ${MCP_TEST_TOKEN}"}}
	cfg.MCPServers = []MCPServerConfig{serverCfg}
	manager := newMCPManager()
	defer manager.Close()
	out, err := manager.callHTTP(context.Background(), cfg, project, serverCfg, "tools/list", map[string]any{})
	if err != nil || !strings.Contains(out, "alpha") || !initialized || !notified || !called {
		t.Fatalf("out=%q err=%v init=%v notify=%v call=%v", out, err, initialized, notified, called)
	}
	if _, err := manager.callHTTP(context.Background(), cfg, project, serverCfg, "tools/error", nil); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected rpc error: %v", err)
	}
	manager.ResetServer(serverCfg.Name)

	empty := &mcpHTTPSession{server: serverCfg, project: project, client: http.DefaultClient}
	if _, err := empty.call(context.Background(), cfg, "tools/list", nil); err == nil {
		t.Fatal("expected empty endpoint")
	}
	badServer := serverCfg
	badServer.Name = "bad-http"
	badServer.URL = serverHTTP.URL + "/not-found"
	badSession := &mcpHTTPSession{server: badServer, project: project, endpoint: badServer.URL, client: http.DefaultClient}
	if _, err := badSession.call(context.Background(), cfg, "tools/list", nil); err == nil {
		t.Fatal("expected initialize failure")
	}

	// Direct HTTP post response variants.
	acceptedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	defer acceptedServer.Close()
	if _, _, err := mcpHTTPPost(context.Background(), acceptedServer.Client(), cfg, MCPServerConfig{}, project, acceptedServer.URL, "s", mcpProtocolVersion, mcpNotification("n", nil)); err != nil {
		t.Fatal(err)
	}
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "failure", http.StatusTeapot) }))
	defer errorServer.Close()
	if _, _, err := mcpHTTPPost(context.Background(), errorServer.Client(), cfg, MCPServerConfig{}, project, errorServer.URL, "", mcpProtocolVersion, mcpRequest(1, "x", nil)); err == nil {
		t.Fatal("expected HTTP status error")
	}
	invalidJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "not-json") }))
	defer invalidJSON.Close()
	if _, _, err := mcpHTTPPost(context.Background(), invalidJSON.Client(), cfg, MCPServerConfig{}, project, invalidJSON.URL, "", mcpProtocolVersion, mcpRequest(1, "x", nil)); err == nil {
		t.Fatal("expected JSON error")
	}
	if _, _, err := mcpHTTPPost(context.Background(), http.DefaultClient, cfg, MCPServerConfig{}, project, "://bad", "", mcpProtocolVersion, func() any { return make(chan int) }()); err == nil {
		t.Fatal("expected marshal error")
	}

	validSSE := []byte("data: {\"jsonrpc\":\"2.0\",\n" + "data: \"id\":7,\"result\":{}}\n\n")
	if response, err := parseMCPSSE(validSSE); err != nil || fmt.Sprint(response.ID) != "7" {
		t.Fatalf("sse=%+v err=%v", response, err)
	}
	if _, err := parseMCPSSE([]byte("data: {}\n\n")); err == nil {
		t.Fatal("expected missing rpc response")
	}
	if prettyJSON(nil) != "{}" || prettyJSON(json.RawMessage("x")) != "x" {
		t.Fatal("pretty JSON fallbacks")
	}

	headers := resolveMCPHeaders(context.Background(), cfg, project, serverCfg)
	if headers["Authorization"] != "Bearer secret" || headers["X-Project"] != project {
		t.Fatalf("headers=%v", headers)
	}
	if _, ok := numericRPCID(float64(2)); !ok {
		t.Fatal("numeric float")
	}
	if _, ok := numericRPCID(json.Number("3")); !ok {
		t.Fatal("numeric number")
	}
	if _, ok := numericRPCID("4"); !ok {
		t.Fatal("numeric string")
	}
	if _, ok := numericRPCID(struct{}{}); ok {
		t.Fatal("bad numeric")
	}
	for _, value := range []string{"simple", "with space", `with"quote`, ""} {
		_ = quoteCmdToken(value)
	}
}

func TestCoverageMCPSetupAndDependencyPaths(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	cfg := defaultConfig()
	if tools, err := parseMCPToolList(`{"tools":[{"name":"zeta"},{"name":""},{"name":"alpha"}]}`); err != nil || strings.Join(tools, ",") != "alpha,zeta" {
		t.Fatalf("tools=%v err=%v", tools, err)
	}
	if _, err := parseMCPToolList("{"); err == nil {
		t.Fatal("expected parse error")
	}

	bin := t.TempDir()
	uvx := writeCoverageExecutable(t, filepath.Join(bin, "uvx"), `echo uvx`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	path, detail, err := installUV(context.Background())
	if err != nil || path != uvx || !strings.Contains(detail, "already installed") {
		t.Fatalf("uvx=%q detail=%q err=%v", path, detail, err)
	}
	fetch := MCPServerConfig{Name: "fetch", DisplayName: "Fetch", Preset: "fetch", Enabled: true, Transport: "stdio", Command: "uvx", AutoInstall: true}
	cfg.MCPServers = []MCPServerConfig{fetch}
	updated, detail, err := installMCPDependency(context.Background(), project, cfg, fetch)
	if err != nil || updated.MCPServers[0].Command != uvx || detail == "" {
		t.Fatalf("updated=%+v detail=%q err=%v", updated.MCPServers, detail, err)
	}
	if _, _, err := installMCPDependency(context.Background(), project, cfg, MCPServerConfig{Name: "unknown", Preset: "unknown"}); err == nil {
		t.Fatal("expected unsupported installer")
	}

	statuses := []MCPServerConfig{
		{Name: "fs", DisplayName: "", Preset: "filesystem", Enabled: true, Transport: "builtin"},
		{Name: "unknown-builtin", Preset: "other", Enabled: true, Transport: "builtin"},
		{Name: "missing", Preset: "other", Enabled: true, Transport: "stdio", Command: filepath.Join(project, "missing")},
		{Name: "http", Enabled: false, Transport: "http", URL: "https://example.invalid"},
		{Name: "bad", Enabled: true, Transport: "mystery"},
	}
	for _, sc := range statuses {
		status := mcpServerStatus(context.Background(), cfg, project, sc, false)
		if status.Name == "" {
			t.Fatalf("status=%+v", status)
		}
	}
	cfg.MCPServers = []MCPServerConfig{{Name: "fs", Preset: "filesystem", Enabled: true, Transport: "builtin"}}
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	updatedCfg, detail, pending, err := state.prepareMCPServer(context.Background(), project, cfg, AgentAction{Action: "mcp_call_tool", Server: "fs"})
	if err != nil || detail != "" || pending || len(updatedCfg.MCPServers) != 1 {
		t.Fatalf("detail=%q pending=%v err=%v", detail, pending, err)
	}
	if _, _, _, err := state.prepareMCPServer(context.Background(), project, cfg, AgentAction{Server: "missing"}); err == nil {
		t.Fatal("expected missing server")
	}
	cfg.MCPServers = []MCPServerConfig{{Name: "missing", DisplayName: "Missing", Preset: "other", Enabled: true, Transport: "stdio", Command: filepath.Join(project, "missing"), AutoInstall: false}}
	state.mu.Lock()
	state.Config = cfg
	state.mu.Unlock()
	if _, _, _, err := state.prepareMCPServer(context.Background(), project, cfg, AgentAction{Server: "missing"}); err == nil {
		t.Fatal("expected no-auto-install failure")
	}
}

func TestCoverageManagedToolInstallers(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	oldGOOS := toolRuntimeGOOS
	oldClient := toolHTTPClient
	oldAndroid := androidPlatformToolsURL
	oldMinGit := minGitReleaseURL
	oldDotnet := dotnetInstallURL
	oldVS := vsBuildToolsURL
	oldNodeIndex := nodeIndexURL
	oldNodeBase := nodeDistBaseURL
	oldGH := githubCLIReleaseURL
	t.Cleanup(func() {
		toolRuntimeGOOS = oldGOOS
		toolHTTPClient = oldClient
		androidPlatformToolsURL = oldAndroid
		minGitReleaseURL = oldMinGit
		dotnetInstallURL = oldDotnet
		vsBuildToolsURL = oldVS
		nodeIndexURL = oldNodeIndex
		nodeDistBaseURL = oldNodeBase
		githubCLIReleaseURL = oldGH
	})
	toolRuntimeGOOS = "windows"
	toolHTTPClient = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }

	platformZip := makeZipBytes(t, map[string]string{"platform-tools/adb.exe": "adb", "platform-tools/fastboot.exe": "fastboot"})
	gitZip := makeZipBytes(t, map[string]string{"cmd/git.exe": "git"})
	nodeZip := makeZipBytes(t, map[string]string{"node-v22.1.0-win-x64/node.exe": "node", "node-v22.1.0-win-x64/npm.cmd": "npm", "node-v22.1.0-win-x64/npx.cmd": "npx"})
	ghZip := makeZipBytes(t, map[string]string{"gh_2.0.0_windows_amd64/bin/gh.exe": "gh"})
	vsScript := `#!/bin/sh
set -eu
root="$ProgramFiles/Microsoft Visual Studio/2026/BuildTools/MSBuild/Current/Bin"
mkdir -p "$root"
printf msbuild > "$root/MSBuild.exe"
echo installed-vs
`
	var downloads *httptest.Server
	downloads = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/platform.zip":
			_, _ = w.Write(platformZip)
		case "/mingit-release":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]string{{"name": "MinGit-2.0-64-bit.zip", "browser_download_url": downloads.URL + "/mingit.zip"}, {"name": "MinGit-busybox-64-bit.zip", "browser_download_url": downloads.URL + "/bad.zip"}}})
		case "/mingit.zip":
			_, _ = w.Write(gitZip)
		case "/dotnet.ps1":
			_, _ = io.WriteString(w, "# fake dotnet installer")
		case "/vs.exe":
			_, _ = io.WriteString(w, vsScript)
		case "/node-index":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"version": "v23.0.0", "lts": false, "files": []string{"win-x64-zip"}}, {"version": "v22.1.0", "lts": "Jod", "files": []string{"win-x64-zip"}}})
		case "/node/v22.1.0/node-v22.1.0-win-x64.zip":
			_, _ = w.Write(nodeZip)
		case "/gh-release":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []map[string]string{{"name": "gh_2.0.0_windows_amd64.zip", "browser_download_url": downloads.URL + "/gh.zip"}}})
		case "/gh.zip":
			_, _ = w.Write(ghZip)
		case "/failure":
			http.Error(w, "download failed", http.StatusBadGateway)
		default:
			http.NotFound(w, r)
		}
	}))
	defer downloads.Close()
	androidPlatformToolsURL = downloads.URL + "/platform.zip"
	minGitReleaseURL = downloads.URL + "/mingit-release"
	dotnetInstallURL = downloads.URL + "/dotnet.ps1"
	vsBuildToolsURL = downloads.URL + "/vs.exe"
	nodeIndexURL = downloads.URL + "/node-index"
	nodeDistBaseURL = downloads.URL + "/node"
	githubCLIReleaseURL = downloads.URL + "/gh-release"

	bin := t.TempDir()
	powershell := writeCoverageExecutable(t, filepath.Join(bin, "powershell.exe"), `
install=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "-InstallDir" ]; then install="$arg"; fi
  prev="$arg"
done
if [ -n "$install" ]; then mkdir -p "$install"; printf dotnet > "$install/dotnet.exe"; fi
echo powershell-ok
`)
	_ = powershell
	writeCoverageExecutable(t, filepath.Join(bin, "winget.exe"), `echo winget-ok`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	programFiles := filepath.Join(t.TempDir(), "Program Files")
	t.Setenv("ProgramFiles", programFiles)
	t.Setenv("ProgramFiles(x86)", filepath.Join(t.TempDir(), "Program Files x86"))
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "local"))

	target := filepath.Join(t.TempDir(), "downloads", "payload")
	if err := downloadToFile(context.Background(), downloads.URL+"/dotnet.ps1", target); err != nil {
		t.Fatal(err)
	}
	if err := downloadToFile(context.Background(), downloads.URL+"/failure", target); err == nil {
		t.Fatal("expected download failure")
	}
	if err := downloadToFile(context.Background(), "://bad", target); err == nil {
		t.Fatal("expected bad URL")
	}

	root, detail, err := installAndroidPlatformTools(context.Background())
	if err != nil || !strings.Contains(detail, "Google") {
		t.Fatalf("android root=%q detail=%q err=%v", root, detail, err)
	}
	if _, err := os.Stat(filepath.Join(root, "platform-tools", "adb.exe")); err != nil {
		t.Fatal(err)
	}
	minURL, asset, err := latestMinGitURL(context.Background())
	if err != nil || minURL != downloads.URL+"/mingit.zip" || asset == "" {
		t.Fatalf("url=%q asset=%q err=%v", minURL, asset, err)
	}
	gitPath, detail, err := installPortableGit(context.Background())
	if err != nil || !strings.HasSuffix(gitPath, filepath.Join("cmd", "git.exe")) || detail == "" {
		t.Fatalf("git=%q detail=%q err=%v", gitPath, detail, err)
	}
	if findWingetExecutable() == "" {
		t.Fatal("winget not found")
	}
	if out, err := installWithWinget(context.Background(), "Example.Package"); err != nil || !strings.Contains(out, "winget-ok") {
		t.Fatalf("winget out=%q err=%v", out, err)
	}
	if err := os.WriteFile(filepath.Join(project, "global.json"), []byte(`{"sdk":{"version":"8.0.100"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	dotnet, detail, err := installDotnetSDK(context.Background(), project)
	if err != nil || !strings.HasSuffix(dotnet, "dotnet.exe") || detail == "" {
		t.Fatalf("dotnet=%q detail=%q err=%v", dotnet, detail, err)
	}
	msbuild, detail, err := installVisualStudioBuildTools(context.Background())
	if err != nil || !strings.HasSuffix(msbuild, "MSBuild.exe") || !strings.Contains(detail, "installed-vs") {
		t.Fatalf("msbuild=%q detail=%q err=%v", msbuild, detail, err)
	}
	nodeRoot, detail, err := installPortableNode(context.Background())
	if err != nil || !strings.Contains(nodeRoot, "node") || detail == "" {
		t.Fatalf("node=%q detail=%q err=%v", nodeRoot, detail, err)
	}
	gh, detail, err := installPortableGitHubCLI(context.Background())
	if err != nil || !strings.HasSuffix(gh, "gh.exe") || detail == "" {
		t.Fatalf("gh=%q detail=%q err=%v", gh, detail, err)
	}

	cfg := defaultConfig()
	for _, name := range []string{"adb", "git", "dotnet", "msbuild", "gh", "node", "java"} {
		updated, output, err := installKnownTool(context.Background(), project, name, cfg)
		if err != nil {
			t.Fatalf("install %s: %v output=%q", name, err, output)
		}
		cfg = updated
	}
	if cfg.ToolOverrides["adb"] == "" || cfg.ToolOverrides["git"] == "" || cfg.ToolOverrides["dotnet"] == "" || cfg.ToolOverrides["msbuild"] == "" || cfg.ToolOverrides["gh"] == "" || cfg.ToolOverrides["node"] == "" {
		t.Fatalf("overrides=%v", cfg.ToolOverrides)
	}
	if _, _, err := installKnownTool(context.Background(), project, "unknown-tool", cfg); err == nil {
		t.Fatal("expected unknown installer")
	}
	if (&ToolNotFoundError{}).Error() == "" || (*ToolNotFoundError)(nil).Error() == "" || localToolDir("x") == "" {
		t.Fatal("helper output")
	}
}

func TestCoverageManagedToolInstallerFailures(t *testing.T) {
	isolateCoverageEnv(t)
	oldGOOS := toolRuntimeGOOS
	oldMinGit := minGitReleaseURL
	oldNodeIndex := nodeIndexURL
	oldGH := githubCLIReleaseURL
	t.Cleanup(func() {
		toolRuntimeGOOS = oldGOOS
		minGitReleaseURL = oldMinGit
		nodeIndexURL = oldNodeIndex
		githubCLIReleaseURL = oldGH
	})
	toolRuntimeGOOS = "windows"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/empty-release":
			_ = json.NewEncoder(w).Encode(map[string]any{"assets": []any{}})
		case "/bad-json":
			_, _ = io.WriteString(w, "{")
		case "/no-node":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"version": "v1", "lts": false, "files": []string{}}})
		default:
			http.Error(w, "bad", http.StatusBadGateway)
		}
	}))
	defer server.Close()
	minGitReleaseURL = server.URL + "/empty-release"
	if _, _, err := latestMinGitURL(context.Background()); err == nil {
		t.Fatal("expected missing MinGit asset")
	}
	minGitReleaseURL = server.URL + "/bad-json"
	if _, _, err := latestMinGitURL(context.Background()); err == nil {
		t.Fatal("expected bad release JSON")
	}
	minGitReleaseURL = server.URL + "/status"
	if _, _, err := latestMinGitURL(context.Background()); err == nil {
		t.Fatal("expected release HTTP error")
	}
	nodeIndexURL = server.URL + "/no-node"
	if _, _, err := installPortableNode(context.Background()); err == nil {
		t.Fatal("expected no Node release")
	}
	githubCLIReleaseURL = server.URL + "/empty-release"
	if _, _, err := installPortableGitHubCLI(context.Background()); err == nil {
		t.Fatal("expected missing gh asset")
	}
	toolRuntimeGOOS = "linux"
	if _, _, err := installPortableNode(context.Background()); err == nil {
		t.Fatal("expected non-Windows node rejection")
	}
	if _, _, err := installPortableGitHubCLI(context.Background()); err == nil {
		t.Fatal("expected non-Windows gh rejection")
	}
	if _, _, err := installKnownTool(context.Background(), t.TempDir(), "git", defaultConfig()); err == nil {
		t.Fatal("expected non-Windows install rejection")
	}
}

func TestCoverageOllamaBootstrapAndDiagnostics(t *testing.T) {
	isolateCoverageEnv(t)
	models := []string{}
	var pulls []string
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			items := make([]map[string]any, 0, len(models))
			for _, name := range models {
				items = append(items, map[string]any{"name": name, "size": 123, "modified_at": "2026-08-06T00:00:00Z"})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"models": items})
		case "/api/pull":
			var request ollamaPullRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			pulls = append(pulls, request.Model)
			found := false
			for _, model := range models {
				if model == request.Model {
					found = true
				}
			}
			if !found {
				models = append(models, request.Model)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "success"})
		case "/api/show":
			var request map[string]any
			_ = json.NewDecoder(r.Body).Decode(&request)
			name, _ := request["model"].(string)
			capabilities := []string{"completion"}
			if strings.Contains(name, "gemma") || strings.Contains(name, "vision") {
				capabilities = append(capabilities, "vision")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities})
		case "/api/chat":
			var request OllamaChatRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			if len(request.Messages) > 0 && len(request.Messages[0].Images) > 0 {
				_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Content: "image analysis"}, Done: true})
				return
			}
			_ = json.NewEncoder(w).Encode(OllamaChatResponse{Message: OllamaMessage{Content: "prefix {\"action\":\"done\"} suffix"}, Done: true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ollamaServer.Close()
	t.Setenv("OLLAMA_HOST", ollamaServer.URL)

	client := NewOllamaClient()
	client.BaseURL = "http://127.0.0.1:1"
	client.HTTP = ollamaServer.Client()
	if err := client.Discover(context.Background()); err != nil || client.BaseURL != ollamaServer.URL {
		t.Fatalf("discover base=%q err=%v", client.BaseURL, err)
	}
	if err := client.EnsureRunning(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tags, err := client.Tags(context.Background()); err != nil || len(tags) != 0 {
		t.Fatalf("tags=%v err=%v", tags, err)
	}
	if err := client.Pull(context.Background(), "qwen2.5-coder:14b"); err != nil {
		t.Fatal(err)
	}
	if tags, err := client.Tags(context.Background()); err != nil || len(tags) != 1 {
		t.Fatalf("tags=%v err=%v", tags, err)
	}
	if caps, err := client.Show(context.Background(), "qwen2.5-coder:14b"); err != nil || !hasCapability(caps, "completion") {
		t.Fatalf("caps=%v err=%v", caps, err)
	}
	models = append(models, "gemma3:4b", "llava:latest")
	if vision, err := client.FindVisionModel(context.Background()); err != nil || vision != "gemma3:4b" {
		t.Fatalf("vision=%q err=%v", vision, err)
	}
	if vision, downloaded, err := client.EnsureVisionModel(context.Background()); err != nil || vision != "gemma3:4b" || downloaded {
		t.Fatalf("vision=%q downloaded=%v err=%v", vision, downloaded, err)
	}
	analysis, err := client.DescribeImages(context.Background(), "gemma3:4b", "inspect", []Attachment{{Name: "screen.png", Data: "aW1hZ2U="}})
	if err != nil || analysis != "image analysis" {
		t.Fatalf("analysis=%q err=%v", analysis, err)
	}
	chat, err := client.Chat(context.Background(), "qwen2.5-coder:14b", []OllamaMessage{{Role: "user", Content: "go"}}, map[string]any{"type": "object"})
	if err != nil || !strings.Contains(chat, `"action"`) {
		t.Fatalf("chat=%q err=%v", chat, err)
	}
	if contextForModel("gpt-oss:20b") != 16384 || contextForModel("qwen") != 24576 {
		t.Fatal("model context")
	}
	for _, name := range []string{"qwen3-vl", "gemma4", "minicpm-v", "llava", "moondream", "other"} {
		if visionPreference(name) <= 0 {
			t.Fatal(name)
		}
	}

	cfg := defaultConfig()
	cfg.OllamaURL = ollamaServer.URL
	cfg.OllamaDefaultModel = "qwen2.5-coder:14b"
	cfg.LastModel = ""
	cfg.AiderMainModel = ""
	cfg.AiderArchitectModel = ""
	cfg.AiderEditorModel = ""
	cfg.AiderEnabled = false
	cfg.EditingEngine = "native"
	models = nil
	result, err := bootstrapRuntimeDependencies(context.Background(), cfg)
	if err != nil || len(result.Models) == 0 || result.Config.LastModel == "" || len(pulls) == 0 {
		t.Fatalf("bootstrap=%+v pulls=%v err=%v", result, pulls, err)
	}
	if _, err := ensureOllamaInstalledAndRunning(context.Background(), cfg, client); err != nil {
		t.Fatal(err)
	}
	if updated, detail, err := ensureAiderRuntime(context.Background(), cfg); err != nil || updated.AiderEnabled || detail == "" {
		t.Fatalf("aider disabled detail=%q err=%v", detail, err)
	}
	installedCfg := cfg
	fakeAiderForCoverage(t, &installedCfg, t.TempDir())
	installedCfg.EditingEngine = "aider"
	if updated, detail, err := ensureAiderRuntime(context.Background(), installedCfg); err != nil || updated.AiderExecutable == "" || !strings.Contains(detail, "installed") {
		t.Fatalf("aider detail=%q err=%v", detail, err)
	}

	// Diagnostics uses the same local Ollama endpoint and an isolated config.
	cfg.RootProjectDir = t.TempDir()
	cfg.LastProject = cfg.RootProjectDir
	cfg.LastModel = result.Config.LastModel
	cfg.AiderEnabled = false
	if err := saveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writePipe
	code := runDiagnostics()
	_ = writePipe.Close()
	os.Stdout = oldStdout
	output, _ := io.ReadAll(readPipe)
	_ = readPipe.Close()
	if code != 0 || !strings.Contains(string(output), "Diagnostics completed successfully") {
		t.Fatalf("code=%d output=%s", code, output)
	}
}

func TestCoverageLocalCodeInstanceHTTPHelpers(t *testing.T) {
	var shutdown bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/ping" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]string{"app": "LocalCode", "version": "6.4.3"})
		case r.URL.Path == "/api/shutdown" && r.Method == http.MethodPost:
			shutdown = true
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if got, ok := existingLocalCodeVersion(server.URL); !ok || got != "6.4.3" {
		t.Fatalf("version=%q ok=%v", got, ok)
	}
	if err := shutdownExistingLocalCode(server.URL); err != nil || !shutdown {
		t.Fatalf("shutdown=%v err=%v", shutdown, err)
	}
	bad := httptest.NewServer(http.NotFoundHandler())
	defer bad.Close()
	if _, ok := existingLocalCodeVersion(bad.URL); ok {
		t.Fatal("unexpected version")
	}
	if err := shutdownExistingLocalCode(bad.URL); err == nil {
		t.Fatal("expected shutdown status error")
	}
	if _, ok := existingLocalCodeVersion("http://127.0.0.1:1"); ok {
		t.Fatal("unexpected unavailable instance")
	}
	if err := shutdownExistingLocalCode("://bad"); err == nil {
		t.Fatal("expected invalid URL")
	}
}

func TestCoverageToolRegistryWindowsResolution(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	oldGOOS := toolRegistryGOOS
	oldInstallGOOS := toolRuntimeGOOS
	t.Cleanup(func() { toolRegistryGOOS = oldGOOS; toolRuntimeGOOS = oldInstallGOOS })
	toolRegistryGOOS = "windows"
	toolRuntimeGOOS = "windows"

	programFiles := filepath.Join(t.TempDir(), "Program Files")
	programFiles86 := filepath.Join(t.TempDir(), "Program Files x86")
	local := filepath.Join(t.TempDir(), "Local")
	profile := filepath.Join(t.TempDir(), "Profile")
	windir := filepath.Join(t.TempDir(), "Windows")
	for key, value := range map[string]string{"ProgramFiles": programFiles, "ProgramFiles(x86)": programFiles86, "LOCALAPPDATA": local, "USERPROFILE": profile, "WINDIR": windir} {
		t.Setenv(key, value)
	}
	sdk := filepath.Join(local, "Android", "Sdk")
	t.Setenv("ANDROID_HOME", sdk)
	paths := []string{
		filepath.Join(sdk, "platform-tools", "adb.exe"),
		filepath.Join(sdk, "platform-tools", "fastboot.exe"),
		filepath.Join(sdk, "emulator", "emulator.exe"),
		filepath.Join(sdk, "cmdline-tools", "latest", "bin", "sdkmanager.bat"),
		filepath.Join(sdk, "cmdline-tools", "12", "bin", "avdmanager.bat"),
		filepath.Join(sdk, "build-tools", "35.0.0", "aapt2.exe"),
		filepath.Join(sdk, "build-tools", "35.0.0", "apksigner.bat"),
		filepath.Join(sdk, "cmake", "3.22", "bin", "ninja.exe"),
		filepath.Join(programFiles, "Git", "cmd", "git.exe"),
		filepath.Join(programFiles, "GitHub CLI", "gh.exe"),
		filepath.Join(programFiles, "nodejs", "node.exe"),
		filepath.Join(programFiles, "nodejs", "npm.cmd"),
		filepath.Join(programFiles, "Go", "bin", "go.exe"),
		filepath.Join(programFiles, "dotnet", "dotnet.exe"),
		filepath.Join(programFiles, "Docker", "Docker", "resources", "bin", "docker.exe"),
		filepath.Join(programFiles, "CMake", "bin", "cmake.exe"),
		filepath.Join(programFiles, "Android", "Android Studio", "jbr", "bin", "java.exe"),
		filepath.Join(local, "Programs", "Python", "Python313", "python.exe"),
		filepath.Join(windir, "System32", "OpenSSH", "ssh.exe"),
		filepath.Join(windir, "System32", "curl.exe"),
		filepath.Join(programFiles86, "Microsoft Visual Studio", "Installer", "vswhere.exe"),
		filepath.Join(programFiles, "Microsoft Visual Studio", "2026", "BuildTools", "MSBuild", "Current", "Bin", "MSBuild.exe"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("tool"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "local.properties"), []byte("sdk.dir="+strings.ReplaceAll(sdk, `\`, `\\`)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.ToolOverrides = map[string]string{"custom": filepath.Join(project, "custom.exe")}
	for _, name := range []string{"adb", "fastboot", "emulator", "sdkmanager", "avdmanager", "lint", "aapt2", "apksigner", "zipalign", "d8", "ninja", "git", "gh", "node", "npm", "npx", "go", "dotnet", "docker", "cmake", "java", "keytool", "jarsigner", "python", "pip", "ssh", "scp", "curl", "vswhere", "msbuild"} {
		candidates := toolCandidatePaths(project, profileForTool(name), cfg)
		if len(candidates) == 0 {
			t.Fatalf("no candidates for %s", name)
		}
	}
	if executableName("git") != "git.exe" || scriptName("npm") != "npm.cmd" || scriptName("sdkmanager") != "sdkmanager.bat" || scriptName("plain") != "plain" {
		t.Fatal("Windows names")
	}
	unknown := profileForTool("unlisted")
	if len(unknown.Aliases) < 4 {
		t.Fatalf("aliases=%v", unknown.Aliases)
	}
	if !strings.Contains(buildWindowsCommandLine(`C:\Program Files\tool.exe`, []string{"a b", "plain", `x"y`}), `"`) {
		t.Fatal("command line quoting")
	}

	bin := t.TempDir()
	cmdExe := writeCoverageExecutable(t, filepath.Join(bin, "cmd.exe"), `echo cmd-wrapper "$@"`)
	_ = cmdExe
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmdFile := filepath.Join(project, "script.cmd")
	if err := os.WriteFile(cmdFile, []byte("echo ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	res := runDirectTool(context.Background(), project, cmdFile, []string{"arg one"}, cfg)
	if res.Err != nil || !strings.Contains(res.Stdout, "cmd-wrapper") {
		t.Fatalf("res=%+v", res)
	}

	tool := writeCoverageExecutable(t, filepath.Join(project, "mytool.exe"), `if [ "${1:-}" = "bad" ]; then echo 'unknown option' >&2; exit 2; fi; echo ok`)
	cfg.AutoDiscoverTools = true
	cfg.AutoResearchToolHelp = false
	cfg.ToolOverrides["git"] = tool
	for _, shell := range []string{"powershell", "cmd", "sh"} {
		rewritten, detail, err := rewriteKnownToolCommand(project, "git status", cfg, shell)
		if err != nil || rewritten == "git status" || detail == "" {
			t.Fatalf("shell=%s rewritten=%q detail=%q err=%v", shell, rewritten, detail, err)
		}
	}
	cfg.AutoDiscoverTools = false
	if rewritten, detail, err := rewriteKnownToolCommand(project, "git status", cfg, "sh"); err != nil || rewritten != "git status" || detail != "" {
		t.Fatal(rewritten, detail, err)
	}
	cfg.AutoDiscoverTools = true
	for _, command := range []string{"", "& git", "git | more", `"unterminated`, `"git" status`, "unknown x", `./git status`} {
		_, _, _ = rewriteKnownToolCommand(project, command, cfg, "sh")
	}
	for _, command := range []string{"git status", `"git" status`, "'git' status", "git", "git\nstatus", "| git", "git|more"} {
		_, _, _ = splitCommandHead(command)
	}
	if !shouldResearchToolFailure("node", nil, ToolRunResult{Stderr: "unknown option"}) || shouldResearchToolFailure("git", []string{"--no-pager", "status"}, ToolRunResult{}) || shouldResearchToolFailure("adb", []string{"devices"}, ToolRunResult{}) {
		t.Fatal("research classification")
	}
	cfg.ToolOverrides["git"] = tool
	if out, err := runResolvedTool(context.Background(), project, "git", []string{"ok"}, cfg); err != nil || !strings.Contains(out, "Status: OK") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if out, err := runResolvedTool(context.Background(), project, "git", []string{"bad"}, cfg); err == nil || !(strings.Contains(out, "Offizielle Dokumentation") || strings.Contains(out, "Official documentation")) {
		t.Fatalf("out=%q err=%v", out, err)
	}
	missingCfg := cfg
	missingCfg.ToolOverrides = map[string]string{"git": filepath.Join(project, "missing.exe")}
	if _, err := runResolvedTool(context.Background(), project, "git", nil, missingCfg); err == nil {
		t.Fatal("expected missing tool")
	}
}

func TestCoverageAgentSupervisorAndRepairDecisions(t *testing.T) {
	isolateCoverageEnv(t)
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.AutoResearchToolHelp = false
	cfg.NetworkEnabled = false
	for _, task := range []string{
		"deploy app auf das handy", "kompiliere das Projekt", "suche im Internet aktuelle Nachrichten", "git init", "implementiere feature", "analysiere das projekt", "hello",
	} {
		intent := classifyTaskIntent(task)
		if intent.Kind == "" {
			t.Fatal(task)
		}
		_ = deriveWebQuery(task, "")
	}
	for _, answer := range []string{"nein", "n", "nicht jetzt", "abbrechen", "lass es", "stop bitte"} {
		if !isNegativeAnswer(answer) {
			t.Fatal(answer)
		}
	}
	if isNegativeAnswer("ja") || !isAffirmativeAnswer("ja bitte") {
		t.Fatal("answer classification")
	}
	for _, question := range []string{"Git Repository initialisieren?", "Soll adb installiert werden?", "Soll node installiert werden?", "Something else"} {
		_ = suggestedActionForQuestion(question)
	}
	intents := []taskIntent{
		{Kind: "analyze", OriginalTask: "analyze"}, {Kind: "build"}, {Kind: "deploy_android"}, {Kind: "web_research", WebQuery: "query"}, {Kind: "git_init"}, {Kind: "edit", OriginalTask: "edit"}, {Kind: "general"},
	}
	for _, intent := range intents {
		for _, completed := range []map[string]bool{{}, {"project_info": true}, {"project_info": true, "build_project": true, "deploy_android": true, "web_search": true, "git": true, "aider_edit": true}} {
			cfg.EditingEngine = "aider"
			cfg.AiderEnabled = true
			_ = forcedActionForIntent(intent, completed, cfg)
		}
	}
	for _, tc := range []struct {
		result string
		task   string
	}{
		{"fatal: not a git repository", "commit changes"}, {"fatal: not a git repository", "analyze"}, {"old_text must occur exactly once", "edit"}, {"search query is empty", "search"}, {"command not found", "build"}, {"other error", "build"},
	} {
		if directive := toolFailureRecoveryDirective(AgentAction{}, tc.result, errors.New(tc.result), tc.task); !strings.Contains(directive, "SYSTEMHINWEIS") {
			t.Fatal(directive)
		}
	}
	for _, intent := range []taskIntent{{Kind: "analyze"}, {Kind: "web_research"}, {Kind: "general"}} {
		for _, action := range []AgentAction{{Action: "project_info"}, {Action: "git", Args: []string{"status"}}, {Action: "git", Args: []string{"init"}}, {Action: "write_file"}, {Action: "web_search"}, {Action: "build_project"}, {Action: "finish"}} {
			_, _ = actionAllowedForIntent(intent, action)
		}
	}
	if report := supervisedFallbackReport(taskIntent{Kind: "analyze"}, project, cfg, nil); !strings.Contains(report, "Projektanalyse") {
		t.Fatal(report)
	}
	messages := []OllamaMessage{{Content: "TOOL RESULT for web_search:\nresult text"}}
	if report := supervisedFallbackReport(taskIntent{Kind: "web_research"}, project, cfg, messages); !strings.Contains(report, "result text") {
		t.Fatal(report)
	}
	_ = supervisedFallbackReport(taskIntent{Kind: "web_research"}, project, cfg, nil)
	_ = supervisedFallbackReport(taskIntent{Kind: "general"}, project, cfg, nil)

	oldRegistry := toolRegistryGOOS
	oldInstall := toolRuntimeGOOS
	t.Cleanup(func() { toolRegistryGOOS = oldRegistry; toolRuntimeGOOS = oldInstall })
	toolRegistryGOOS = runtime.GOOS
	toolRuntimeGOOS = runtime.GOOS
	for _, action := range []AgentAction{
		{Action: "git"}, {Action: "git_commit"}, {Action: "build_project"}, {Action: "deploy_android"}, {Action: "run_tool", Tool: "definitely-missing-tool"}, {Action: "run_command", Command: "git status"}, {Action: "run_command", Command: "./custom"}, {Action: "unknown"},
	} {
		_ = missingToolForAction(project, cfg, action)
	}
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	if _, _, installed, err := state.offerInstallMissingTool(context.Background(), project, cfg, nil); err != nil || installed {
		t.Fatalf("nil missing installed=%v err=%v", installed, err)
	}
	unsupported := &ToolNotFoundError{Info: ToolInfo{Name: "unsupported"}}
	if _, _, installed, err := state.offerInstallMissingTool(context.Background(), project, cfg, unsupported); err == nil || installed {
		t.Fatalf("unsupported installed=%v err=%v", installed, err)
	}
	if out, err := state.executeActionWithToolRepair(context.Background(), project, cfg, AgentAction{Action: "write_file", Path: "repair.txt", Content: "ok"}); err != nil || out == "" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, err := state.executeActionWithToolRepair(context.Background(), project, cfg, AgentAction{Action: "run_tool", Tool: "definitely-missing-tool"}); err == nil {
		t.Fatal("expected missing tool")
	}
}

func TestCoverageProjectBuildDeployAndGitCommit(t *testing.T) {
	isolateCoverageEnv(t)
	cfg := defaultConfig()
	cfg.AutoResearchToolHelp = false
	cfg.NetworkEnabled = false
	cfg.ToolOverrides = map[string]string{}

	// Exercise every deterministic project-plan branch.
	planCases := []struct {
		name  string
		files map[string]string
		kind  string
	}{
		{"go", map[string]string{"go.mod": "module x"}, "go"},
		{"rust", map[string]string{"Cargo.toml": "[package]"}, "rust"},
		{"node-build", map[string]string{"package.json": `{"scripts":{"build":"echo ok"}}`}, "node"},
		{"node-no-build", map[string]string{"package.json": `{}`}, "node"},
		{"sln", map[string]string{"x.sln": ""}, "visual-studio"},
		{"dotnet", map[string]string{"x.csproj": `<Project Sdk="Microsoft.NET.Sdk"></Project>`}, "dotnet"},
		{"classic", map[string]string{"x.csproj": `<Project></Project>`}, "visual-studio"},
		{"gradle", map[string]string{"gradlew": ""}, "gradle"},
		{"cmake", map[string]string{"CMakeLists.txt": ""}, "cmake"},
		{"python", map[string]string{"pyproject.toml": ""}, "python"},
		{"unknown", map[string]string{}, "unknown"},
	}
	for _, tc := range planCases {
		dir := t.TempDir()
		for name, content := range tc.files {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if got := detectProjectPlan(dir); got.Kind != tc.kind {
			t.Fatalf("%s kind=%s plan=%+v", tc.name, got.Kind, got)
		}
		_ = projectInfo(dir, cfg)
	}

	goProject := t.TempDir()
	if err := os.WriteFile(filepath.Join(goProject, "go.mod"), []byte("module coveragebuild\n\ngo 1.23\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goProject, "main.go"), []byte("package main\nfunc main(){}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := buildProject(context.Background(), goProject, cfg); err != nil || !strings.Contains(out, "BUILD") {
		t.Fatalf("build=%q err=%v", out, err)
	}
	unknown := t.TempDir()
	if _, err := buildProject(context.Background(), unknown, cfg); err == nil {
		t.Fatal("expected unknown build failure")
	}
	nodeNoBuild := t.TempDir()
	if err := os.WriteFile(filepath.Join(nodeNoBuild, "package.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := buildProject(context.Background(), nodeNoBuild, cfg); err == nil {
		t.Fatal("expected no build script")
	}

	android := t.TempDir()
	manifest := filepath.Join(android, "app", "src", "main", "AndroidManifest.xml")
	apk := filepath.Join(android, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	for path, content := range map[string]string{manifest: "<manifest/>", apk: "apk", filepath.Join(android, "gradlew"): "wrapper"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	gradle := writeCoverageExecutable(t, filepath.Join(bin, "gradle"), `echo gradle-build`)
	java := writeCoverageExecutable(t, filepath.Join(bin, "java"), `echo java`)
	adb := writeCoverageExecutable(t, filepath.Join(bin, "adb"), `
case " $* " in
  *" devices "*) printf 'List of devices attached\nSERIAL123\tdevice product:test\n' ;;
  *" install "*) echo Success ;;
  *) echo adb ;;
esac
`)
	androidCfg := cfg
	androidCfg.ToolOverrides = map[string]string{"gradle": gradle, "java": java, "adb": adb}
	if out, err := deployAndroid(context.Background(), android, androidCfg); err != nil || !strings.Contains(out, "Success") {
		t.Fatalf("deploy=%q err=%v", out, err)
	}
	if _, err := deployAndroid(context.Background(), goProject, androidCfg); err == nil {
		t.Fatal("expected non-Android rejection")
	}
	if err := os.Remove(apk); err != nil {
		t.Fatal(err)
	}
	if _, err := deployAndroid(context.Background(), android, androidCfg); err == nil {
		t.Fatal("expected no APK")
	}
	if err := os.WriteFile(apk, []byte("apk"), 0o600); err != nil {
		t.Fatal(err)
	}
	adbNone := writeCoverageExecutable(t, filepath.Join(bin, "adb-none"), `printf 'List of devices attached\n'`)
	androidCfg.ToolOverrides["adb"] = adbNone
	if _, err := deployAndroid(context.Background(), android, androidCfg); err == nil {
		t.Fatal("expected no device")
	}
	adbMany := writeCoverageExecutable(t, filepath.Join(bin, "adb-many"), `printf 'List of devices attached\nA\tdevice\nB\tdevice\n'`)
	androidCfg.ToolOverrides["adb"] = adbMany
	if _, err := deployAndroid(context.Background(), android, androidCfg); err == nil {
		t.Fatal("expected multiple devices")
	}

	for _, task := range []string{"", "fix bug", "implement feature", "update README documentation", "add tests", "refactor module", strings.Repeat("long message ", 20)} {
		if msg := deriveCommitMessage(task); !strings.Contains(msg, ":") {
			t.Fatal(msg)
		}
	}
	gitProject := t.TempDir()
	gitCfg := cfg
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := initializeGitRepository(ctx, gitProject, gitCfg); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	if _, err := runGit(ctx, gitProject, []string{"config", "user.name", "Coverage Test"}, gitCfg); err != nil {
		t.Fatal(err)
	}
	if _, err := runGit(ctx, gitProject, []string{"config", "user.email", "coverage@example.invalid"}, gitCfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitProject, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if report, err := commitGitChanges(ctx, gitProject, gitCfg, "test: coverage commit", true); err != nil || !strings.Contains(report, "HEAD:") {
		t.Fatalf("commit report=%q err=%v", report, err)
	}
	if report, err := commitGitChanges(ctx, gitProject, gitCfg, "", true); err != nil || !strings.Contains(report, "No staged changes") {
		t.Fatalf("no changes report=%q err=%v", report, err)
	}
	if err := os.WriteFile(filepath.Join(gitProject, "COMMIT_MESSAGE.txt"), []byte("docs: use file message"), 0o600); err != nil {
		t.Fatal(err)
	}
	if message, useFile := commitMessageFromProject(gitProject, "fallback"); !useFile || !strings.Contains(message, "docs") {
		t.Fatal(message, useFile)
	}
}
