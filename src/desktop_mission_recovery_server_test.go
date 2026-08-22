// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopMissionRecoveryRoutesUseDesktopSecurityBoundary(t *testing.T) {
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	server := NewServer(&AppState{})

	t.Run("inspection-is-registered", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/mission-recovery", nil)
		req.Host = "127.0.0.1:32145"
		server.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("non-loopback-host-is-rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/mission-recovery", nil)
		req.Host = "example.com"
		server.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})

	t.Run("cross-origin-continuation-is-rejected", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/mission-recovery/continue", strings.NewReader(`{}`))
		req.Host = "127.0.0.1:32145"
		req.Header.Set("Origin", "https://example.com")
		server.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
		}
	})
}
