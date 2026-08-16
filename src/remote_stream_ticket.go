// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

const remoteStreamTicketTTL = 45 * time.Second

var remoteStreamTicketStore = struct {
	sync.Mutex
	tickets map[string]time.Time
}{tickets: map[string]time.Time{}}

func remoteStreamTicketHash(ticket string) string {
	return remoteTokenHash("stream-ticket:" + strings.TrimSpace(ticket))
}

func issueRemoteStreamTicket() (string, time.Time, error) {
	ticket, err := randomRemoteToken()
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(remoteStreamTicketTTL)
	hash := remoteStreamTicketHash(ticket)
	remoteStreamTicketStore.Lock()
	now := time.Now()
	for existing, expiry := range remoteStreamTicketStore.tickets {
		if !now.Before(expiry) {
			delete(remoteStreamTicketStore.tickets, existing)
		}
	}
	remoteStreamTicketStore.tickets[hash] = expires
	remoteStreamTicketStore.Unlock()
	return ticket, expires, nil
}

func consumeRemoteStreamTicket(ticket string) bool {
	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return false
	}
	hash := remoteStreamTicketHash(ticket)
	now := time.Now()
	remoteStreamTicketStore.Lock()
	defer remoteStreamTicketStore.Unlock()
	for existing, expiry := range remoteStreamTicketStore.tickets {
		if !now.Before(expiry) {
			delete(remoteStreamTicketStore.tickets, existing)
		}
	}
	expires, ok := remoteStreamTicketStore.tickets[hash]
	if !ok || !now.Before(expires) {
		delete(remoteStreamTicketStore.tickets, hash)
		return false
	}
	delete(remoteStreamTicketStore.tickets, hash)
	return true
}

func (s *RemoteServer) handleEventTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ticket, expires, err := issueRemoteStreamTicket()
	if err != nil {
		http.Error(w, "could not issue stream ticket", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = writeJSON(w, map[string]any{"ticket": ticket, "expires_at": expires})
}

func (s *RemoteServer) withStreamTicket(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !consumeRemoteStreamTicket(r.URL.Query().Get("ticket")) {
			http.Error(w, "valid stream ticket required", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func resetRemoteStreamTicketsForTest() {
	remoteStreamTicketStore.Lock()
	remoteStreamTicketStore.tickets = map[string]time.Time{}
	remoteStreamTicketStore.Unlock()
}
