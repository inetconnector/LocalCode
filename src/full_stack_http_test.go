// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFullStackLoopbackDesktopAndRemoteHTTP(t *testing.T) {
	state := newRemoteTestState(t)

	desktop := httptest.NewServer(NewServer(state))
	defer desktop.Close()

	pingResp, err := desktop.Client().Get(desktop.URL + "/api/ping")
	if err != nil {
		t.Fatal(err)
	}
	pingBody, _ := io.ReadAll(pingResp.Body)
	_ = pingResp.Body.Close()
	if pingResp.StatusCode != http.StatusOK || !strings.Contains(string(pingBody), `"app":"LocalCode"`) {
		t.Fatalf("desktop ping status=%d body=%s", pingResp.StatusCode, pingBody)
	}

	settingsReq, err := http.NewRequest(http.MethodPost, desktop.URL+"/api/settings", strings.NewReader(`{"ui_accent":"#123456"}`))
	if err != nil {
		t.Fatal(err)
	}
	settingsReq.Header.Set("Content-Type", "application/json")
	settingsReq.Header.Set("Origin", desktop.URL)
	settingsResp, err := desktop.Client().Do(settingsReq)
	if err != nil {
		t.Fatal(err)
	}
	settingsBody, _ := io.ReadAll(settingsResp.Body)
	_ = settingsResp.Body.Close()
	if settingsResp.StatusCode != http.StatusOK {
		t.Fatalf("desktop settings status=%d body=%s", settingsResp.StatusCode, settingsBody)
	}
	state.mu.RLock()
	accent := state.Config.UIAccent
	state.mu.RUnlock()
	if accent != "#123456" {
		t.Fatalf("desktop settings did not reach state over real HTTP: %q", accent)
	}

	pairingReq, err := http.NewRequest(http.MethodPost, desktop.URL+"/api/remote/pairing", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	pairingReq.Header.Set("Content-Type", "application/json")
	pairingReq.Header.Set("Origin", desktop.URL)
	pairingResp, err := desktop.Client().Do(pairingReq)
	if err != nil {
		t.Fatal(err)
	}
	defer pairingResp.Body.Close()
	if pairingResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(pairingResp.Body)
		t.Fatalf("desktop pairing status=%d body=%s", pairingResp.StatusCode, body)
	}
	var pairing struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(pairingResp.Body).Decode(&pairing); err != nil {
		t.Fatal(err)
	}
	if len(pairing.Code) != 8 {
		t.Fatalf("unexpected pairing code %q", pairing.Code)
	}

	remote := httptest.NewServer(NewRemoteServer(state))
	defer remote.Close()

	pairReq, err := http.NewRequest(http.MethodPost, remote.URL+"/remote/api/pair", strings.NewReader(`{"code":"`+pairing.Code+`","device_name":"HTTP integration phone"}`))
	if err != nil {
		t.Fatal(err)
	}
	pairReq.Header.Set("Content-Type", "application/json")
	pairReq.Header.Set("Origin", remote.URL)
	pairResp, err := remote.Client().Do(pairReq)
	if err != nil {
		t.Fatal(err)
	}
	pairBody, _ := io.ReadAll(pairResp.Body)
	_ = pairResp.Body.Close()
	if pairResp.StatusCode != http.StatusOK {
		t.Fatalf("remote pair status=%d body=%s", pairResp.StatusCode, pairBody)
	}
	if strings.Contains(string(pairBody), "token_hash") {
		t.Fatalf("remote pairing leaked token hash over network: %s", pairBody)
	}
	var paired struct {
		Token  string           `json:"token"`
		Device RemoteDeviceView `json:"device"`
	}
	if err := json.Unmarshal(pairBody, &paired); err != nil {
		t.Fatal(err)
	}
	if paired.Token == "" || paired.Device.Name != "HTTP integration phone" || paired.Device.ExpiresAt.IsZero() {
		t.Fatalf("unexpected network pairing result: %#v", paired)
	}

	statusReq, err := http.NewRequest(http.MethodGet, remote.URL+"/remote/api/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	statusReq.Header.Set("X-LocalCode-Remote-Token", paired.Token)
	statusResp, err := remote.Client().Do(statusReq)
	if err != nil {
		t.Fatal(err)
	}
	statusBody, _ := io.ReadAll(statusResp.Body)
	_ = statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK || !strings.Contains(string(statusBody), `"app":"LocalCode Remote"`) {
		t.Fatalf("remote authenticated status=%d body=%s", statusResp.StatusCode, statusBody)
	}

	ticketReq, err := http.NewRequest(http.MethodPost, remote.URL+"/remote/api/event-ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	ticketReq.Header.Set("X-LocalCode-Remote-Token", paired.Token)
	ticketReq.Header.Set("Origin", remote.URL)
	ticketResp, err := remote.Client().Do(ticketReq)
	if err != nil {
		t.Fatal(err)
	}
	defer ticketResp.Body.Close()
	if ticketResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ticketResp.Body)
		t.Fatalf("event-ticket status=%d body=%s", ticketResp.StatusCode, body)
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(ticketResp.Body).Decode(&ticket); err != nil {
		t.Fatal(err)
	}
	if ticket.Ticket == "" {
		t.Fatal("empty event stream ticket")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	eventsReq, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.URL+"/remote/api/events?ticket="+ticket.Ticket, nil)
	if err != nil {
		t.Fatal(err)
	}
	eventsResp, err := remote.Client().Do(eventsReq)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(eventsResp.Body)
	line, err := reader.ReadString('\n')
	_ = eventsResp.Body.Close()
	cancel()
	if err != nil {
		t.Fatalf("reading SSE greeting: %v", err)
	}
	if eventsResp.StatusCode != http.StatusOK || strings.TrimSpace(line) != ": connected" {
		t.Fatalf("SSE status=%d first-line=%q", eventsResp.StatusCode, line)
	}

	reuseResp, err := remote.Client().Get(remote.URL + "/remote/api/events?ticket=" + ticket.Ticket)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, reuseResp.Body)
	_ = reuseResp.Body.Close()
	if reuseResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("single-use stream ticket was reusable over network: status=%d", reuseResp.StatusCode)
	}
}
