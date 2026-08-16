// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jsonTestBody(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRemoteAuthenticatedWorkflowHandlers(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.CreateProjectDocs = false
	project := state.Project
	state.mu.Unlock()
	remote := NewRemoteServer(state)

	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	ping := serveHTTP(remote, http.MethodGet, "/remote/api/ping", "", "")
	if ping.Code != http.StatusOK || !strings.Contains(ping.Body.String(), `"pairing":true`) {
		t.Fatalf("pairing ping status=%d body=%s", ping.Code, ping.Body.String())
	}
	if rr := serveHTTP(remote, http.MethodPost, "/remote/api/ping", "{}", ""); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST ping status=%d", rr.Code)
	}

	token, _, err := state.PairRemoteDevice(code, "coverage phone")
	if err != nil {
		t.Fatal(err)
	}
	if remote.pairingOpen() {
		t.Fatal("pairing must close after successful device pairing")
	}

	projects := serveHTTP(remote, http.MethodGet, "/remote/api/projects", "", token)
	if projects.Code != http.StatusOK || !strings.Contains(projects.Body.String(), filepath.Base(project)) {
		t.Fatalf("projects status=%d body=%s", projects.Code, projects.Body.String())
	}
	if rr := serveHTTP(remote, http.MethodPost, "/remote/api/projects", "{}", token); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST projects status=%d", rr.Code)
	}

	threads := serveHTTP(remote, http.MethodGet, "/remote/api/threads", "", token)
	if threads.Code != http.StatusOK || !strings.Contains(threads.Body.String(), `"threads"`) {
		t.Fatalf("threads status=%d body=%s", threads.Code, threads.Body.String())
	}

	newChat := serveHTTP(remote, http.MethodPost, "/remote/api/new-chat", jsonTestBody(t, map[string]any{"project": project}), token)
	if newChat.Code != http.StatusOK {
		t.Fatalf("new chat status=%d body=%s", newChat.Code, newChat.Body.String())
	}
	var created struct {
		Thread ChatThreadSummary `json:"thread"`
	}
	if err := json.Unmarshal(newChat.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Thread.ID == "" || created.Thread.Project != project {
		t.Fatalf("unexpected created thread: %#v", created.Thread)
	}
	if rr := serveHTTP(remote, http.MethodGet, "/remote/api/new-chat", "", token); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET new-chat status=%d", rr.Code)
	}

	second := serveHTTP(remote, http.MethodPost, "/remote/api/new-chat", jsonTestBody(t, map[string]any{"project": project}), token)
	if second.Code != http.StatusOK {
		t.Fatalf("second chat status=%d body=%s", second.Code, second.Body.String())
	}
	var createdSecond struct {
		Thread ChatThreadSummary `json:"thread"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &createdSecond); err != nil {
		t.Fatal(err)
	}

	selectFirst := serveHTTP(remote, http.MethodPost, "/remote/api/select-chat", jsonTestBody(t, map[string]any{"id": created.Thread.ID}), token)
	if selectFirst.Code != http.StatusOK {
		t.Fatalf("select chat status=%d body=%s", selectFirst.Code, selectFirst.Body.String())
	}
	if rr := serveHTTP(remote, http.MethodPost, "/remote/api/select-chat", `{"id":"missing"}`, token); rr.Code != http.StatusConflict {
		t.Fatalf("select missing chat status=%d", rr.Code)
	}

	snapshot := serveHTTP(remote, http.MethodGet, "/remote/api/snapshot", "", token)
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), created.Thread.ID) {
		t.Fatalf("snapshot status=%d body=%s", snapshot.Code, snapshot.Body.String())
	}
	historical := serveHTTP(remote, http.MethodGet, "/remote/api/snapshot?thread_id="+createdSecond.Thread.ID, "", token)
	if historical.Code != http.StatusOK || !strings.Contains(historical.Body.String(), `"running":false`) {
		t.Fatalf("historical snapshot status=%d body=%s", historical.Code, historical.Body.String())
	}

	state.mu.Lock()
	state.Running = true
	state.RunID = "coverage-run"
	state.RunPhase = "working"
	state.mu.Unlock()
	chat := serveHTTP(remote, http.MethodPost, "/remote/api/chat", jsonTestBody(t, map[string]any{
		"message": "do not start while already running", "model": "test-model", "project": project, "thread_id": created.Thread.ID,
	}), token)
	if chat.Code != http.StatusConflict {
		t.Fatalf("chat while running status=%d body=%s", chat.Code, chat.Body.String())
	}
	stop := serveHTTP(remote, http.MethodPost, "/remote/api/stop", "{}", token)
	if stop.Code != http.StatusOK || !strings.Contains(stop.Body.String(), `"was_running":true`) {
		t.Fatalf("stop status=%d body=%s", stop.Code, stop.Body.String())
	}

	if rr := serveHTTP(remote, http.MethodPost, "/remote/api/approve", `{"id":"missing","approve":true}`, token); rr.Code != http.StatusNotFound {
		t.Fatalf("missing approval status=%d", rr.Code)
	}
	pending := &PendingAction{ID: "p-invalid", Action: AgentAction{Action: "run_command", Message: "x", Command: "echo x"}, Result: make(chan ApprovalDecision, 1)}
	state.mu.Lock()
	state.Pending = pending
	state.mu.Unlock()
	if rr := serveHTTP(remote, http.MethodPost, "/remote/api/approve", `{"id":"p-invalid","decision":"nonsense"}`, token); rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid approval decision status=%d", rr.Code)
	}
	state.mu.Lock()
	state.Pending = nil
	state.mu.Unlock()

	bearerReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/remote/api/status", nil)
	bearerReq.Host = "127.0.0.1"
	bearerReq.Header.Set("Authorization", "Bearer "+token)
	bearerRR := httptest.NewRecorder()
	remote.ServeHTTP(bearerRR, bearerReq)
	if bearerRR.Code != http.StatusOK {
		t.Fatalf("Bearer authentication status=%d body=%s", bearerRR.Code, bearerRR.Body.String())
	}

	headPage := serveHTTP(remote, http.MethodHead, "/remote", "", "")
	if headPage.Code != http.StatusOK || headPage.Body.Len() != 0 {
		t.Fatalf("HEAD remote page status=%d body=%q", headPage.Code, headPage.Body.String())
	}
	if rr := serveHTTP(remote, http.MethodGet, "/not-remote", "", ""); rr.Code != http.StatusNotFound {
		t.Fatalf("unexpected remote path status=%d", rr.Code)
	}
}

func TestRemoteAddressAndOriginHelpers(t *testing.T) {
	cases := []struct {
		scheme string
		port   string
		want   string
	}{
		{"http", "", "80"}, {"https", "", "443"}, {"ftp", "", ""}, {"http", "1234", "1234"},
	}
	for _, tc := range cases {
		if got := effectiveOriginPort(tc.scheme, tc.port); got != tc.want {
			t.Fatalf("effectiveOriginPort(%q,%q)=%q want %q", tc.scheme, tc.port, got, tc.want)
		}
	}
	if !sameRequestOrigin("http://127.0.0.1", "127.0.0.1:80") {
		t.Fatal("default HTTP port should compare equal")
	}
	if sameRequestOrigin("https://127.0.0.1", "127.0.0.1:80") || sameRequestOrigin("file:///tmp/x", "127.0.0.1") {
		t.Fatal("mismatched/unsupported origins must be rejected")
	}
	if !containsString([]string{"a", "b"}, "b") || containsString([]string{"a"}, "z") {
		t.Fatal("containsString mismatch")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	urls := remoteURLsForListener(ln.Addr(), "127.0.0.1")
	if len(urls) != 1 || !strings.HasPrefix(urls[0], "http://127.0.0.1:") || !strings.HasSuffix(urls[0], "/remote") {
		t.Fatalf("unexpected remote URLs: %#v", urls)
	}
	for _, ip := range activeLANIPv4Addresses() {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() == nil || parsed.IsLoopback() || parsed.IsLinkLocalUnicast() {
			t.Fatalf("invalid LAN address returned: %q", ip)
		}
	}
	if urls, err := startRemoteHTTPServer(newRemoteTestState(t), Config{RemoteEnabled: false}); err != nil || urls != nil {
		t.Fatalf("disabled remote server should not listen: urls=%v err=%v", urls, err)
	}
}

func TestBinaryAssetParsersAndSafeWrites(t *testing.T) {
	if _, _, err := bmpDimensions([]byte{1, 2, 3}); err == nil {
		t.Fatal("short BMP must be rejected")
	}
	bmp := make([]byte, 26)
	binary.LittleEndian.PutUint32(bmp[18:22], uint32(320))
	negHeight := int32(-200)
	binary.LittleEndian.PutUint32(bmp[22:26], uint32(negHeight))
	w, h, err := bmpDimensions(bmp)
	if err != nil || w != 320 || h != 200 {
		t.Fatalf("BMP dimensions=%dx%d err=%v", w, h, err)
	}

	vp8x := make([]byte, 30)
	copy(vp8x[:4], "RIFF")
	copy(vp8x[8:12], "WEBP")
	copy(vp8x[12:16], "VP8X")
	binary.LittleEndian.PutUint32(vp8x[16:20], 10)
	vp8x[24] = 63 // width 64
	vp8x[27] = 31 // height 32
	w, h, err = webpDimensions(vp8x)
	if err != nil || w != 64 || h != 32 {
		t.Fatalf("VP8X dimensions=%dx%d err=%v", w, h, err)
	}
	if _, _, err := webpDimensions([]byte("not a webp")); err == nil {
		t.Fatal("invalid WebP signature must fail")
	}

	pngPayload := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 1, 2, 3, 4}
	ico := make([]byte, 22+len(pngPayload))
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)
	binary.LittleEndian.PutUint32(ico[14:18], uint32(len(pngPayload)))
	binary.LittleEndian.PutUint32(ico[18:22], 22)
	copy(ico[22:], pngPayload)
	gotPayload, err := icoPNGPayload(ico)
	if err != nil || string(gotPayload) != string(pngPayload) {
		t.Fatalf("ICO payload=%x err=%v", gotPayload, err)
	}
	badICO := append([]byte(nil), ico...)
	badICO[22] = 0
	if _, err := icoPNGPayload(badICO); err == nil {
		t.Fatal("non-PNG ICO payload must be rejected")
	}

	root := t.TempDir()
	post, err := writeBinaryProjectFile(root, "assets/data.bin", []byte("one"))
	if err != nil || !strings.Contains(post, "exists=true") {
		t.Fatalf("initial binary write post=%q err=%v", post, err)
	}
	post, err = writeBinaryProjectFile(root, "assets/data.bin", []byte("two"))
	if err != nil || !strings.Contains(post, "sha256=") {
		t.Fatalf("replacement binary write post=%q err=%v", post, err)
	}
	data, err := os.ReadFile(filepath.Join(root, "assets", "data.bin"))
	if err != nil || string(data) != "two" {
		t.Fatalf("binary content=%q err=%v", data, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := writeBinaryProjectFile(root, "folder", []byte("x")); err == nil {
		t.Fatal("writing binary bytes to a directory must fail")
	}
	if _, err := writeBinaryProjectFile(root, "../escape.bin", []byte("x")); err == nil {
		t.Fatal("binary write traversal must fail")
	}
}

func TestSkillShellQuotingAndMutationPaths(t *testing.T) {
	cfg := defaultConfig()
	cfg.TerminalShell = "powershell"
	if got := shellQuoteForConfiguredShell(cfg, "a'b"); got != "'a''b'" {
		t.Fatalf("PowerShell quote=%q", got)
	}
	if got := invocationForSkillScriptPath(cfg, `C:\\work dir\\tool.ps1`); !strings.HasPrefix(got, "& '") {
		t.Fatalf("PowerShell invocation=%q", got)
	}
	cfg.TerminalShell = "cmd"
	if got := shellQuoteForConfiguredShell(cfg, `a"b`); !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("cmd quote=%q", got)
	}
	if got := invocationForSkillScriptPath(cfg, `C:\\tool.cmd`); strings.HasPrefix(got, "& ") {
		t.Fatalf("cmd invocation must not use call operator: %q", got)
	}
	cfg.AgentEnvironment = "wsl"
	cfg.TerminalShell = "powershell"
	if got := shellQuoteForConfiguredShell(cfg, "a'b"); got != `'a'\''b'` {
		t.Fatalf("WSL quote=%q", got)
	}
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"scripts/build.ps1", true}, {"tool.cmd", true}, {"../tool.ps1", false}, {"a b.ps1", false}, {"echo;rm", false}, {"", false},
	} {
		if got := skillScriptLooksLikeRelativePath(tc.value); got != tc.want {
			t.Fatalf("skillScriptLooksLikeRelativePath(%q)=%v want %v", tc.value, got, tc.want)
		}
	}

	paths := mutatedActionPaths(AgentAction{Action: "copy_path", Source: "a.txt", Destination: "b.txt"})
	if len(paths) != 1 || paths[0] != "b.txt" {
		t.Fatalf("copy mutation paths=%#v", paths)
	}
	for _, action := range []AgentAction{
		{Action: "write_file", Path: "x.txt"}, {Action: "replace_text", Path: "x.txt"}, {Action: "delete_file", Path: "x.txt"},
		{Action: "move_path", Source: "a", Destination: "b"}, {Action: "create_svg_asset", Path: "x.svg"}, {Action: "generate_image_asset", Path: "x.png"},
	} {
		if len(mutatedActionPaths(action)) == 0 {
			t.Fatalf("expected mutation path for %#v", action)
		}
	}
}

func TestProjectAPKDiscoveryAndMemoryDirectList(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "app", "build", "outputs", "apk", "release", "app-release.apk"),
		filepath.Join(root, "app", "build", "outputs", "apk", "debug", "app-debug.apk"),
		filepath.Join(root, "app", "build", "outputs", "androidTest", "app-test.apk"),
		filepath.Join(root, "node_modules", "ignored.apk"),
		filepath.Join(root, "app", "bad-unaligned.apk"),
	}
	for i, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("apk"), 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().Add(time.Duration(i) * time.Second)
		_ = os.Chtimes(path, stamp, stamp)
	}
	apks := findAPKs(root)
	if len(apks) != 2 || !strings.Contains(strings.ToLower(apks[0]), "debug") {
		t.Fatalf("APK discovery=%#v", apks)
	}

	state := newRemoteTestState(t)
	project := state.Project
	state.mu.Lock()
	state.Config.Memories = []MemoryEntry{
		{ID: "1111111111111111", Scope: memoryScopeProject, Project: project, Content: "project note", CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "2222222222222222", Scope: memoryScopeGlobal, Content: "global note", CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	state.mu.Unlock()
	entries := filterMemoriesForDirectList(state, project)
	if len(entries) != 2 {
		t.Fatalf("direct memory list=%#v", entries)
	}
	out, err := state.executeDirectMemoryRequest(project, directMemoryRequest{Kind: "forget", Query: "no such memory"})
	if err != nil || !strings.Contains(out, "NO MATCHING MEMORIES") {
		t.Fatalf("direct forget no-match out=%q err=%v", out, err)
	}
	if _, err := state.executeDirectMemoryRequest(project, directMemoryRequest{Kind: "unsupported"}); err == nil {
		t.Fatal("unsupported direct memory request must fail")
	}
}

func TestAiderFormattingAndSafeServerErrorBranches(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "en"
	formatted := formatAiderRunResult(AiderRunResult{
		Output: "tool output", ChangedFiles: []string{"a.go", "b.go"}, BackupDir: "backup-1", Executable: "aider", Duration: 1250 * time.Millisecond, ExitCode: 0,
	}, cfg)
	for _, want := range []string{"Aider executable: aider", "a.go", "b.go", "AIDER OUTPUT", "tool output"} {
		if !strings.Contains(formatted, want) {
			t.Fatalf("formatted Aider result missing %q: %s", want, formatted)
		}
	}
	if got := (&AiderNotInstalledError{Status: AiderStatus{Error: "missing"}}).Error(); !strings.Contains(got, "missing") {
		t.Fatalf("Aider error=%q", got)
	}
	if got := (&AiderNotInstalledError{}).Error(); !strings.Contains(strings.ToLower(got), "nicht installiert") {
		t.Fatalf("default Aider error=%q", got)
	}

	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.AiderEnabled = false
	state.Config.AiderAutoInstall = false
	state.Config.CreateProjectDocs = false
	state.CurrentThread = "thread-coverage"
	state.Model = "fallback-model"
	state.mu.Unlock()
	thread, model := state.currentAiderThreadAndModel(state.Config)
	if thread != "thread-coverage" || model == "" {
		t.Fatalf("Aider thread/model=%q/%q", thread, model)
	}

	server := NewServer(state)
	if rr := serveHTTP(server, http.MethodPost, "/api/aider/status", "{}", ""); rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST aider status=%d", rr.Code)
	}
	if rr := serveHTTP(server, http.MethodPost, "/api/aider/setup", `{"action":"unsupported"}`, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("unsupported aider setup=%d body=%s", rr.Code, rr.Body.String())
	}
	state.mu.Lock()
	state.Running = true
	state.mu.Unlock()
	if rr := serveHTTP(server, http.MethodPost, "/api/aider/setup", `{"action":"test"}`, ""); rr.Code != http.StatusConflict {
		t.Fatalf("aider setup while running=%d", rr.Code)
	}
	state.mu.Lock()
	state.Running = false
	state.Project = ""
	state.LastAiderBackup = ""
	state.mu.Unlock()
	if rr := serveHTTP(server, http.MethodPost, "/api/aider/undo", "{}", ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("aider undo without project=%d body=%s", rr.Code, rr.Body.String())
	}
}
