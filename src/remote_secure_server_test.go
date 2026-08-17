// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fixedAddr string

func (a fixedAddr) Network() string { return "tcp" }
func (a fixedAddr) String() string  { return string(a) }

func TestRemoteRequiresTLS(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"", false}, {"localhost", false}, {"127.0.0.1", false}, {"[::1]", false},
		{"0.0.0.0", true}, {"::", true}, {"192.168.1.10", true}, {"localcode.lan", true},
	}
	for _, tc := range cases {
		if got := remoteRequiresTLS(tc.host); got != tc.want {
			t.Fatalf("remoteRequiresTLS(%q)=%v want %v", tc.host, got, tc.want)
		}
	}
}

func TestRemoteTLSCertificatePersistsAndCoversRequestedIP(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(base, "config"))
	pair, fingerprint, err := ensureRemoteTLSCertificate("192.168.23.44")
	if err != nil {
		t.Fatal(err)
	}
	if len(pair.Certificate) == 0 || len(fingerprint) != 64 {
		t.Fatalf("unexpected TLS material: certs=%d fingerprint=%q", len(pair.Certificate), fingerprint)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if !certificateCoversIPs(leaf, []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("192.168.23.44")}) {
		t.Fatalf("certificate SANs do not cover requested IPs: %#v", leaf.IPAddresses)
	}
	if certificateFingerprint(leaf) != fingerprint || certificateFingerprint(nil) != "" {
		t.Fatal("certificate fingerprint helper mismatch")
	}
	certPath, keyPath := remoteTLSCertificatePaths()
	if _, err := os.Stat(certPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatal(err)
	}
	pair2, fingerprint2, err := ensureRemoteTLSCertificate("192.168.23.44")
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint2 != fingerprint || len(pair2.Certificate) == 0 {
		t.Fatalf("persisted certificate was not reused: %q vs %q", fingerprint2, fingerprint)
	}
	if _, _, ok := loadRemoteTLSCertificate(certPath, keyPath, []net.IP{net.ParseIP("10.9.8.7")}); ok {
		t.Fatal("certificate unexpectedly accepted for an uncovered IP")
	}
}

func TestSecureRemoteURLsAndPairingURI(t *testing.T) {
	urls := secureRemoteURLsForListener(fixedAddr("0.0.0.0:43123"), "192.168.7.9")
	if len(urls) != 1 || urls[0] != "https://192.168.7.9:43123/remote" {
		t.Fatalf("unexpected secure URLs: %#v", urls)
	}
	uri := remotePairingURI(urls[0], strings.Repeat("A", 64))
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "localcode" || parsed.Host != "pair" || parsed.Query().Get("url") != urls[0] || len(parsed.Query().Get("fp")) != 64 {
		t.Fatalf("unexpected pairing URI: %s", uri)
	}
}

func makeMDNSQuery(name string) []byte {
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[4:6], 1)
	packet = dnsAppendName(packet, name)
	packet = append(packet, 0, 12, 0, 1)
	return packet
}

func TestRemoteMDNSPacketAndQueryParsing(t *testing.T) {
	fingerprint := strings.Repeat("B", 64)
	urls := []string{"https://192.168.4.8:32146/remote"}
	packet := buildRemoteMDNSResponse(32146, []net.IP{net.ParseIP("192.168.4.8"), net.ParseIP("10.0.0.3")}, fingerprint, urls)
	if len(packet) < 100 {
		t.Fatalf("mDNS response unexpectedly small: %d", len(packet))
	}
	if flags := binary.BigEndian.Uint16(packet[2:4]); flags != 0x8400 {
		t.Fatalf("unexpected mDNS flags: 0x%x", flags)
	}
	if answers := binary.BigEndian.Uint16(packet[6:8]); answers != 5 {
		t.Fatalf("unexpected mDNS answer count: %d", answers)
	}
	text := string(packet)
	for _, marker := range []string{"_localcode", "LocalCode", "tls=1", "fp=" + fingerprint, "localcode://pair"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("mDNS packet missing %q", marker)
		}
	}
	if !remoteMDNSQueryMatches(makeMDNSQuery(remoteMDNSServiceType)) {
		t.Fatal("service query was not recognized")
	}
	if !remoteMDNSQueryMatches(makeMDNSQuery("_services._dns-sd._udp.local.")) {
		t.Fatal("DNS-SD enumeration query was not recognized")
	}
	if remoteMDNSQueryMatches(makeMDNSQuery("_printer._tcp.local.")) || remoteMDNSQueryMatches([]byte{1, 2, 3}) {
		t.Fatal("unrelated or malformed mDNS query was accepted")
	}
	if buildRemoteMDNSResponse(0, nil, fingerprint, urls) != nil || buildRemoteMDNSResponse(70000, nil, fingerprint, urls) != nil {
		t.Fatal("invalid port produced an mDNS response")
	}
}

func TestDNSHelpers(t *testing.T) {
	encoded := dnsAppendName(nil, "alpha.beta.local.")
	name, next, ok := dnsReadName(encoded, 0)
	if !ok || name != "alpha.beta.local." || next != len(encoded) {
		t.Fatalf("dnsReadName=%q next=%d ok=%v encoded=%d", name, next, ok, len(encoded))
	}
	if dnsAppendName(nil, strings.Repeat("x", 64)+".local.") != nil {
		t.Fatal("oversized DNS label accepted")
	}
	if _, _, ok := dnsReadName([]byte{0xc0}, 0); ok {
		t.Fatal("truncated DNS compression pointer accepted")
	}
	txt := dnsTXT("a=1", strings.Repeat("z", 300))
	if len(txt) != 1+3+1+255 {
		t.Fatalf("unexpected TXT size: %d", len(txt))
	}
	record := dnsAppendRecord(nil, "x.local.", 1, 1, 5, []byte{127, 0, 0, 1})
	if len(record) == 0 {
		t.Fatal("DNS record helper returned empty output")
	}
}

func TestRemoteDiscoveryRoute(t *testing.T) {
	state := newRemoteTestState(t)
	remote := NewRemoteServer(state)
	fingerprint := strings.Repeat("C", 64)
	registerRemoteDiscoveryRoute(remote, fingerprint, []string{"https://192.168.1.2:32146/remote"})
	req := httptest.NewRequest(http.MethodGet, "https://192.168.1.2/remote/api/discovery", nil)
	rr := httptest.NewRecorder()
	remote.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("discovery status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["tls"] != true || body["tls_fingerprint"] != fingerprint || body["service"] != remoteMDNSServiceType {
		t.Fatalf("unexpected discovery body: %#v", body)
	}
	bad := httptest.NewRequest(http.MethodPost, "https://192.168.1.2/remote/api/discovery", nil)
	badRR := httptest.NewRecorder()
	remote.ServeHTTP(badRR, bad)
	if badRR.Code != http.StatusMethodNotAllowed {
		t.Fatalf("discovery POST status=%d", badRR.Code)
	}
}

func TestStartProductionRemoteDisabledAndLoopback(t *testing.T) {
	state := newRemoteTestState(t)
	cfg := state.Config
	cfg.RemoteEnabled = false
	urls, err := startProductionRemoteServer(state, cfg)
	if err != nil || urls != nil {
		t.Fatalf("disabled secure remote: urls=%v err=%v", urls, err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	cfg.RemoteEnabled = true
	cfg.RemoteBindHost = "127.0.0.1"
	cfg.RemotePort = port
	urls, err = startProductionRemoteServer(state, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 || !strings.HasPrefix(urls[0], "http://127.0.0.1:") {
		t.Fatalf("loopback remote unexpectedly changed transport: %#v", urls)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(urls[0] + "/api/ping")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("loopback remote ping=%d", resp.StatusCode)
	}
}

func TestTLSConfigCanServeGeneratedCertificate(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALCODE_CONFIG_HOME", filepath.Join(base, "config"))
	pair, _, err := ensureRemoteTLSCertificate("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair}}
	if cfg.MinVersion != tls.VersionTLS12 || len(cfg.Certificates) != 1 {
		t.Fatal("unexpected TLS configuration")
	}
	if pair.Leaf == nil || pair.Leaf.NotAfter.Before(time.Now().Add(300*24*time.Hour)) {
		t.Fatal("generated certificate lifetime unexpectedly short")
	}
}
