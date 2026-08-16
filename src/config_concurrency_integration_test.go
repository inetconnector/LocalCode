// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSettingsAndApprovalRuleDoNotLoseEachOther(t *testing.T) {
	state := newConfigTransactionTestState(t)
	server := NewServer(state)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 1)

	go func() {
		defer wg.Done()
		<-start
		_, err := state.addApprovalRule("", AgentAction{Action: "git", Args: []string{"status"}}, "global")
		if err != nil {
			errCh <- err
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/settings", strings.NewReader(`{"ui_accent":"#123456"}`))
		req.Host = "127.0.0.1"
		rr := httptest.NewRecorder()
		server.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			errCh <- &statusCodeError{code: rr.Code, body: rr.Body.String()}
		}
	}()
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatal(err)
	}

	state.mu.RLock()
	cfg := state.Config
	state.mu.RUnlock()
	if cfg.UIAccent != "#123456" {
		t.Fatalf("settings update lost: %q", cfg.UIAccent)
	}
	if len(cfg.ApprovalRules) == 0 {
		t.Fatal("approval rule update lost")
	}
}

type statusCodeError struct {
	code int
	body string
}

func (e *statusCodeError) Error() string { return http.StatusText(e.code) + ": " + e.body }
