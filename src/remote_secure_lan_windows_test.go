// SPDX-License-Identifier: Apache-2.0

//go:build windows

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProductionLANRemoteServesHTTPSWithGeneratedCertificate(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(base, "config"))

	state := newRemoteTestState(t)
	pair, fingerprint, err := ensureRemoteTLSCertificate("0.0.0.0")
	if err != nil {
		t.Fatal(err)
	}
	urls := []string{"https://192.168.1.20:32146/remote"}
	for _, remoteURL := range urls {
		if !strings.HasPrefix(remoteURL, "https://") {
			t.Fatalf("LAN URL is not HTTPS: %q", remoteURL)
		}
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
	remote := NewRemoteServer(state)
	registerRemoteDiscoveryRoute(remote, fingerprint, urls)
	server := httptest.NewUnstartedServer(remote)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	server.StartTLS()
	t.Cleanup(server.Close)

	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: "127.0.0.1",
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	endpoint := server.URL + "/remote/api/discovery"

	resp, err := client.Get(endpoint)
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
