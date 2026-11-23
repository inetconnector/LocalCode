// SPDX-License-Identifier: Apache-2.0

package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAndroidUpdateFeedURL = "https://api.github.com/repos/inetconnector/LocalCode/releases/latest"
	androidUpdateMaxAPKBytes    = 64 << 20 // 64 MiB
	androidUpdateFeedMaxBytes   = 512 << 10
	androidUpdateFeedCacheTTL   = 5 * time.Minute
)

type AndroidAPKMetadataSidecar struct {
	VersionName string `json:"version_name"`
	VersionCode int64  `json:"version_code"`
	SHA256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	BuiltAt     string `json:"built_at,omitempty"`
}

type AndroidUpdateCandidate struct {
	Source              string    `json:"source"` // windows_lan | external_release
	VersionName         string    `json:"version_name"`
	VersionCode         int64     `json:"version_code"`
	APKSHA256           string    `json:"apk_sha256"`
	APKSizeBytes        int64     `json:"apk_size_bytes"`
	LocalPath           string    `json:"-"`
	ExternalDownloadURL string    `json:"external_download_url,omitempty"`
	ReleaseNotes        string    `json:"release_notes,omitempty"`
	PublishedAt         time.Time `json:"published_at,omitempty"`
}

type AndroidUpdateResponse struct {
	OK                  bool   `json:"ok"`
	AutoUpdateEnabled   bool   `json:"auto_update_enabled"`
	APKAvailable        bool   `json:"apk_available"`
	UpdateAvailable     bool   `json:"update_available"`
	Source              string `json:"source,omitempty"`
	VersionName         string `json:"version_name,omitempty"`
	VersionCode         int64  `json:"version_code,omitempty"`
	APKSHA256           string `json:"apk_sha256,omitempty"`
	APKSizeBytes        int64  `json:"apk_size_bytes,omitempty"`
	DownloadURL         string `json:"download_url,omitempty"`
	ExternalDownloadURL string `json:"external_download_url,omitempty"`
	ReleaseNotes        string `json:"release_notes,omitempty"`
	PublishedAt         string `json:"published_at,omitempty"`
	CurrentVersionName  string `json:"current_version_name,omitempty"`
	CurrentVersionCode  int64  `json:"current_version_code,omitempty"`
}

var (
	androidExternalCacheMu         sync.Mutex
	androidExternalCacheURL        string
	androidExternalCacheAt         time.Time
	androidExternalCacheValue      *AndroidUpdateCandidate
	androidUpdateHTTPClient        = &http.Client{Timeout: 12 * time.Second}
	androidUpdateAllowLoopbackHTTP = false
	androidSemverPattern           = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
)

func androidAutoUpdateEnabled(cfg Config) bool {
	if cfg.RemoteAndroidAutoUpdate == nil {
		return true
	}
	return *cfg.RemoteAndroidAutoUpdate
}

func parseAndroidVersionCode(versionName string) int64 {
	m := androidSemverPattern.FindStringSubmatch(strings.TrimSpace(versionName))
	if len(m) != 4 {
		return 0
	}
	maj, err1 := strconv.ParseInt(m[1], 10, 64)
	min, err2 := strconv.ParseInt(m[2], 10, 64)
	pat, err3 := strconv.ParseInt(m[3], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0
	}
	return maj*10000 + min*100 + pat
}

func normalizeSemverLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "v")
	raw = strings.TrimPrefix(raw, "V")
	return strings.TrimSpace(raw)
}

func compareSemverStrings(a, b string) int {
	ma := androidSemverPattern.FindStringSubmatch(normalizeSemverLabel(a))
	mb := androidSemverPattern.FindStringSubmatch(normalizeSemverLabel(b))
	if len(ma) != 4 || len(mb) != 4 {
		return strings.Compare(normalizeSemverLabel(a), normalizeSemverLabel(b))
	}
	for i := 1; i <= 3; i++ {
		va, _ := strconv.Atoi(ma[i])
		vb, _ := strconv.Atoi(mb[i])
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

func compareAndroidCandidates(a, b *AndroidUpdateCandidate) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	if a.VersionCode != b.VersionCode {
		if a.VersionCode > b.VersionCode {
			return 1
		}
		return -1
	}
	if c := compareSemverStrings(a.VersionName, b.VersionName); c != 0 {
		return c
	}
	if a.PublishedAt.After(b.PublishedAt) {
		return 1
	}
	if b.PublishedAt.After(a.PublishedAt) {
		return -1
	}
	return 0
}

func isAndroidUpdateNewer(cand *AndroidUpdateCandidate, currentCode int64, currentName, currentSHA256 string) bool {
	if cand == nil || (cand.APKSHA256 == "" && cand.ExternalDownloadURL == "") {
		return false
	}
	if currentCode <= 0 && strings.TrimSpace(currentName) == "" && strings.TrimSpace(currentSHA256) == "" {
		return true
	}
	if cand.VersionCode > 0 && currentCode > 0 {
		if cand.VersionCode > currentCode {
			return true
		}
		if cand.VersionCode < currentCode {
			return false
		}
	}
	if strings.TrimSpace(cand.VersionName) != "" && strings.TrimSpace(currentName) != "" {
		cmp := compareSemverStrings(cand.VersionName, currentName)
		if cmp > 0 {
			return true
		}
		if cmp < 0 {
			return false
		}
	}
	// Same semantic version / code: if Windows built a newer signed APK (different SHA-256), offer it automatically.
	curHash := strings.ToLower(strings.TrimSpace(currentSHA256))
	if curHash != "" && cand.Source == "windows_lan" && !strings.EqualFold(cand.APKSHA256, curHash) {
		return true
	}
	return false
}

func validateAPKArchive(apkPath string) (string, int64, error) {
	st, err := os.Stat(apkPath)
	if err != nil {
		return "", 0, err
	}
	if st.IsDir() || st.Size() <= 0 || st.Size() > androidUpdateMaxAPKBytes {
		return "", 0, fmt.Errorf("invalid apk file size: %d", st.Size())
	}
	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		return "", 0, fmt.Errorf("invalid apk zip container: %w", err)
	}
	hasManifest := false
	hasDex := false
	for _, f := range zr.File {
		if f.Name == "AndroidManifest.xml" {
			hasManifest = true
		} else if f.Name == "classes.dex" {
			hasDex = true
		}
	}
	_ = zr.Close()
	if !hasManifest || !hasDex {
		return "", 0, errors.New("apk missing AndroidManifest.xml or classes.dex")
	}

	f, err := os.Open(apkPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func readVersionNearPath(apkPath string) string {
	dir := filepath.Dir(apkPath)
	for i := 0; i < 4; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, "VERSION.txt")); err == nil {
			if v := normalizeSemverLabel(string(data)); v != "" {
				return v
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if v := normalizeSemverLabel(version); parseAndroidVersionCode(v) > 0 {
		return v
	}
	return "1.0.0"
}

func inspectLocalAndroidAPK(apkPath string) (*AndroidUpdateCandidate, error) {
	hash, size, err := validateAPKArchive(apkPath)
	if err != nil {
		return nil, err
	}
	st, _ := os.Stat(apkPath)
	modTime := time.Now()
	if st != nil {
		modTime = st.ModTime()
	}

	vName := ""
	vCode := int64(0)
	if metaBytes, metaErr := os.ReadFile(apkPath + ".json"); metaErr == nil {
		var sidecar AndroidAPKMetadataSidecar
		if json.Unmarshal(metaBytes, &sidecar) == nil {
			if sidecar.SHA256 == "" || strings.EqualFold(strings.TrimSpace(sidecar.SHA256), hash) {
				vName = normalizeSemverLabel(sidecar.VersionName)
				vCode = sidecar.VersionCode
				if sidecar.BuiltAt != "" {
					if parsed, tErr := time.Parse(time.RFC3339, sidecar.BuiltAt); tErr == nil {
						modTime = parsed
					}
				}
			}
		}
	}
	if vName == "" {
		vName = readVersionNearPath(apkPath)
	}
	if vCode <= 0 {
		vCode = parseAndroidVersionCode(vName)
		if vCode <= 0 {
			vCode = 1
		}
	}
	return &AndroidUpdateCandidate{
		Source:       "windows_lan",
		VersionName:  vName,
		VersionCode:  vCode,
		APKSHA256:    hash,
		APKSizeBytes: size,
		LocalPath:    apkPath,
		PublishedAt:  modTime,
	}, nil
}

func candidateLocalAndroidAPKPaths(cfg Config) []string {
	explicit := strings.TrimSpace(os.Getenv("LOCALCODE_ANDROID_APK_PATH"))
	if explicit == "" {
		explicit = strings.TrimSpace(cfg.RemoteAndroidAPKPath)
	}
	if explicit != "" {
		return []string{filepath.Clean(explicit)}
	}

	out := make([]string, 0, 12)
	seen := map[string]bool{}
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		clean := filepath.Clean(p)
		key := strings.ToLower(clean)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, clean)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(filepath.Join(exeDir, "LocalCode-Remote-release.apk"))
		add(filepath.Join(exeDir, "LocalCode-Remote-debug.apk"))
		add(filepath.Join(exeDir, "LocalCode-Remote.apk"))
		add(filepath.Join(exeDir, "dist", "LocalCode-Remote-release.apk"))
		add(filepath.Join(exeDir, "dist", "LocalCode-Remote-debug.apk"))
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "dist", "LocalCode-Remote-release.apk"))
		add(filepath.Join(wd, "dist", "LocalCode-Remote-debug.apk"))
		add(filepath.Join(wd, "..", "dist", "LocalCode-Remote-release.apk"))
		add(filepath.Join(wd, "..", "dist", "LocalCode-Remote-debug.apk"))
	}
	cacheDir := filepath.Join(appDataDir(), "android-updates")
	if entries, err := os.ReadDir(cacheDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".apk") {
				add(filepath.Join(cacheDir, e.Name()))
			}
		}
	}
	return out
}

func discoverLocalAndroidAPK(cfg Config) *AndroidUpdateCandidate {
	var best *AndroidUpdateCandidate
	for _, p := range candidateLocalAndroidAPKPaths(cfg) {
		cand, err := inspectLocalAndroidAPK(p)
		if err != nil {
			continue
		}
		if best == nil || compareAndroidCandidates(cand, best) > 0 {
			best = cand
		}
	}
	return best
}

func isAllowedExternalUpdateURL(raw string, allowLoopbackHTTP bool) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil {
		return false
	}
	if strings.EqualFold(u.Scheme, "https") {
		return true
	}
	if allowLoopbackHTTP && strings.EqualFold(u.Scheme, "http") && loopbackRequestHost(u.Host) {
		return true
	}
	return false
}

func parseExternalUpdateFeed(body []byte, allowLoopbackHTTP bool) (*AndroidUpdateCandidate, error) {
	// Format A: Direct JSON manifest {version_name, version_code, apk_url, apk_sha256, release_notes, published_at}
	var direct struct {
		VersionName  string `json:"version_name"`
		VersionCode  int64  `json:"version_code"`
		APKURL       string `json:"apk_url"`
		DownloadURL  string `json:"download_url"`
		APKSHA256    string `json:"apk_sha256"`
		SHA256       string `json:"sha256"`
		APKSizeBytes int64  `json:"apk_size_bytes"`
		ReleaseNotes string `json:"release_notes"`
		PublishedAt  string `json:"published_at"`
	}
	if err := json.Unmarshal(body, &direct); err == nil {
		dl := strings.TrimSpace(direct.APKURL)
		if dl == "" {
			dl = strings.TrimSpace(direct.DownloadURL)
		}
		if dl != "" && isAllowedExternalUpdateURL(dl, allowLoopbackHTTP) {
			vName := normalizeSemverLabel(direct.VersionName)
			vCode := direct.VersionCode
			if vCode <= 0 {
				vCode = parseAndroidVersionCode(vName)
			}
			hash := strings.ToLower(strings.TrimSpace(direct.APKSHA256))
			if hash == "" {
				hash = strings.ToLower(strings.TrimSpace(direct.SHA256))
			}
			var pub time.Time
			if direct.PublishedAt != "" {
				pub, _ = time.Parse(time.RFC3339, direct.PublishedAt)
			}
			return &AndroidUpdateCandidate{
				Source:              "external_release",
				VersionName:         vName,
				VersionCode:         vCode,
				APKSHA256:           hash,
				APKSizeBytes:        direct.APKSizeBytes,
				ExternalDownloadURL: dl,
				ReleaseNotes:        strings.TrimSpace(direct.ReleaseNotes),
				PublishedAt:         pub,
			}, nil
		}
	}

	// Format B: GitHub Releases API JSON
	var gh struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		Assets      []struct {
			Name               string `json:"name"`
			Size               int64  `json:"size"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Digest             string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &gh); err != nil {
		return nil, err
	}
	for _, asset := range gh.Assets {
		lower := strings.ToLower(asset.Name)
		if !strings.HasSuffix(lower, ".apk") {
			continue
		}
		if !isAllowedExternalUpdateURL(asset.BrowserDownloadURL, allowLoopbackHTTP) {
			continue
		}
		vName := normalizeSemverLabel(gh.TagName)
		if vName == "" {
			vName = normalizeSemverLabel(gh.Name)
		}
		vCode := parseAndroidVersionCode(vName)
		hash := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
		var pub time.Time
		if gh.PublishedAt != "" {
			pub, _ = time.Parse(time.RFC3339, gh.PublishedAt)
		}
		return &AndroidUpdateCandidate{
			Source:              "external_release",
			VersionName:         vName,
			VersionCode:         vCode,
			APKSHA256:           hash,
			APKSizeBytes:        asset.Size,
			ExternalDownloadURL: asset.BrowserDownloadURL,
			ReleaseNotes:        strings.TrimSpace(gh.Body),
			PublishedAt:         pub,
		}, nil
	}
	return nil, errors.New("no apk asset found in external update feed")
}

func fetchExternalAndroidUpdate(ctx context.Context, feedURL string, client *http.Client, allowLoopbackHTTP bool) (*AndroidUpdateCandidate, error) {
	feedURL = strings.TrimSpace(feedURL)
	if feedURL == "" {
		feedURL = defaultAndroidUpdateFeedURL
	}
	if !isAllowedExternalUpdateURL(feedURL, allowLoopbackHTTP) {
		return nil, errors.New("external android update feed URL must use HTTPS")
	}

	androidExternalCacheMu.Lock()
	if androidExternalCacheURL == feedURL && time.Since(androidExternalCacheAt) < androidUpdateFeedCacheTTL {
		cached := androidExternalCacheValue
		androidExternalCacheMu.Unlock()
		return cached, nil
	}
	androidExternalCacheMu.Unlock()

	if client == nil {
		client = androidUpdateHTTPClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json, application/json")
	req.Header.Set("User-Agent", "LocalCode-Android-Updater/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("external update feed HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, androidUpdateFeedMaxBytes))
	if err != nil {
		return nil, err
	}
	cand, err := parseExternalUpdateFeed(body, allowLoopbackHTTP)
	if err != nil {
		return nil, err
	}
	androidExternalCacheMu.Lock()
	androidExternalCacheURL = feedURL
	androidExternalCacheAt = time.Now()
	androidExternalCacheValue = cand
	androidExternalCacheMu.Unlock()
	return cand, nil
}

func cacheExternalAndroidAPK(ctx context.Context, cand *AndroidUpdateCandidate, client *http.Client, allowLoopbackHTTP bool) (string, string, int64, error) {
	if cand == nil || !isAllowedExternalUpdateURL(cand.ExternalDownloadURL, allowLoopbackHTTP) {
		return "", "", 0, errors.New("invalid external apk download url")
	}
	cacheDir := filepath.Join(appDataDir(), "android-updates")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", "", 0, err
	}
	safeName := fmt.Sprintf("LocalCode-Remote-%d.apk", maxInt64(1, cand.VersionCode))
	dst := filepath.Join(cacheDir, safeName)
	if existingHash, existingSize, err := validateAPKArchive(dst); err == nil {
		if cand.APKSHA256 == "" || strings.EqualFold(existingHash, cand.APKSHA256) {
			return dst, existingHash, existingSize, nil
		}
	}

	if client == nil {
		client = androidUpdateHTTPClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cand.ExternalDownloadURL, nil)
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("User-Agent", "LocalCode-Android-Updater/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("external apk download HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(cacheDir, ".apk-dl-*")
	if err != nil {
		return "", "", 0, err
	}
	tmpName := tmp.Name()
	n, copyErr := io.Copy(tmp, io.LimitReader(resp.Body, androidUpdateMaxAPKBytes+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil || n > androidUpdateMaxAPKBytes {
		_ = os.Remove(tmpName)
		if n > androidUpdateMaxAPKBytes {
			return "", "", 0, errors.New("external apk exceeds maximum size")
		}
		if copyErr != nil {
			return "", "", 0, copyErr
		}
		return "", "", 0, closeErr
	}
	actualHash, actualSize, valErr := validateAPKArchive(tmpName)
	if valErr != nil {
		_ = os.Remove(tmpName)
		return "", "", 0, valErr
	}
	if cand.APKSHA256 != "" && !strings.EqualFold(actualHash, cand.APKSHA256) {
		_ = os.Remove(tmpName)
		return "", "", 0, fmt.Errorf("external apk sha256 mismatch: expected %s got %s", cand.APKSHA256, actualHash)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(dst)
		if err2 := os.Rename(tmpName, dst); err2 != nil {
			_ = os.Remove(tmpName)
			return "", "", 0, err2
		}
	}
	sidecar := AndroidAPKMetadataSidecar{
		VersionName: cand.VersionName,
		VersionCode: cand.VersionCode,
		SHA256:      actualHash,
		SizeBytes:   actualSize,
		BuiltAt:     time.Now().UTC().Format(time.RFC3339),
	}
	if metaBytes, err := json.MarshalIndent(sidecar, "", "  "); err == nil {
		_ = os.WriteFile(dst+".json", metaBytes, 0o644)
	}
	return dst, actualHash, actualSize, nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func resolveLatestAndroidUpdate(ctx context.Context, cfg Config, checkExternal bool, allowLoopbackHTTP bool) *AndroidUpdateCandidate {
	best := discoverLocalAndroidAPK(cfg)
	feedURL := strings.TrimSpace(os.Getenv("LOCALCODE_ANDROID_UPDATE_URL"))
	if feedURL == "" {
		feedURL = strings.TrimSpace(cfg.RemoteAndroidUpdateFeedURL)
	}
	if checkExternal || cfg.NetworkEnabled || feedURL != "" {
		if feedURL == "" {
			feedURL = defaultAndroidUpdateFeedURL
		}
		extCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		defer cancel()
		if ext, err := fetchExternalAndroidUpdate(extCtx, feedURL, androidUpdateHTTPClient, allowLoopbackHTTP); err == nil && ext != nil {
			if best == nil || compareAndroidCandidates(ext, best) > 0 {
				best = ext
			}
		}
	}
	return best
}

func buildAndroidUpdateResponse(ctx context.Context, cfg Config, r *http.Request) AndroidUpdateResponse {
	currentCode, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("current_version_code")), 10, 64)
	currentName := strings.TrimSpace(r.URL.Query().Get("current_version"))
	currentSHA := strings.TrimSpace(r.URL.Query().Get("current_sha256"))
	checkExternal := r.URL.Query().Get("check_external") == "1"

	cand := resolveLatestAndroidUpdate(ctx, cfg, checkExternal, androidUpdateAllowLoopbackHTTP)
	resp := AndroidUpdateResponse{
		OK:                 true,
		AutoUpdateEnabled:  androidAutoUpdateEnabled(cfg),
		CurrentVersionName: currentName,
		CurrentVersionCode: currentCode,
	}
	if cand == nil {
		return resp
	}
	resp.APKAvailable = true
	resp.Source = cand.Source
	resp.VersionName = cand.VersionName
	resp.VersionCode = cand.VersionCode
	resp.APKSHA256 = cand.APKSHA256
	resp.APKSizeBytes = cand.APKSizeBytes
	resp.DownloadURL = "/remote/api/android-update/download"
	resp.ExternalDownloadURL = cand.ExternalDownloadURL
	resp.ReleaseNotes = cand.ReleaseNotes
	if !cand.PublishedAt.IsZero() {
		resp.PublishedAt = cand.PublishedAt.UTC().Format(time.RFC3339)
	}
	resp.UpdateAvailable = isAndroidUpdateNewer(cand, currentCode, currentName, currentSHA)
	return resp
}

func (s *RemoteServer) handleAndroidUpdateInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, buildAndroidUpdateResponse(r.Context(), cfg, r))
}

func (s *RemoteServer) handleAndroidUpdateDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	cand := resolveLatestAndroidUpdate(r.Context(), cfg, r.URL.Query().Get("check_external") == "1", androidUpdateAllowLoopbackHTTP)
	if cand == nil {
		http.Error(w, "no android apk available", http.StatusNotFound)
		return
	}
	apkPath := cand.LocalPath
	hash := cand.APKSHA256
	size := cand.APKSizeBytes
	if apkPath == "" && cand.ExternalDownloadURL != "" {
		dlPath, dlHash, dlSize, err := cacheExternalAndroidAPK(r.Context(), cand, androidUpdateHTTPClient, androidUpdateAllowLoopbackHTTP)
		if err != nil {
			http.Error(w, "failed to download android apk: "+err.Error(), http.StatusBadGateway)
			return
		}
		apkPath = dlPath
		hash = dlHash
		size = dlSize
	}
	if apkPath == "" {
		http.Error(w, "android apk path unavailable", http.StatusNotFound)
		return
	}
	f, err := os.Open(apkPath)
	if err != nil {
		http.Error(w, "android apk unreadable", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="LocalCode-Remote.apk"`)
	w.Header().Set("X-LocalCode-APK-SHA256", hash)
	w.Header().Set("X-LocalCode-APK-Version", cand.VersionName)
	w.Header().Set("X-LocalCode-APK-Version-Code", strconv.FormatInt(cand.VersionCode, 10))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, f)
}

func (s *Server) handleDesktopAndroidUpdateInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	cfg := s.state.Config
	s.state.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, buildAndroidUpdateResponse(r.Context(), cfg, r))
}
