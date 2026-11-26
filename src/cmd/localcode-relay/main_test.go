// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func makeTestProof(token, sid, role, method string, ts int64) string {
	tsStr := strconv.FormatInt(ts, 10)
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(domainSeparation + "|req|" + sid + "|" + role + "|" + strings.ToUpper(method) + "|" + tsStr))
	return tsStr + "." + hex.EncodeToString(mac.Sum(nil))
}

func makeValidEnvelope(sid, role string, seq uint64) opaqueEnvelope {
	dir := "workstation->mobile"
	if role == "mobile" {
		dir = "mobile->workstation"
	}
	return opaqueEnvelope{
		Version:    protocolVersion,
		SessionID:  sid,
		SenderRole: role,
		Direction:  dir,
		Seq:        seq,
		Nonce:      base64.RawURLEncoding.EncodeToString([]byte("123456789012")),
		Ciphertext: base64.RawURLEncoding.EncodeToString([]byte("0123456789abcdef_opaque_payload")),
		Timestamp:  time.Now().Unix(),
	}
}

func TestRateLimiterAllowAndPurge(t *testing.T) {
	def := newRateLimiter(0, 0)
	if def.rate != defaultRateLimit || def.burst != float64(defaultRateBurst) {
		t.Fatalf("unexpected defaults: rate=%v burst=%v", def.rate, def.burst)
	}
	lim := newRateLimiter(1.0, 2)
	base := time.Now()
	if !lim.Allow("k1", base) || !lim.Allow("k1", base) {
		t.Fatal("initial burst should succeed")
	}
	if lim.Allow("k1", base) {
		t.Fatal("third immediate request should be rate limited")
	}
	// After 2 seconds, tokens refill up to burst cap
	if !lim.Allow("k1", base.Add(5*time.Second)) {
		t.Fatal("request after refill should succeed")
	}
	lim.PurgeIdle(base.Add(10 * time.Second))
	lim.mu.Lock()
	remaining := len(lim.buckets)
	lim.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("expected all idle buckets purged, got %d", remaining)
	}
}

func TestVerifyProofEdgeCases(t *testing.T) {
	now := time.Now()
	tok := strings.Repeat("a", 40)
	sid := "session_identifier_1234567890"
	valid := makeTestProof(tok, sid, "workstation", http.MethodPost, now.Unix())
	if !verifyProof(tok, sid, "workstation", http.MethodPost, valid, now) {
		t.Fatal("expected valid proof to verify")
	}
	for _, bad := range []string{
		"",
		"not-two-parts",
		"notanumber.deadbeef",
		makeTestProof(tok, sid, "workstation", http.MethodPost, now.Unix()-proofMaxClockSkewSec-10),
		makeTestProof(tok, sid, "workstation", http.MethodPost, now.Unix()+proofMaxClockSkewSec+10),
		strconv.FormatInt(now.Unix(), 10) + ".zzzz",
		strconv.FormatInt(now.Unix(), 10) + ".deadbeef",
		makeTestProof(strings.Repeat("b", 40), sid, "workstation", http.MethodPost, now.Unix()),
	} {
		if verifyProof(tok, sid, "workstation", http.MethodPost, bad, now) {
			t.Fatalf("expected proof %q to fail verification", bad)
		}
	}
	if verifyProof("", sid, "workstation", http.MethodPost, valid, now) {
		t.Fatal("empty token must fail")
	}
}

func TestValidateEnvelopeCases(t *testing.T) {
	sid := "session_identifier_1234567890"
	valid := makeValidEnvelope(sid, "workstation", 1)
	if err := validateEnvelope(&valid, sid); err != nil {
		t.Fatalf("expected valid workstation envelope: %v", err)
	}
	validMob := makeValidEnvelope(sid, "mobile", 2)
	if err := validateEnvelope(&validMob, sid); err != nil {
		t.Fatalf("expected valid mobile envelope: %v", err)
	}

	cases := []func(*opaqueEnvelope){
		func(e *opaqueEnvelope) { e.Version = 99 },
		func(e *opaqueEnvelope) { e.SessionID = "other_session_id_123456" },
		func(e *opaqueEnvelope) { e.Direction = "mobile->workstation" },
		func(e *opaqueEnvelope) { e.SenderRole = "mobile"; e.Direction = "workstation->mobile" },
		func(e *opaqueEnvelope) { e.SenderRole = "observer" },
		func(e *opaqueEnvelope) { e.Seq = 0 },
		func(e *opaqueEnvelope) { e.Nonce = "short" },
		func(e *opaqueEnvelope) { e.Ciphertext = "short" },
	}
	for i, mutate := range cases {
		env := makeValidEnvelope(sid, "workstation", 1)
		mutate(&env)
		if err := validateEnvelope(&env, sid); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestClientIPHelpers(t *testing.T) {
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "192.168.1.50:54321"
	if got := clientIP(req1); got != "192.168.1.50" {
		t.Fatalf("got %q", got)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "10.0.0.5"
	if got := clientIP(req2); got != "10.0.0.5" {
		t.Fatalf("got %q", got)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "   "
	if got := clientIP(req3); got != "unknown" {
		t.Fatalf("got %q", got)
	}
}

func TestRelayServerHTTPFlowSendPollAuthAndErrors(t *testing.T) {
	srv := newRelayServer(relayConfig{
		MaxQueueDepth:   2,
		MaxFrameBytes:   2048,
		MaxSessions:     1,
		MaxPollTimeout:  200 * time.Millisecond,
		RateLimitPerSec: 100,
		RateBurst:       100,
	})
	defer srv.Close()

	// Health endpoint GET + 405
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/relay/v1/health", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"zero_knowledge":true`) {
		t.Fatalf("health status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/health", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("health POST status=%d", rr.Code)
	}

	// Routing error paths
	for _, tc := range []struct {
		path string
		want int
	}{
		{"/unknown", http.StatusNotFound},
		{"/relay/v1/sessions/noslash", http.StatusNotFound},
		{"/relay/v1/sessions/short/send", http.StatusBadRequest},
		{"/relay/v1/sessions/session_identifier_1234567890/other", http.StatusNotFound},
	} {
		rr = httptest.NewRecorder()
		srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != tc.want {
			t.Fatalf("path %s status=%d want=%d", tc.path, rr.Code, tc.want)
		}
	}

	sid := "session_identifier_1234567890"
	tok := strings.Repeat("x", 40)

	// Method & payload errors on /send
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/send", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("send GET status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/sessions/"+sid+"/send", nil))
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("send empty status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/sessions/"+sid+"/send", strings.NewReader(strings.Repeat("a", 3000))))
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("send oversized status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/sessions/"+sid+"/send", strings.NewReader("{bad")))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("send bad json status=%d", rr.Code)
	}

	badEnv := makeValidEnvelope(sid, "workstation", 0)
	rawBadEnv, _ := json.Marshal(badEnv)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/sessions/"+sid+"/send", bytes.NewReader(rawBadEnv)))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("send invalid envelope status=%d", rr.Code)
	}

	// Auth error checks on /send
	sendFrame := func(sidVal, role string, seq uint64, authHeader, proof string) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(makeValidEnvelope(sidVal, role, seq))
		req := httptest.NewRequest(http.MethodPost, "/relay/v1/sessions/"+sidVal+"/send", bytes.NewReader(raw))
		if authHeader != "" {
			req.Header.Set("Authorization", authHeader)
		}
		if proof != "" {
			req.Header.Set("X-Relay-Proof", proof)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	if rec := sendFrame(sid, "workstation", 1, "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status=%d", rec.Code)
	}
	if rec := sendFrame(sid, "workstation", 1, "Bearer short", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("short bearer status=%d", rec.Code)
	}
	if rec := sendFrame(sid, "workstation", 1, "Bearer "+tok, "badproof"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad proof status=%d", rec.Code)
	}

	// Valid workstation->mobile sends + queue full 429
	nowTs := time.Now().Unix()
	wsProof := makeTestProof(tok, sid, "workstation", http.MethodPost, nowTs)
	if rec := sendFrame(sid, "workstation", 1, "Bearer "+tok, wsProof); rec.Code != http.StatusAccepted {
		t.Fatalf("valid send 1 status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := sendFrame(sid, "workstation", 2, "Bearer "+tok, wsProof); rec.Code != http.StatusAccepted {
		t.Fatalf("valid send 2 status=%d", rec.Code)
	}
	if rec := sendFrame(sid, "workstation", 3, "Bearer "+tok, wsProof); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("queue full status=%d want 429", rec.Code)
	}

	// Wrong token on existing session -> 401
	wrongTok := strings.Repeat("y", 40)
	if rec := sendFrame(sid, "workstation", 4, "Bearer "+wrongTok, makeTestProof(wrongTok, sid, "workstation", http.MethodPost, nowTs)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token mismatch status=%d", rec.Code)
	}

	// Session capacity limit (MaxSessions=1) -> 503 for second session
	sid2 := "second_session_identifier_99999"
	if rec := sendFrame(sid2, "workstation", 1, "Bearer "+tok, makeTestProof(tok, sid2, "workstation", http.MethodPost, nowTs)); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("capacity limit status=%d want 503", rec.Code)
	}

	// Mobile->workstation send + queue full 429
	mobProof := makeTestProof(tok, sid, "mobile", http.MethodPost, nowTs)
	if rec := sendFrame(sid, "mobile", 1, "Bearer "+tok, mobProof); rec.Code != http.StatusAccepted {
		t.Fatalf("mobile send 1 status=%d", rec.Code)
	}
	if rec := sendFrame(sid, "mobile", 2, "Bearer "+tok, mobProof); rec.Code != http.StatusAccepted {
		t.Fatalf("mobile send 2 status=%d", rec.Code)
	}
	if rec := sendFrame(sid, "mobile", 3, "Bearer "+tok, mobProof); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("mobile queue full status=%d", rec.Code)
	}

	// Poll error checks
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/relay/v1/sessions/"+sid+"/poll?role=mobile", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("poll POST status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/poll?role=invalid", nil))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("poll invalid role status=%d", rr.Code)
	}
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/poll?role=mobile", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("poll unauth status=%d", rr.Code)
	}

	// Partial drain (max=1) then remaining drain for mobile and workstation
	pollReq := func(role, query string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/poll?role="+role+query, nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("X-Relay-Proof", makeTestProof(tok, sid, role, http.MethodGet, time.Now().Unix()))
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	rec1 := pollReq("mobile", "&max=1")
	if rec1.Code != http.StatusOK || !strings.Contains(rec1.Body.String(), `"seq":1`) {
		t.Fatalf("mobile poll 1 status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	rec2 := pollReq("mobile", "&max=10")
	if rec2.Code != http.StatusOK || !strings.Contains(rec2.Body.String(), `"seq":2`) {
		t.Fatalf("mobile poll 2 status=%d body=%s", rec2.Code, rec2.Body.String())
	}

	// Long-poll timeout returning empty frames
	recTimeout := pollReq("mobile", "&timeout=50ms")
	if recTimeout.Code != http.StatusOK || !strings.Contains(recTimeout.Body.String(), `"frames":[]`) {
		t.Fatalf("mobile timeout poll status=%d body=%s", recTimeout.Code, recTimeout.Body.String())
	}

	// Workstation drain
	recWS := pollReq("workstation", "&max=10&timeout=1")
	if recWS.Code != http.StatusOK || !strings.Contains(recWS.Body.String(), `"seq":1`) {
		t.Fatalf("workstation poll status=%d body=%s", recWS.Code, recWS.Body.String())
	}

	// Long-poll woken by concurrent send
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- pollReq("workstation", "&timeout=150ms")
	}()
	time.Sleep(20 * time.Millisecond)
	_ = sendFrame(sid, "mobile", 10, "Bearer "+tok, makeTestProof(tok, sid, "mobile", http.MethodPost, time.Now().Unix()))
	recWake := <-done
	if recWake.Code != http.StatusOK || !strings.Contains(recWake.Body.String(), `"seq":10`) {
		t.Fatalf("woken poll status=%d body=%s", recWake.Code, recWake.Body.String())
	}

	// Cancelled context during long poll
	ctx, cancel := context.WithCancel(context.Background())
	reqCancel := httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/poll?role=workstation&timeout=200ms", nil).WithContext(ctx)
	reqCancel.Header.Set("Authorization", "Bearer "+tok)
	reqCancel.Header.Set("X-Relay-Proof", makeTestProof(tok, sid, "workstation", http.MethodGet, time.Now().Unix()))
	cancel()
	srv.ServeHTTP(httptest.NewRecorder(), reqCancel)
}

func TestRelayServerRateLimitAndRunFlags(t *testing.T) {
	srv := newRelayServer(relayConfig{RateLimitPerSec: 0.01, RateBurst: 1})
	defer srv.Close()
	sid := "session_identifier_1234567890"
	r1 := httptest.NewRecorder()
	srv.ServeHTTP(r1, httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/poll?role=mobile", nil))
	r2 := httptest.NewRecorder()
	srv.ServeHTTP(r2, httptest.NewRequest(http.MethodGet, "/relay/v1/sessions/"+sid+"/poll?role=mobile", nil))
	if r2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 rate limit, got %d", r2.Code)
	}

	var out, errBuf bytes.Buffer
	if code := run([]string{"--unknown-flag"}, &out, &errBuf); code != 2 {
		t.Fatalf("unknown flag exit=%d want 2", code)
	}
	if code := run([]string{"--tls-cert", "only-cert.pem"}, &out, &errBuf); code != 2 {
		t.Fatalf("mismatched tls flags exit=%d want 2", code)
	}
	if code := run([]string{"--listen", "invalid:addr:port:99999"}, &out, &errBuf); code != 1 {
		t.Fatalf("invalid listen addr exit=%d want 1", code)
	}
}
