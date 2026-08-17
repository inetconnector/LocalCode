// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/x509"
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteSecureCoverageEdges(t *testing.T) {
	if got := desiredRemoteMDNSIPs("192.168.50.9"); len(got) != 1 || !got[0].Equal(net.ParseIP("192.168.50.9")) {
		t.Fatalf("direct mDNS IPs=%v", got)
	}
	if got := desiredRemoteMDNSIPs("127.0.0.1"); len(got) != 0 {
		t.Fatalf("loopback must not be advertised over mDNS: %v", got)
	}
	_ = desiredRemoteMDNSIPs("0.0.0.0")
	_ = desiredRemoteTLSIPs("0.0.0.0")

	cert := &x509.Certificate{IPAddresses: []net.IP{net.ParseIP("192.168.50.9")}}
	if !certificateCoversIPs(cert, nil) {
		t.Fatal("an empty requested-IP set should be covered")
	}
	if certificateCoversIPs(cert, []net.IP{net.ParseIP("192.168.50.10")}) {
		t.Fatal("certificate unexpectedly covers an absent IP")
	}

	urls := secureRemoteURLsForListener(fixedAddr("malformed"), "192.168.50.9")
	if len(urls) != 1 || urls[0] != "https://192.168.50.9:32146/remote" {
		t.Fatalf("default-port secure URLs=%v", urls)
	}
	_ = secureRemoteURLsForListener(fixedAddr("0.0.0.0:32146"), "0.0.0.0")

	parentFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(filepath.Join(parentFile, "child"), []byte("secret")); err == nil {
		t.Fatal("writePrivateFile should fail when its parent is a regular file")
	}

	if got := dnsAppendRecord(nil, strings.Repeat("z", 64)+".local.", 1, 1, 1, []byte{1}); got != nil {
		t.Fatal("invalid DNS record name should be rejected")
	}
	if err := startRemoteMDNS(32146, "127.0.0.1", "fp", nil); err != nil {
		t.Fatalf("loopback mDNS should be a no-op: %v", err)
	}

	cfg := defaultConfig()
	cfg.RemoteEnabled = true
	cfg.RemoteBindHost = "256.256.256.256"
	cfg.RemotePort = 32146
	state := newRemoteTestState(t)
	if urls, err := startProductionRemoteServer(state, cfg); err == nil || urls != nil {
		t.Fatalf("invalid LAN bind should fail before firewall/TLS, urls=%v err=%v", urls, err)
	}
}

func TestDNSCompressionAndMalformedQuestions(t *testing.T) {
	// Name at offset 0, followed by a compression pointer back to it.
	base := dnsAppendName(nil, "localcode.local.")
	packet := append(append([]byte{}, base...), 0xc0, 0x00)
	name, next, ok := dnsReadName(packet, len(base))
	if !ok || name != "localcode.local." || next != len(base)+2 {
		t.Fatalf("compressed DNS name=%q next=%d ok=%v", name, next, ok)
	}
	if _, _, ok := dnsReadName([]byte{0xc0, 0xff}, 0); ok {
		t.Fatal("out-of-range DNS compression pointer accepted")
	}
	if _, _, ok := dnsReadName([]byte{64, 'x'}, 0); ok {
		t.Fatal("oversized/truncated DNS label accepted")
	}

	question := make([]byte, 12)
	binary.BigEndian.PutUint16(question[4:6], 1)
	question = append(question, dnsAppendName(nil, remoteMDNSInstance)...)
	question = append(question, 0, 33, 0, 1)
	if !remoteMDNSQueryMatches(question) {
		t.Fatal("instance query was not recognized")
	}
	truncated := make([]byte, 12)
	binary.BigEndian.PutUint16(truncated[4:6], 1)
	truncated = append(truncated, 1, 'x', 0)
	if remoteMDNSQueryMatches(truncated) {
		t.Fatal("truncated DNS question was accepted")
	}
}
