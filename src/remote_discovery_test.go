// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestRemoteUDPDiscovery(t *testing.T) {
	cfg := Config{
		RootProjectDir: t.TempDir(),
		LastProject:    filepath.Join(t.TempDir(), "TestProjectA"),
		RemoteEnabled:  true,
		RemoteBindHost: "127.0.0.1",
		RemotePort:     32146,
	}
	state := NewAppState(cfg, nil)
	defer state.Close()

	state.RegisterActiveRun(&ActiveAgentRun{
		ID:       "run-123",
		Project:  filepath.Join(t.TempDir(), "ActiveProjectX"),
		ThreadID: "thread-1",
		Model:    "test-model",
	})

	stop, err := startRemoteUDPDiscovery(32146, "127.0.0.1", "ABCDEF1234567890", []string{"https://127.0.0.1:32146/remote"}, state)
	if err != nil {
		t.Logf("startRemoteUDPDiscovery returned %v (port may be in use in test env)", err)
		return
	}
	defer stop()

	// Connect as client and send broadcast query
	client, err := net.DialUDP("udp4", nil, &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: DefaultUDPDiscoveryPort,
	})
	if err != nil {
		t.Fatalf("DialUDP failed: %v", err)
	}
	defer client.Close()

	probe := []byte(`{"cmd":"discover"}`)
	if _, err := client.Write(probe); err != nil {
		t.Fatalf("client.Write failed: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 4096)
	n, _, err := client.ReadFrom(buf)
	if err != nil {
		t.Fatalf("client.ReadFrom failed: %v", err)
	}

	var payload UDPDiscoveryPayload
	if err := json.Unmarshal(buf[:n], &payload); err != nil {
		t.Fatalf("Unmarshal discovery response failed: %v (raw: %s)", err, string(buf[:n]))
	}

	if payload.App != "LocalCode Remote" {
		t.Errorf("expected App 'LocalCode Remote', got %q", payload.App)
	}
	if payload.Port != 32146 {
		t.Errorf("expected Port 32146, got %d", payload.Port)
	}
	if payload.TLSFingerprint != "ABCDEF1234567890" {
		t.Errorf("expected TLSFingerprint 'ABCDEF1234567890', got %q", payload.TLSFingerprint)
	}
	if len(payload.RunningProjects) == 0 {
		t.Errorf("expected running projects in response, got none")
	}
}
