// SPDX-License-Identifier: Apache-2.0

package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoteServerHasNoMissionRecoveryControlRoutes(t *testing.T) {
	remote := NewRemoteServer(&AppState{})
	for _, path := range []string{"/remote/api/mission-recovery", "/remote/api/mission-recovery/continue"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		remote.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("remote recovery path %q status=%d want=%d", path, rr.Code, http.StatusNotFound)
		}
	}
}
