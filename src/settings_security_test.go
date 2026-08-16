// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPublicSettingsConfigRedactsRemoteDeviceCredentials(t *testing.T) {
	cfg := defaultConfig()
	cfg.RemoteDevices = []RemoteDevice{{ID: "device-1", Name: "Phone", TokenHash: "secret-hash", PairedAt: time.Now()}}
	data, err := json.Marshal(publicSettingsConfig(cfg))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "secret-hash") || strings.Contains(text, "token_hash") || strings.Contains(text, "device-1") {
		t.Fatalf("public settings leaked remote device state: %s", text)
	}
}

func TestSettingsAPICannotOverwriteRemoteDevices(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteDevices = []RemoteDevice{{ID: "device-1", Name: "Phone", TokenHash: "secret-hash", PairedAt: time.Now()}}
	state.mu.Unlock()

	server := NewServer(state)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/settings", strings.NewReader(`{"ui_accent":"#123456","remote_devices":[]}`))
	req.Host = "127.0.0.1"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("settings status=%d body=%s", rr.Code, rr.Body.String())
	}
	state.mu.RLock()
	devices := append([]RemoteDevice(nil), state.Config.RemoteDevices...)
	accent := state.Config.UIAccent
	state.mu.RUnlock()
	if accent != "#123456" {
		t.Fatalf("ordinary setting was not applied: %q", accent)
	}
	if len(devices) != 1 || devices[0].ID != "device-1" || devices[0].TokenHash != "secret-hash" {
		t.Fatalf("generic settings patch modified security-owned remote devices: %#v", devices)
	}
}

func TestSettingsGETDoesNotExposeRemoteDeviceCredentials(t *testing.T) {
	state := newRemoteTestState(t)
	state.mu.Lock()
	state.Config.RemoteDevices = []RemoteDevice{{ID: "device-1", Name: "Phone", TokenHash: "secret-hash", PairedAt: time.Now()}}
	state.mu.Unlock()
	server := NewServer(state)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/settings", nil)
	req.Host = "127.0.0.1"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("settings GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "secret-hash") || strings.Contains(body, "token_hash") || strings.Contains(body, "device-1") {
		t.Fatalf("settings GET leaked remote device credentials: %s", body)
	}
}
