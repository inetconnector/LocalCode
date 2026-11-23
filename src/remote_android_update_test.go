// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func buildMinimalTestAPK(t *testing.T, payload string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"AndroidManifest.xml": "manifest-" + payload,
		"classes.dex":         "dex-" + payload,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func resetExternalAndroidUpdateCacheForTest() {
	androidExternalCacheMu.Lock()
	androidExternalCacheURL = ""
	androidExternalCacheAt = time.Time{}
	androidExternalCacheValue = nil
	androidExternalCacheMu.Unlock()
}

func TestAndroidUpdateVersionComparisonAndHashDetection(t *testing.T) {
	if got := parseAndroidVersionCode("v1.2.3"); got != 10203 {
		t.Fatalf("expected 10203, got %d", got)
	}
	if got := parseAndroidVersionCode("invalid"); got != 0 {
		t.Fatalf("expected 0 for invalid version, got %d", got)
	}
	if compareSemverStrings("1.10.0", "1.2.9") <= 0 {
		t.Fatal("expected 1.10.0 > 1.2.9")
	}

	lanCand := &AndroidUpdateCandidate{
		Source:      "windows_lan",
		VersionName: "1.4.2",
		VersionCode: 10402,
		APKSHA256:   strings.Repeat("a", 64),
	}
	if !isAndroidUpdateNewer(lanCand, 10401, "1.4.1", strings.Repeat("b", 64)) {
		t.Fatal("expected higher versionCode to trigger update")
	}
	if isAndroidUpdateNewer(lanCand, 10403, "1.4.3", strings.Repeat("b", 64)) {
		t.Fatal("expected lower versionCode NOT to downgrade")
	}
	if isAndroidUpdateNewer(lanCand, 10402, "1.4.2", strings.Repeat("a", 64)) {
		t.Fatal("expected identical version and SHA-256 NOT to trigger update")
	}
	if !isAndroidUpdateNewer(lanCand, 10402, "1.4.2", strings.Repeat("c", 64)) {
		t.Fatal("expected rebuilt Windows LAN APK with newer SHA-256 to trigger automatic update")
	}

	extCandNoDigest := &AndroidUpdateCandidate{
		Source:              "external_release",
		VersionName:         "2.0.0",
		VersionCode:         20000,
		ExternalDownloadURL: "https://github.com/inetconnector/LocalCode/releases/download/v2.0.0/LocalCode-Remote.apk",
	}
	if !isAndroidUpdateNewer(extCandNoDigest, 10402, "1.4.2", strings.Repeat("a", 64)) {
		t.Fatal("expected higher external release without pre-declared digest to trigger update")
	}
	if isAndroidUpdateNewer(extCandNoDigest, 20000, "2.0.0", strings.Repeat("a", 64)) {
		t.Fatal("expected same-version external release without hash mismatch NOT to loop")
	}
}

func TestAndroidUpdateLocalAPKDiscoveryAndValidation(t *testing.T) {
	dir := t.TempDir()
	apkPath := filepath.Join(dir, "LocalCode-Remote-debug.apk")
	raw, wantHash := buildMinimalTestAPK(t, "v1.9.5")
	if err := os.WriteFile(apkPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar := AndroidAPKMetadataSidecar{
		VersionName: "1.9.5",
		VersionCode: 10905,
		SHA256:      wantHash,
		SizeBytes:   int64(len(raw)),
		BuiltAt:     "2026-11-23T20:00:00Z",
	}
	metaBytes, _ := json.Marshal(sidecar)
	if err := os.WriteFile(apkPath+".json", metaBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	cand, err := inspectLocalAndroidAPK(apkPath)
	if err != nil {
		t.Fatalf("inspectLocalAndroidAPK failed: %v", err)
	}
	if cand.VersionName != "1.9.5" || cand.VersionCode != 10905 || cand.APKSHA256 != wantHash || cand.APKSizeBytes != int64(len(raw)) {
		t.Fatalf("unexpected candidate metadata: %+v", cand)
	}

	// APK without sidecar falls back to VERSION.txt in parent/same directory
	noSidecarDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(noSidecarDir, "VERSION.txt"), []byte("4.2.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noSidecarAPK := filepath.Join(noSidecarDir, "LocalCode-Remote-debug.apk")
	raw2, hash2 := buildMinimalTestAPK(t, "v4.2.1")
	if err := os.WriteFile(noSidecarAPK, raw2, 0o644); err != nil {
		t.Fatal(err)
	}
	cand2, err := inspectLocalAndroidAPK(noSidecarAPK)
	if err != nil {
		t.Fatalf("inspectLocalAndroidAPK without sidecar failed: %v", err)
	}
	if cand2.VersionName != "4.2.1" || cand2.VersionCode != 40201 || cand2.APKSHA256 != hash2 {
		t.Fatalf("expected VERSION.txt fallback 4.2.1/40201, got %+v", cand2)
	}

	// Default candidate paths include dist & appDataDir when no explicit override is set
	paths := candidateLocalAndroidAPKPaths(defaultConfig())
	if len(paths) == 0 {
		t.Fatal("expected non-empty default APK candidate paths")
	}

	// Explicit disable of auto-update via config
	disabled := false
	cfgDisabled := defaultConfig()
	cfgDisabled.RemoteAndroidAutoUpdate = &disabled
	if androidAutoUpdateEnabled(cfgDisabled) {
		t.Fatal("expected androidAutoUpdateEnabled=false when explicitly disabled")
	}

	// Reject invalid non-APK archive (missing classes.dex)
	badAPK := filepath.Join(dir, "corrupt.apk")
	var badBuf bytes.Buffer
	zw := zip.NewWriter(&badBuf)
	w, _ := zw.Create("AndroidManifest.xml")
	_, _ = w.Write([]byte("only-manifest"))
	_ = zw.Close()
	if err := os.WriteFile(badAPK, badBuf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectLocalAndroidAPK(badAPK); err == nil {
		t.Fatal("expected archive missing classes.dex to be rejected")
	}
}

func TestAndroidUpdateRemoteAndDesktopEndpoints(t *testing.T) {
	resetExternalAndroidUpdateCacheForTest()
	t.Cleanup(resetExternalAndroidUpdateCacheForTest)

	dir := t.TempDir()
	apkPath := filepath.Join(dir, "LocalCode-Remote-release.apk")
	raw, wantHash := buildMinimalTestAPK(t, "endpoint-2.5.0")
	if err := os.WriteFile(apkPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar, _ := json.Marshal(AndroidAPKMetadataSidecar{
		VersionName: "2.5.0",
		VersionCode: 20500,
		SHA256:      wantHash,
		SizeBytes:   int64(len(raw)),
	})
	if err := os.WriteFile(apkPath+".json", sidecar, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := defaultConfig()
	cfg.RemoteEnabled = true
	cfg.NetworkEnabled = false
	cfg.RemoteAndroidAPKPath = apkPath
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)

	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "Test Android")
	if err != nil {
		t.Fatal(err)
	}

	rs := NewRemoteServer(state)
	var handler http.Handler = rs

	// 1. Unauthenticated access must fail with 401
	unauthReq := httptest.NewRequest(http.MethodGet, "/remote/api/android-update", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauthRec.Code)
	}

	// 2. Authenticated older client sees update_available=true
	infoReq := httptest.NewRequest(http.MethodGet, "/remote/api/android-update?current_version_code=10000&current_version=1.0.0", nil)
	infoReq.Header.Set("X-LocalCode-Remote-Token", token)
	infoRec := httptest.NewRecorder()
	handler.ServeHTTP(infoRec, infoReq)
	if infoRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for update info, got %d: %s", infoRec.Code, infoRec.Body.String())
	}
	var resp AndroidUpdateResponse
	if err := json.Unmarshal(infoRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode update info: %v", err)
	}
	if !resp.OK || !resp.AutoUpdateEnabled || !resp.APKAvailable || !resp.UpdateAvailable {
		t.Fatalf("expected update available response, got %+v", resp)
	}
	if resp.Source != "windows_lan" || resp.VersionName != "2.5.0" || resp.VersionCode != 20500 || resp.APKSHA256 != wantHash {
		t.Fatalf("unexpected update metadata: %+v", resp)
	}

	// 3. Authenticated up-to-date client sees update_available=false
	sameReq := httptest.NewRequest(http.MethodGet, "/remote/api/android-update?current_version_code=20500&current_version=2.5.0&current_sha256="+wantHash, nil)
	sameReq.Header.Set("X-LocalCode-Remote-Token", token)
	sameRec := httptest.NewRecorder()
	handler.ServeHTTP(sameRec, sameReq)
	var sameResp AndroidUpdateResponse
	_ = json.Unmarshal(sameRec.Body.Bytes(), &sameResp)
	if sameResp.UpdateAvailable {
		t.Fatalf("expected up-to-date client not to trigger update, got %+v", sameResp)
	}

	// 4. Authenticated APK download streams exact bytes + SHA-256 header
	dlReq := httptest.NewRequest(http.MethodGet, "/remote/api/android-update/download", nil)
	dlReq.Header.Set("X-LocalCode-Remote-Token", token)
	dlRec := httptest.NewRecorder()
	handler.ServeHTTP(dlRec, dlReq)
	if dlRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on APK download, got %d", dlRec.Code)
	}
	if dlRec.Header().Get("X-LocalCode-APK-SHA256") != wantHash {
		t.Fatalf("expected SHA256 header %s, got %s", wantHash, dlRec.Header().Get("X-LocalCode-APK-SHA256"))
	}
	if !bytes.Equal(dlRec.Body.Bytes(), raw) {
		t.Fatalf("downloaded APK bytes mismatch (%d vs %d)", dlRec.Body.Len(), len(raw))
	}

	// 4b. HEAD request returns headers with empty body; POST returns 405
	headReq := httptest.NewRequest(http.MethodHead, "/remote/api/android-update/download", nil)
	headReq.Header.Set("X-LocalCode-Remote-Token", token)
	headRec := httptest.NewRecorder()
	handler.ServeHTTP(headRec, headReq)
	if headRec.Code != http.StatusOK || headRec.Body.Len() != 0 || headRec.Header().Get("X-LocalCode-APK-SHA256") != wantHash {
		t.Fatalf("unexpected HEAD response: code=%d len=%d sha=%s", headRec.Code, headRec.Body.Len(), headRec.Header().Get("X-LocalCode-APK-SHA256"))
	}
	postReq := httptest.NewRequest(http.MethodPost, "/remote/api/android-update", nil)
	postReq.Header.Set("X-LocalCode-Remote-Token", token)
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST /remote/api/android-update, got %d", postRec.Code)
	}

	// 5. Desktop inspection endpoint returns APK status
	srv := NewServer(state)
	deskReq := httptest.NewRequest(http.MethodGet, "/api/remote/android-update", nil)
	deskRec := httptest.NewRecorder()
	srv.handleDesktopAndroidUpdateInfo(deskRec, deskReq)
	if deskRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on desktop update info, got %d", deskRec.Code)
	}
	var deskResp AndroidUpdateResponse
	if err := json.Unmarshal(deskRec.Body.Bytes(), &deskResp); err != nil {
		t.Fatal(err)
	}
	if !deskResp.APKAvailable || deskResp.APKSHA256 != wantHash {
		t.Fatalf("unexpected desktop update response: %+v", deskResp)
	}
}

func TestAndroidUpdateExternalFeedsProxyAndSHA256Verification(t *testing.T) {
	resetExternalAndroidUpdateCacheForTest()
	t.Cleanup(resetExternalAndroidUpdateCacheForTest)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Non-HTTPS public URLs must always be rejected
	if isAllowedExternalUpdateURL("http://example.com/update.json", false) {
		t.Fatal("plain HTTP public URL must be rejected")
	}
	if !isAllowedExternalUpdateURL("https://api.github.com/repos/inetconnector/LocalCode/releases/latest", false) {
		t.Fatal("HTTPS external feed URL must be allowed")
	}

	rawAPK, wantHash := buildMinimalTestAPK(t, "external-3.1.0")
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/direct.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version_name":  "3.1.0",
			"version_code":  30100,
			"apk_url":       baseURL + "/app.apk",
			"apk_sha256":    wantHash,
			"release_notes": "External feed release",
		})
	})
	mux.HandleFunc("/github.json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name":     "v3.2.0",
			"body":         "GitHub Release APK",
			"published_at": "2026-11-23T21:00:00Z",
			"assets": []map[string]any{
				{
					"name":                 "LocalCode-Remote-v3.2.0.apk",
					"size":                 len(rawAPK),
					"browser_download_url": baseURL + "/app.apk",
					"digest":               "sha256:" + wantHash,
				},
			},
		})
	})
	mux.HandleFunc("/app.apk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		_, _ = w.Write(rawAPK)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	baseURL = ts.URL

	// 1. Parse GitHub Releases feed
	ghCand, err := fetchExternalAndroidUpdate(context.Background(), ts.URL+"/github.json", ts.Client(), true)
	if err != nil {
		t.Fatalf("fetchExternalAndroidUpdate github failed: %v", err)
	}
	if ghCand.Source != "external_release" || ghCand.VersionName != "3.2.0" || ghCand.VersionCode != 30200 || ghCand.APKSHA256 != wantHash {
		t.Fatalf("unexpected github candidate: %+v", ghCand)
	}

	// 2. End-to-end RemoteServer download proxy caches verified external APK
	resetExternalAndroidUpdateCacheForTest()
	prevAllow := androidUpdateAllowLoopbackHTTP
	prevClient := androidUpdateHTTPClient
	androidUpdateAllowLoopbackHTTP = true
	androidUpdateHTTPClient = ts.Client()
	t.Cleanup(func() {
		androidUpdateAllowLoopbackHTTP = prevAllow
		androidUpdateHTTPClient = prevClient
	})

	cfg := defaultConfig()
	cfg.RemoteEnabled = true
	cfg.RemoteAndroidAPKPath = filepath.Join(t.TempDir(), "nonexistent.apk")
	cfg.RemoteAndroidUpdateFeedURL = ts.URL + "/direct.json"
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "External Test Phone")
	if err != nil {
		t.Fatal(err)
	}
	rs := NewRemoteServer(state)
	dlReq := httptest.NewRequest(http.MethodGet, "/remote/api/android-update/download?check_external=1", nil)
	dlReq.Header.Set("X-LocalCode-Remote-Token", token)
	dlRec := httptest.NewRecorder()
	rs.ServeHTTP(dlRec, dlReq)
	if dlRec.Code != http.StatusOK {
		t.Fatalf("expected 200 proxying external APK, got %d: %s", dlRec.Code, dlRec.Body.String())
	}
	if !bytes.Equal(dlRec.Body.Bytes(), rawAPK) || dlRec.Header().Get("X-LocalCode-APK-SHA256") != wantHash {
		t.Fatalf("proxied external APK content or SHA-256 mismatch")
	}

	// 3. Tampered APK (wrong expected SHA-256) MUST be rejected
	badCand := &AndroidUpdateCandidate{
		Source:              "external_release",
		VersionName:         "9.9.9",
		VersionCode:         90909,
		APKSHA256:           strings.Repeat("0", 64),
		ExternalDownloadURL: ts.URL + "/app.apk",
	}
	if _, _, _, err := cacheExternalAndroidAPK(context.Background(), badCand, ts.Client(), true); err == nil {
		t.Fatal("expected SHA-256 mismatch to reject external APK")
	}
}

func TestAndroidUpdateEdgeCasesAndCandidateRanking(t *testing.T) {
	resetExternalAndroidUpdateCacheForTest()
	t.Cleanup(resetExternalAndroidUpdateCacheForTest)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// 1. androidAutoUpdateEnabled default nil -> true
	if !androidAutoUpdateEnabled(Config{}) {
		t.Fatal("expected nil RemoteAndroidAutoUpdate to default to true")
	}

	// 2. compareAndroidCandidates exhaustive branches
	t1 := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	cLow := &AndroidUpdateCandidate{VersionCode: 10000, VersionName: "1.0.0", PublishedAt: t1}
	cHigh := &AndroidUpdateCandidate{VersionCode: 20000, VersionName: "2.0.0", PublishedAt: t1}
	cSameCodeNewerName := &AndroidUpdateCandidate{VersionCode: 10000, VersionName: "1.0.1", PublishedAt: t1}
	cSameVersionNewerTime := &AndroidUpdateCandidate{VersionCode: 10000, VersionName: "1.0.0", PublishedAt: t2}

	if compareAndroidCandidates(nil, nil) != 0 || compareAndroidCandidates(nil, cLow) >= 0 || compareAndroidCandidates(cLow, nil) <= 0 {
		t.Fatal("nil candidate comparisons failed")
	}
	if compareAndroidCandidates(cHigh, cLow) <= 0 || compareAndroidCandidates(cLow, cHigh) >= 0 {
		t.Fatal("versionCode candidate comparisons failed")
	}
	if compareAndroidCandidates(cSameCodeNewerName, cLow) <= 0 || compareAndroidCandidates(cLow, cSameCodeNewerName) >= 0 {
		t.Fatal("versionName tie-breaker failed")
	}
	if compareAndroidCandidates(cSameVersionNewerTime, cLow) <= 0 || compareAndroidCandidates(cLow, cSameVersionNewerTime) >= 0 {
		t.Fatal("publishedAt tie-breaker failed")
	}
	if compareAndroidCandidates(cLow, cLow) != 0 {
		t.Fatal("equal candidate comparison must return 0")
	}

	// 3. isAndroidUpdateNewer empty-client & semver-only branches
	if isAndroidUpdateNewer(nil, 1, "1.0.0", "") {
		t.Fatal("nil candidate must not be newer")
	}
	if !isAndroidUpdateNewer(&AndroidUpdateCandidate{VersionName: "1.2.0", APKSHA256: "abc"}, 0, "", "") {
		t.Fatal("empty client metadata must accept available candidate")
	}
	if !isAndroidUpdateNewer(&AndroidUpdateCandidate{VersionName: "1.3.0", APKSHA256: "abc"}, 0, "1.2.0", "") {
		t.Fatal("higher semver without versionCode must be newer")
	}
	if isAndroidUpdateNewer(&AndroidUpdateCandidate{VersionName: "1.1.0", APKSHA256: "abc"}, 0, "1.2.0", "") {
		t.Fatal("lower semver without versionCode must not be newer")
	}
	if compareSemverStrings("alpha", "beta") >= 0 {
		t.Fatal("non-semver fallback string comparison failed")
	}

	// 4. readVersionNearPath fallback when no VERSION.txt exists
	isolatedDir := t.TempDir()
	fallbackVer := readVersionNearPath(filepath.Join(isolatedDir, "app.apk"))
	if fallbackVer == "" {
		t.Fatal("expected non-empty fallback version")
	}

	// 5. parseExternalUpdateFeed aliases & error cases
	directAliasJSON := []byte(`{"version_name":"v5.1.0","download_url":"https://example.com/a.apk","sha256":"deadbeef","published_at":"2026-11-23T12:00:00Z"}`)
	aliasCand, err := parseExternalUpdateFeed(directAliasJSON, false)
	if err != nil || aliasCand.VersionCode != 50100 || aliasCand.APKSHA256 != "deadbeef" || aliasCand.PublishedAt.IsZero() {
		t.Fatalf("unexpected alias feed parse: cand=%+v err=%v", aliasCand, err)
	}
	if _, err := parseExternalUpdateFeed([]byte(`{"tag_name":"v1.0.0","assets":[{"name":"notes.txt","browser_download_url":"https://example.com/notes.txt"}]}`), false); err == nil {
		t.Fatal("expected feed without apk asset to fail")
	}
	if _, err := parseExternalUpdateFeed([]byte(`not-json`), false); err == nil {
		t.Fatal("expected malformed feed JSON to fail")
	}

	// 6. fetchExternalAndroidUpdate & cacheExternalAndroidAPK error/cache paths
	rawAPK, wantHash := buildMinimalTestAPK(t, "cache-hit-4.0.0")
	var baseURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/ok-feed", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version_name": "4.0.0",
			"version_code": 40000,
			"apk_url":      baseURL + "/ok.apk",
			"apk_sha256":   wantHash,
		})
	})
	mux.HandleFunc("/ok.apk", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(rawAPK)
	})
	mux.HandleFunc("/bad.apk", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-a-zip"))
	})
	mux.HandleFunc("/err", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	baseURL = ts.URL

	if _, err := fetchExternalAndroidUpdate(context.Background(), "http://example.com/insecure.json", ts.Client(), false); err == nil {
		t.Fatal("expected insecure external feed URL error")
	}
	if _, err := fetchExternalAndroidUpdate(context.Background(), ts.URL+"/err", ts.Client(), true); err == nil {
		t.Fatal("expected HTTP 500 feed error")
	}
	c1, err := fetchExternalAndroidUpdate(context.Background(), ts.URL+"/ok-feed", ts.Client(), true)
	if err != nil {
		t.Fatalf("first fetchExternalAndroidUpdate failed: %v", err)
	}
	// Second fetch hits in-memory cache
	c2, err := fetchExternalAndroidUpdate(context.Background(), ts.URL+"/ok-feed", ts.Client(), true)
	if err != nil || c2.VersionCode != c1.VersionCode {
		t.Fatalf("cached fetchExternalAndroidUpdate failed: %+v err=%v", c2, err)
	}

	// First cacheExternalAndroidAPK downloads; second hits on-disk cache
	p1, h1, s1, err := cacheExternalAndroidAPK(context.Background(), c1, ts.Client(), true)
	if err != nil || p1 == "" || h1 != wantHash || s1 != int64(len(rawAPK)) {
		t.Fatalf("cacheExternalAndroidAPK download failed: %s %s %d err=%v", p1, h1, s1, err)
	}
	p2, h2, _, err := cacheExternalAndroidAPK(context.Background(), c1, ts.Client(), true)
	if err != nil || p2 != p1 || h2 != wantHash {
		t.Fatalf("cacheExternalAndroidAPK cache hit failed: %s %s err=%v", p2, h2, err)
	}
	if _, _, _, err := cacheExternalAndroidAPK(context.Background(), nil, ts.Client(), true); err == nil {
		t.Fatal("expected nil candidate error")
	}
	if _, _, _, err := cacheExternalAndroidAPK(context.Background(), &AndroidUpdateCandidate{VersionCode: 40001, ExternalDownloadURL: ts.URL + "/err"}, ts.Client(), true); err == nil {
		t.Fatal("expected HTTP 500 apk download error")
	}
	if _, _, _, err := cacheExternalAndroidAPK(context.Background(), &AndroidUpdateCandidate{VersionCode: 40002, ExternalDownloadURL: ts.URL + "/bad.apk"}, ts.Client(), true); err == nil {
		t.Fatal("expected invalid zip apk download error")
	}
	if maxInt64(5, 2) != 5 || maxInt64(1, 9) != 9 {
		t.Fatal("maxInt64 failed")
	}

	// 7. Handler error paths: 405 & 404 on remote download + 405 on desktop info + 502 on external download failure
	prevAllow := androidUpdateAllowLoopbackHTTP
	prevClient := androidUpdateHTTPClient
	androidUpdateAllowLoopbackHTTP = true
	androidUpdateHTTPClient = ts.Client()
	t.Cleanup(func() {
		androidUpdateAllowLoopbackHTTP = prevAllow
		androidUpdateHTTPClient = prevClient
	})

	cfg := defaultConfig()
	cfg.RemoteEnabled = true
	cfg.NetworkEnabled = false
	cfg.RemoteAndroidAPKPath = filepath.Join(t.TempDir(), "missing.apk")
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	code, _, _, _ := state.StartRemotePairing()
	token, _, _ := state.PairRemoteDevice(code, "Edge Phone")
	rs := NewRemoteServer(state)

	postDl := httptest.NewRequest(http.MethodPost, "/remote/api/android-update/download", nil)
	postDl.Header.Set("X-LocalCode-Remote-Token", token)
	postDlRec := httptest.NewRecorder()
	rs.ServeHTTP(postDlRec, postDl)
	if postDlRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST download, got %d", postDlRec.Code)
	}

	notFoundDl := httptest.NewRequest(http.MethodGet, "/remote/api/android-update/download", nil)
	notFoundDl.Header.Set("X-LocalCode-Remote-Token", token)
	notFoundRec := httptest.NewRecorder()
	rs.ServeHTTP(notFoundRec, notFoundDl)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when no APK available, got %d", notFoundRec.Code)
	}

	srv := NewServer(state)
	deskPost := httptest.NewRequest(http.MethodPost, "/api/remote/android-update", nil)
	deskPostRec := httptest.NewRecorder()
	srv.handleDesktopAndroidUpdateInfo(deskPostRec, deskPost)
	if deskPostRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST desktop android-update, got %d", deskPostRec.Code)
	}
}

func TestAndroidAutoUpdateContractsAcrossAndroidAndRemoteUI(t *testing.T) {
	manifestBytes, err := os.ReadFile(filepath.Join("..", "android", "app", "src", "main", "AndroidManifest.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestBytes), `android.permission.REQUEST_INSTALL_PACKAGES`) {
		t.Fatal("AndroidManifest.xml must declare REQUEST_INSTALL_PACKAGES for self-updates")
	}

	javaBytes, err := os.ReadFile(filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "MainActivity.java"))
	if err != nil {
		t.Fatal(err)
	}
	javaText := string(javaBytes)
	for _, required := range []string{
		`PackageInstaller.SessionParams.MODE_FULL_INSTALL`,
		`PackageInstaller.SessionParams.USER_ACTION_NOT_REQUIRED`,
		`Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES`,
		`pinnedLanSSLContext`,
		`APK SHA-256 checksum mismatch`,
		`public void syncRemoteToken(String token)`,
		`public void checkForAppUpdate(String token, boolean force)`,
		`public String getInstalledAppVersion()`,
		`/remote/api/android-update`,
		`DEFAULT_EXTERNAL_UPDATE_FEED`,
	} {
		if !strings.Contains(javaText, required) {
			t.Fatalf("MainActivity.java auto-update contract missing %q", required)
		}
	}

	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	remoteText := string(remoteBytes)
	for _, required := range []string{
		`function syncAndroidUpdateToken()`,
		`window.localCodeUpdateStatus=`,
		`async function checkAppUpdateNow()`,
		`id="checkAppUpdateBtn"`,
		`id="settingsAppVersion"`,
		`id="updateBanner"`,
	} {
		if !strings.Contains(remoteText, required) {
			t.Fatalf("remote.html auto-update contract missing %q", required)
		}
	}

	buildScriptBytes, err := os.ReadFile(filepath.Join("..", "scripts", "build-android.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	buildScript := string(buildScriptBytes)
	for _, required := range []string{
		`android-debug.keystore`,
		`VERSION.txt`,
		`--version-code`,
		`--version-name`,
		`.json`,
	} {
		if !strings.Contains(buildScript, required) {
			t.Fatalf("scripts/build-android.ps1 auto-update signing/versioning contract missing %q", required)
		}
	}
}
