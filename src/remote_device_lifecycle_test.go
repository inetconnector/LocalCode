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

func TestRemoteDeviceViewNeverExposesTokenHash(t *testing.T) {
	device := RemoteDevice{ID: "device-1", Name: "Phone", TokenHash: "secret-hash", PairedAt: time.Now(), LastSeenAt: time.Now()}
	data, err := json.Marshal(remoteDeviceView(device))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-hash") || strings.Contains(string(data), "token_hash") {
		t.Fatalf("public remote-device view leaked token hash: %s", data)
	}
	if !strings.Contains(string(data), "expires_at") {
		t.Fatalf("public remote-device view missing expiry: %s", data)
	}
}

func TestRemoteTokenExpiresAfterDeviceTTL(t *testing.T) {
	state := newRemoteTestState(t)
	token := "expired-device-token"
	state.mu.Lock()
	state.Config.RemoteDevices = []RemoteDevice{{
		ID: "expired-device", Name: "Old phone", TokenHash: remoteTokenHash(token),
		PairedAt: time.Now().Add(-remoteDeviceTokenTTL - time.Hour), LastSeenAt: time.Now().Add(-time.Hour),
	}}
	state.mu.Unlock()
	if state.RemoteTokenValid(token) {
		t.Fatal("expired remote device token was accepted")
	}
}

func TestRemoteDeviceCanBeRevokedFromDesktopAPI(t *testing.T) {
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, device, err := state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if !state.RemoteTokenValid(token) {
		t.Fatal("fresh paired token rejected")
	}

	server := NewServer(state)
	rr := serveDesktopHTTP(server, http.MethodPost, "/api/remote/revoke", `{"id":"`+device.ID+`"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", rr.Code, rr.Body.String())
	}
	if state.RemoteTokenValid(token) {
		t.Fatal("revoked remote token still accepted")
	}
}

func TestRemoteDevicesDesktopAPIReturnsSanitizedViews(t *testing.T) {
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(state)
	rr := serveDesktopHTTP(server, http.MethodGet, "/api/remote/devices", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("devices status=%d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "token_hash") {
		t.Fatalf("desktop device list leaked token hash: %s", body)
	}
	if !strings.Contains(body, "expires_at") {
		t.Fatalf("desktop device list missing expiry: %s", body)
	}
}

func TestRemoteDeviceCanBeUnpairedFromRemoteAPI(t *testing.T) {
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	if !state.RemoteTokenValid(token) {
		t.Fatal("fresh paired token rejected")
	}

	remoteServer := NewRemoteServer(state)
	req := httptest.NewRequest(http.MethodPost, "https://127.0.0.1/remote/api/unpair", strings.NewReader(`{}`))
	req.Host = "127.0.0.1"
	req.Header.Set("X-LocalCode-Remote-Token", token)
	rr := httptest.NewRecorder()
	remoteServer.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unpair status=%d body=%s", rr.Code, rr.Body.String())
	}
	if state.RemoteTokenValid(token) {
		t.Fatal("unpaired remote token still valid after unpair")
	}

	// Calling unpair again with revoked token should fail auth with 401
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "https://127.0.0.1/remote/api/unpair", strings.NewReader(`{}`))
	req2.Host = "127.0.0.1"
	req2.Header.Set("X-LocalCode-Remote-Token", token)
	remoteServer.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("unpair with invalid token status=%d, expected 401", rr2.Code)
	}
}

func TestRevokeRemoteTokenErrors(t *testing.T) {
	state := newRemoteTestState(t)
	if err := state.RevokeRemoteToken(""); err == nil {
		t.Fatal("expected error on empty token")
	}
	if err := state.RevokeRemoteToken("non-existent-token"); err == nil {
		t.Fatal("expected error on unknown token")
	}
	if err := state.RevokeRemoteDevice(""); err == nil {
		t.Fatal("expected error on empty device id")
	}
	if err := state.RevokeRemoteDevice("non-existent-id"); err == nil {
		t.Fatal("expected error on unknown device id")
	}
}

func serveDesktopHTTP(server *Server, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "http://127.0.0.1"+path, strings.NewReader(body))
	req.Host = "127.0.0.1"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	return rr
}
