// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProductionLANRemoteServesHTTPSWithGeneratedCertificate(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(base, "config"))

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}

	originalFirewallRunner := runRemoteFirewallPowerShell
	runRemoteFirewallPowerShell = func(string) error { return nil }
	t.Cleanup(func() { runRemoteFirewallPowerShell = originalFirewallRunner })

	state := newRemoteTestState(t)
	cfg := state.Config
	cfg.RemoteEnabled = true
	cfg.RemoteBindHost = "0.0.0.0"
	cfg.RemotePort = port

	urls, err := startProductionRemoteServer(state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) == 0 {
		t.Fatal("LAN HTTPS server returned no advertised URLs")
	}
	for _, remoteURL := range urls {
		if !strings.HasPrefix(remoteURL, "https://") {
			t.Fatalf("LAN URL is not HTTPS: %q", remoteURL)
		}
	}

	state.mu.Lock()
	listenAddr := state.RemoteListenAddr
	stateURLs := append([]string(nil), state.RemoteURLs...)
	state.mu.Unlock()
	_, actualPortText, err := net.SplitHostPort(listenAddr)
	if err != nil {
		t.Fatalf("invalid LAN listen address %q: %v", listenAddr, err)
	}
	actualPort, err := strconv.Atoi(actualPortText)
	if err != nil || actualPort <= 0 {
		t.Fatalf("invalid LAN listen port %q: %v", actualPortText, err)
	}
	if len(stateURLs) == 0 {
		t.Fatal("state did not retain advertised LAN URLs")
	}

	certPath, _ := remoteTLSCertificatePaths()
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("generated LAN certificate could not be added to test trust pool")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: "127.0.0.1",
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	endpoint := "https://127.0.0.1:" + strconv.Itoa(actualPort) + "/remote/api/discovery"

	var resp *http.Response
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = client.Get(endpoint)
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("LAN HTTPS discovery request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("LAN HTTPS discovery status=%d", resp.StatusCode)
	}
	var discovery struct {
		TLS         bool     `json:"tls"`
		Fingerprint string   `json:"tls_fingerprint"`
		RemoteURLs  []string `json:"remote_urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&discovery); err != nil {
		t.Fatal(err)
	}
	if !discovery.TLS || len(discovery.Fingerprint) != 64 || len(discovery.RemoteURLs) == 0 {
		t.Fatalf("unexpected LAN discovery payload: %#v", discovery)
	}
}
