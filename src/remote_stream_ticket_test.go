// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoteStreamTicketIsSingleUse(t *testing.T) {
	resetRemoteStreamTicketsForTest()
	t.Cleanup(resetRemoteStreamTicketsForTest)
	ticket, expires, err := issueRemoteStreamTicket()
	if err != nil {
		t.Fatal(err)
	}
	if ticket == "" || !expires.After(time.Now()) {
		t.Fatal("invalid stream ticket")
	}
	if !consumeRemoteStreamTicket(ticket) {
		t.Fatal("fresh stream ticket rejected")
	}
	if consumeRemoteStreamTicket(ticket) {
		t.Fatal("stream ticket reused")
	}
}

func TestRemoteStreamTicketExpires(t *testing.T) {
	resetRemoteStreamTicketsForTest()
	t.Cleanup(resetRemoteStreamTicketsForTest)
	ticket, _, err := issueRemoteStreamTicket()
	if err != nil {
		t.Fatal(err)
	}
	hash := remoteStreamTicketHash(ticket)
	remoteStreamTicketStore.Lock()
	remoteStreamTicketStore.tickets[hash] = time.Now().Add(-time.Second)
	remoteStreamTicketStore.Unlock()
	if consumeRemoteStreamTicket(ticket) {
		t.Fatal("expired stream ticket accepted")
	}
}

func TestRemoteEventTicketRequiresAuthentication(t *testing.T) {
	resetRemoteStreamTicketsForTest()
	t.Cleanup(resetRemoteStreamTicketsForTest)
	state := newRemoteTestState(t)
	remote := NewRemoteServer(state)
	if rr := serveHTTP(remote, http.MethodPost, "/remote/api/event-ticket", "", ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ticket status = %d", rr.Code)
	}
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	rr := serveHTTP(remote, http.MethodPost, "/remote/api/event-ticket", "", token)
	if rr.Code != http.StatusOK {
		t.Fatalf("ticket status = %d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Ticket == "" || !consumeRemoteStreamTicket(body.Ticket) {
		t.Fatal("issued ticket not registered")
	}
}

func TestRemoteSSERejectsDeviceTokenInQuery(t *testing.T) {
	resetRemoteStreamTicketsForTest()
	t.Cleanup(resetRemoteStreamTicketsForTest)
	state := newRemoteTestState(t)
	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := state.PairRemoteDevice(code, "phone")
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/remote/api/events?token="+token, nil)
	req.Host = "127.0.0.1"
	rr := httptest.NewRecorder()
	NewRemoteServer(state).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("device query token SSE status = %d", rr.Code)
	}
}
