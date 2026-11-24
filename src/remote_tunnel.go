// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	remoteTunnelMu      sync.Mutex
	remoteTunnelCmd     *exec.Cmd
	remoteTunnelURL     string
	remoteTunnelStarted time.Time
	tryCloudflareRe     = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)
)

func findCloudflaredBinary() string {
	candidates := []string{
		`C:\Program Files (x86)\cloudflared\cloudflared.exe`,
		`C:\Program Files\cloudflared\cloudflared.exe`,
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("cloudflared"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	return ""
}

func currentRemoteTunnelURL() string {
	remoteTunnelMu.Lock()
	defer remoteTunnelMu.Unlock()
	if remoteTunnelCmd == nil || remoteTunnelCmd.Process == nil {
		return ""
	}
	if remoteTunnelCmd.ProcessState != nil && remoteTunnelCmd.ProcessState.Exited() {
		remoteTunnelCmd = nil
		remoteTunnelURL = ""
		return ""
	}
	return remoteTunnelURL
}

func stopRemoteMobileTunnel() {
	remoteTunnelMu.Lock()
	cmd := remoteTunnelCmd
	remoteTunnelCmd = nil
	remoteTunnelURL = ""
	remoteTunnelMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func ensureRemoteMobileTunnel(ctx context.Context, remotePort int) (string, error) {
	if existing := currentRemoteTunnelURL(); existing != "" {
		return existing, nil
	}
	bin := findCloudflaredBinary()
	if bin == "" {
		return "", errors.New("cloudflared nicht installiert (benötigt für 4G/5G Mobildaten-Tunnel)")
	}
	if remotePort <= 0 {
		remotePort = 32146
	}
	targetURL := fmt.Sprintf("https://127.0.0.1:%d", remotePort)

	remoteTunnelMu.Lock()
	if remoteTunnelCmd != nil && remoteTunnelCmd.Process != nil && remoteTunnelURL != "" {
		u := remoteTunnelURL
		remoteTunnelMu.Unlock()
		return u, nil
	}
	if remoteTunnelCmd != nil && remoteTunnelCmd.Process != nil {
		_ = remoteTunnelCmd.Process.Kill()
		remoteTunnelCmd = nil
		remoteTunnelURL = ""
	}

	cmd := exec.Command(bin, "tunnel", "--url", targetURL, "--no-tls-verify", "--no-autoupdate")
	hideCommandWindow(cmd)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		remoteTunnelMu.Unlock()
		return "", err
	}
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		remoteTunnelMu.Unlock()
		return "", err
	}
	remoteTunnelCmd = cmd
	remoteTunnelStarted = time.Now()
	remoteTunnelMu.Unlock()

	foundCh := make(chan string, 1)
	scanPipe := func(r *bufio.Scanner) {
		for r.Scan() {
			line := r.Text()
			if m := tryCloudflareRe.FindString(line); m != "" {
				select {
				case foundCh <- m:
				default:
				}
			}
		}
	}
	go scanPipe(bufio.NewScanner(stderr))
	if stdout != nil {
		go scanPipe(bufio.NewScanner(stdout))
	}
	go func(c *exec.Cmd) {
		_ = c.Wait()
		remoteTunnelMu.Lock()
		if remoteTunnelCmd == c {
			remoteTunnelCmd = nil
			remoteTunnelURL = ""
		}
		remoteTunnelMu.Unlock()
	}(cmd)

	waitCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	select {
	case u := <-foundCh:
		remoteTunnelMu.Lock()
		remoteTunnelURL = strings.TrimRight(u, "/")
		remoteTunnelMu.Unlock()
		return strings.TrimRight(u, "/"), nil
	case <-waitCtx.Done():
		stopRemoteMobileTunnel()
		return "", errors.New("Mobildaten-Tunnel konnte innerhalb von 18s keine HTTPS-URL erhalten")
	}
}

func (s *Server) handleRemoteTunnel(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		base := currentRemoteTunnelURL()
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{
			"ok":          true,
			"running":     base != "",
			"tunnel_url":  base,
			"remote_url":  func() string { if base != "" { return base + "/remote" }; return "" }(),
			"cloudflared": findCloudflaredBinary() != "",
		})
		return
	}
	if r.Method == http.MethodDelete {
		stopRemoteMobileTunnel()
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{"ok": true, "running": false})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.state.mu.RLock()
	port := s.state.Config.RemotePort
	if port <= 0 {
		port = 32146
	}
	s.state.mu.RUnlock()

	base, err := ensureRemoteMobileTunnel(r.Context(), port)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	remoteURL := base + "/remote"
	code, expires, _, err := s.state.StartRemotePairing()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	deepLink := fmt.Sprintf("localcode://pair?url=%s&code=%s", url.QueryEscape(remoteURL), url.QueryEscape(code))
	webLink := fmt.Sprintf("%s#code=%s", remoteURL, url.QueryEscape(code))
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{
		"ok":         true,
		"running":    true,
		"tunnel_url": base,
		"remote_url": remoteURL,
		"code":       code,
		"expires_at": expires,
		"deep_link":  deepLink,
		"web_link":   webLink,
	})
}
