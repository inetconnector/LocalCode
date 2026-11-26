//go:build windows

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestMobileSafeProductionRemoteServerSetsTLSFingerprintForQRPairing(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	origFirewall := runRemoteFirewallPowerShell
	runRemoteFirewallPowerShell = func(string) error { return nil }
	t.Cleanup(func() { runRemoteFirewallPowerShell = origFirewall })

	probe, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	_, portText, err := net.SplitHostPort(probe.Addr().String())
	if err != nil {
		_ = probe.Close()
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		_ = probe.Close()
		t.Fatal(err)
	}
	_ = probe.Close()

	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteEnabled = true
	state.Config.RemoteBindHost = "0.0.0.0"
	state.Config.RemotePort = port
	cfg := state.Config
	state.mu.Unlock()

	urls, err := startMobileSafeProductionRemoteServer(state, cfg)
	if err != nil {
		t.Fatalf("startMobileSafeProductionRemoteServer failed: %v", err)
	}
	if len(urls) == 0 {
		t.Fatal("expected at least one HTTPS remote URL")
	}

	state.mu.RLock()
	fp := state.RemoteTLSFingerprint
	state.mu.RUnlock()
	if !regexp.MustCompile(`^[0-9A-F]{64}$`).MatchString(fp) {
		t.Fatalf("expected 64-char hex SHA-256 TLS fingerprint on state, got %q", fp)
	}

	srv := NewServer(state)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/remote/pairing", nil)
	srv.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/remote/pairing returned %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK          bool   `json:"ok"`
		Fingerprint string `json:"fingerprint"`
		DeepLink    string `json:"deep_link"`
		WebLink     string `json:"web_link"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode pairing response: %v", err)
	}
	if !body.OK || body.Fingerprint != fp {
		t.Fatalf("pairing fingerprint=%q want %q", body.Fingerprint, fp)
	}
	if !strings.Contains(body.DeepLink, "fp="+fp) {
		t.Fatalf("QR deep_link missing real TLS fingerprint %q: %s", fp, body.DeepLink)
	}
	if !strings.Contains(body.WebLink, "fp="+fp) {
		t.Fatalf("QR web_link missing real TLS fingerprint %q: %s", fp, body.WebLink)
	}
}
