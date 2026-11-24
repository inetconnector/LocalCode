// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestE2EEHKDFRFC5869AndDomainSeparation(t *testing.T) {
	// RFC 5869 Appendix A.1 (Basic test case with SHA-256)
	ikm, _ := hex.DecodeString("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	wantOKM := "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"

	gotOKM, err := DeriveHKDFSHA256(ikm, salt, info, 42)
	if err != nil {
		t.Fatalf("DeriveHKDFSHA256 failed: %v", err)
	}
	if hex.EncodeToString(gotOKM) != wantOKM {
		t.Fatalf("RFC 5869 A.1 mismatch:\ngot:  %s\nwant: %s", hex.EncodeToString(gotOKM), wantOKM)
	}

	// X25519 + HKDF-SHA256 directional key derivation & domain separation
	wsKP, err := GenerateRelayKeyPair()
	if err != nil {
		t.Fatalf("workstation keypair: %v", err)
	}
	mobKP, err := GenerateRelayKeyPair()
	if err != nil {
		t.Fatalf("mobile keypair: %v", err)
	}
	sid, err := GenerateRelaySessionID()
	if err != nil {
		t.Fatalf("session id: %v", err)
	}

	wsKeys, err := DeriveRelaySessionKeys(wsKP.Private, mobKP.Public, sid, RelayRoleWorkstation)
	if err != nil {
		t.Fatalf("workstation derive keys: %v", err)
	}
	mobKeys, err := DeriveRelaySessionKeys(mobKP.Private, wsKP.Public, sid, RelayRoleMobile)
	if err != nil {
		t.Fatalf("mobile derive keys: %v", err)
	}

	if !bytes.Equal(wsKeys.WorkstationToMobileKey, mobKeys.WorkstationToMobileKey) {
		t.Fatal("workstation->mobile key mismatch between endpoints")
	}
	if !bytes.Equal(wsKeys.MobileToWorkstationKey, mobKeys.MobileToWorkstationKey) {
		t.Fatal("mobile->workstation key mismatch between endpoints")
	}
	if bytes.Equal(wsKeys.WorkstationToMobileKey, wsKeys.MobileToWorkstationKey) {
		t.Fatal("directional keys must be distinct")
	}
	if wsKeys.SafetyFingerprint == "" || wsKeys.SafetyFingerprint != mobKeys.SafetyFingerprint {
		t.Fatalf("safety fingerprint mismatch: ws=%q mob=%q", wsKeys.SafetyFingerprint, mobKeys.SafetyFingerprint)
	}

	otherSID, _ := GenerateRelaySessionID()
	otherKeys, err := DeriveRelaySessionKeys(wsKP.Private, mobKP.Public, otherSID, RelayRoleWorkstation)
	if err != nil {
		t.Fatalf("derive other session keys: %v", err)
	}
	if bytes.Equal(wsKeys.WorkstationToMobileKey, otherKeys.WorkstationToMobileKey) {
		t.Fatal("different session IDs must derive different keys")
	}
}

func TestE2EEPairingURIAndFingerprintVerification(t *testing.T) {
	wsKP, err := GenerateRelayKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := GenerateRelaySessionID()
	if err != nil {
		t.Fatal(err)
	}
	authSecret, err := GenerateRelayAuthSecret()
	if err != nil {
		t.Fatal(err)
	}

	uri, fp, err := GenerateRelayPairingURI("https://relay.localcode.example:8443", sid, wsKP.Public, authSecret)
	if err != nil {
		t.Fatalf("GenerateRelayPairingURI: %v", err)
	}
	if !strings.HasPrefix(uri, "localcode://relay-pair?") {
		t.Fatalf("unexpected pairing URI format: %s", uri)
	}

	parsed, err := ParseRelayPairingURI(uri)
	if err != nil {
		t.Fatalf("ParseRelayPairingURI: %v", err)
	}
	if parsed.SessionID != sid {
		t.Fatalf("sid=%q want=%q", parsed.SessionID, sid)
	}
	if !bytes.Equal(parsed.PeerPublicRaw, wsKP.Public.Bytes()) {
		t.Fatal("parsed public key mismatch")
	}
	if !bytes.Equal(parsed.AuthSecret, authSecret) {
		t.Fatal("parsed auth secret mismatch")
	}
	if !parsed.VerifyFingerprint(fp) {
		t.Fatal("fingerprint verification failed for valid pairing payload")
	}
	if parsed.VerifyFingerprint(strings.Repeat("0", 64)) {
		t.Fatal("fingerprint verification must reject mismatched fingerprint")
	}

	// Tampered fingerprint in URI must be rejected
	tamperedFP := strings.Replace(uri, fp, strings.Repeat("a", 64), 1)
	if _, err := ParseRelayPairingURI(tamperedFP); !errors.Is(err, ErrRelayFingerprintFail) {
		t.Fatalf("expected ErrRelayFingerprintFail for tampered fp, got %v", err)
	}

	// Non-HTTPS remote relay URL must be rejected fail-closed
	if _, _, err := GenerateRelayPairingURI("http://relay.public.example:8080", sid, wsKP.Public, authSecret); err == nil {
		t.Fatal("expected error for non-HTTPS public relay URL")
	}
}

func TestE2EECipherTamperAndReplayProtection(t *testing.T) {
	wsKP, _ := GenerateRelayKeyPair()
	mobKP, _ := GenerateRelayKeyPair()
	sid, _ := GenerateRelaySessionID()

	wsKeys, err := DeriveRelaySessionKeys(wsKP.Private, mobKP.Public, sid, RelayRoleWorkstation)
	if err != nil {
		t.Fatal(err)
	}
	mobKeys, err := DeriveRelaySessionKeys(mobKP.Private, wsKP.Public, sid, RelayRoleMobile)
	if err != nil {
		t.Fatal(err)
	}
	wsCipher, err := NewRelayCipherState(wsKeys, RelayRoleWorkstation)
	if err != nil {
		t.Fatal(err)
	}
	mobCipher, err := NewRelayCipherState(mobKeys, RelayRoleMobile)
	if err != nil {
		t.Fatal(err)
	}

	env1, err := mobCipher.Seal([]byte("hello-1"))
	if err != nil {
		t.Fatal(err)
	}
	env2, err := mobCipher.Seal([]byte("hello-2"))
	if err != nil {
		t.Fatal(err)
	}
	env3, err := mobCipher.Seal([]byte("hello-3"))
	if err != nil {
		t.Fatal(err)
	}

	// Out-of-order delivery within window succeeds (seq 2 before seq 1)
	pt2, err := wsCipher.Open(env2)
	if err != nil || string(pt2) != "hello-2" {
		t.Fatalf("open env2 failed: %v %q", err, pt2)
	}
	pt1, err := wsCipher.Open(env1)
	if err != nil || string(pt1) != "hello-1" {
		t.Fatalf("open env1 failed: %v %q", err, pt1)
	}

	// Replay of seq 1 or seq 2 MUST be rejected
	if _, err := wsCipher.Open(env1); !errors.Is(err, ErrRelayReplayDetected) {
		t.Fatalf("replay of env1: got err=%v want=%v", err, ErrRelayReplayDetected)
	}
	if _, err := wsCipher.Open(env2); !errors.Is(err, ErrRelayReplayDetected) {
		t.Fatalf("replay of env2: got err=%v want=%v", err, ErrRelayReplayDetected)
	}

	// Tampered ciphertext MUST be rejected AND must not consume seq 3
	tampered := *env3
	ctRaw, _ := base64.RawURLEncoding.DecodeString(tampered.Ciphertext)
	ctRaw[0] ^= 0xff
	tampered.Ciphertext = base64.RawURLEncoding.EncodeToString(ctRaw)
	if _, err := wsCipher.Open(&tampered); !errors.Is(err, ErrRelayDecryptFailed) {
		t.Fatalf("tampered ciphertext: got err=%v want=%v", err, ErrRelayDecryptFailed)
	}

	// Forged high-seq packet with bad MAC MUST NOT advance the sliding replay window
	forgedHigh := *env3
	forgedHigh.Seq = 999999
	forgedNonce := buildRelayNonce(forgedHigh.Direction, forgedHigh.Seq)
	forgedHigh.Nonce = base64.RawURLEncoding.EncodeToString(forgedNonce[:])
	if _, err := wsCipher.Open(&forgedHigh); !errors.Is(err, ErrRelayDecryptFailed) {
		t.Fatalf("forged high seq: got err=%v want=%v", err, ErrRelayDecryptFailed)
	}

	// Legitimate seq 3 MUST still succeed because unauthenticated forged frames did not advance window
	pt3, err := wsCipher.Open(env3)
	if err != nil || string(pt3) != "hello-3" {
		t.Fatalf("legitimate env3 after forged attempts failed: %v %q", err, pt3)
	}

	// Advance past 128-frame window and verify stale frame is rejected
	staleEnv, err := mobCipher.Seal([]byte("will-become-stale"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 135; i++ {
		e, err := mobCipher.Seal([]byte("advance"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := wsCipher.Open(e); err != nil {
			t.Fatalf("advance open %d failed: %v", i, err)
		}
	}
	if _, err := wsCipher.Open(staleEnv); !errors.Is(err, ErrRelayReplayDetected) {
		t.Fatalf("stale frame outside replay window: got err=%v want=%v", err, ErrRelayReplayDetected)
	}
}

// wireSnifferMiddleware captures 100% of raw HTTP request and response bodies seen by the relay.
type wireSnifferMiddleware struct {
	next http.Handler
	mu   sync.Mutex
	wire bytes.Buffer
}

func (w *wireSnifferMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	reqBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(reqBody))

	rec := httptest.NewRecorder()
	w.next.ServeHTTP(rec, r)

	respBody := rec.Body.Bytes()
	for k, vals := range rec.Header() {
		for _, v := range vals {
			rw.Header().Add(k, v)
		}
	}
	rw.WriteHeader(rec.Code)
	_, _ = rw.Write(respBody)

	w.mu.Lock()
	w.wire.WriteString(r.Method + " " + r.URL.RequestURI() + "\n")
	w.wire.Write(reqBody)
	w.wire.WriteByte('\n')
	w.wire.Write(respBody)
	w.wire.WriteByte('\n')
	w.mu.Unlock()
}

func (w *wireSnifferMiddleware) Snapshot() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wire.String()
}

func setupPairedRelayHarness(t *testing.T, localHandler http.Handler) (
	relaySrv *ZeroKnowledgeRelayServer,
	sniffer *wireSnifferMiddleware,
	bridge *RelayWorkstationBridge,
	mobile *RelayMobileClient,
	relayHTTP *httptest.Server,
	cleanup func(),
) {
	t.Helper()

	localHTTP := httptest.NewServer(localHandler)
	relaySrv = NewZeroKnowledgeRelayServer(ZeroKnowledgeRelayConfig{
		MaxQueueDepth:   DefaultRelayMaxQueueDepth,
		MaxFrameBytes:   DefaultRelayMaxFrameBytes,
		SessionIdleTTL:  5 * time.Minute,
		MaxPollTimeout:  2 * time.Second,
		RateLimitPerSec: 1000,
		RateBurst:       1000,
	})
	sniffer = &wireSnifferMiddleware{next: relaySrv}
	relayHTTP = httptest.NewServer(sniffer)

	wsKP, err := GenerateRelayKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	mobKP, err := GenerateRelayKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	sid, err := GenerateRelaySessionID()
	if err != nil {
		t.Fatal(err)
	}
	authSecret, err := GenerateRelayAuthSecret()
	if err != nil {
		t.Fatal(err)
	}

	// Verify pairing URI roundtrip as part of setup
	pairURI, fp, err := GenerateRelayPairingURI(relayHTTP.URL, sid, wsKP.Public, authSecret)
	if err != nil {
		t.Fatal(err)
	}
	parsedPair, err := ParseRelayPairingURI(pairURI)
	if err != nil || !parsedPair.VerifyFingerprint(fp) {
		t.Fatalf("pairing roundtrip failed: %v", err)
	}

	relayAuthToken, err := DeriveRelayAuthToken(parsedPair.AuthSecret, parsedPair.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	wsKeys, err := DeriveRelaySessionKeys(wsKP.Private, mobKP.Public, sid, RelayRoleWorkstation)
	if err != nil {
		t.Fatal(err)
	}
	mobKeys, err := DeriveRelaySessionKeys(mobKP.Private, parsedPair.PeerPublicKey, parsedPair.SessionID, RelayRoleMobile)
	if err != nil {
		t.Fatal(err)
	}
	wsCipher, err := NewRelayCipherState(wsKeys, RelayRoleWorkstation)
	if err != nil {
		t.Fatal(err)
	}
	mobCipher, err := NewRelayCipherState(mobKeys, RelayRoleMobile)
	if err != nil {
		t.Fatal(err)
	}

	bridge, err = NewRelayWorkstationBridge(RelayWorkstationBridgeConfig{
		RelayURL:       relayHTTP.URL,
		LocalTargetURL: localHTTP.URL,
		LocalAuthToken: "local-loopback-token",
		RelayAuthToken: relayAuthToken,
		Cipher:         wsCipher,
		HTTPClient:     relayHTTP.Client(),
		LocalClient:    localHTTP.Client(),
		PollTimeout:    300 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}

	mobile, err = NewRelayMobileClient(parsedPair.RelayURL, relayAuthToken, mobCipher, relayHTTP.Client())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := bridge.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}

	cleanup = func() {
		cancel()
		bridge.Stop()
		relaySrv.Close()
		relayHTTP.Close()
		localHTTP.Close()
	}
	return relaySrv, sniffer, bridge, mobile, relayHTTP, cleanup
}

func TestRelayEndToEndZeroKnowledgeTunnel(t *testing.T) {
	const (
		secretPrompt = "Refactor payment vault using SUPER_SECRET_API_KEY_99887766"
		secretCode   = "func unlockMasterVault() string { return \"TOP_SECRET_CIPHER_MATERIAL_42\" }"
		secretDiff   = "@@ -10,2 +10,2 @@ -oldSecretToken +newSecretToken_XYZ123"
	)

	localMux := http.NewServeMux()
	localMux.HandleFunc("/remote/api/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "e2ee-loopback"})
	})
	localMux.HandleFunc("/remote/api/chat", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer local-loopback-token" {
			http.Error(w, "missing local loopback auth", http.StatusUnauthorized)
			return
		}
		var req map[string]string
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":     true,
			"echo":   req["prompt"],
			"patch":  secretCode,
			"diff":   secretDiff,
		})
	})

	_, sniffer, _, mobile, _, cleanup := setupPairedRelayHarness(t, localMux)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Ping over E2EE relay
	pingResp, err := mobile.DoRPC(ctx, RelayRPCRequest{
		Method: http.MethodGet,
		Path:   "/remote/api/ping",
	})
	if err != nil {
		t.Fatalf("DoRPC ping failed: %v", err)
	}
	if pingResp.StatusCode != http.StatusOK || !strings.Contains(string(pingResp.Body), "e2ee-loopback") {
		t.Fatalf("unexpected ping response: status=%d body=%s", pingResp.StatusCode, string(pingResp.Body))
	}

	// 2. Chat containing confidential prompt/code/diff over E2EE relay
	chatBody, _ := json.Marshal(map[string]string{"prompt": secretPrompt})
	chatResp, err := mobile.DoRPC(ctx, RelayRPCRequest{
		Method:  http.MethodPost,
		Path:    "/remote/api/chat",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    chatBody,
	})
	if err != nil {
		t.Fatalf("DoRPC chat failed: %v", err)
	}
	if chatResp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected chat status=%d body=%s", chatResp.StatusCode, string(chatResp.Body))
	}
	var parsedChat map[string]any
	if err := json.Unmarshal(chatResp.Body, &parsedChat); err != nil {
		t.Fatalf("decode chat response json: %v", err)
	}
	if parsedChat["echo"] != secretPrompt || parsedChat["patch"] != secretCode || parsedChat["diff"] != secretDiff {
		t.Fatalf("decrypted response missing expected content: %+v", parsedChat)
	}

	// 3. ZERO-KNOWLEDGE PROOF: wire sniffer on the relay must observe 0 bytes of plaintext!
	wire := sniffer.Snapshot()
	if len(wire) == 0 {
		t.Fatal("wire sniffer captured no traffic")
	}
	for _, secret := range []string{
		secretPrompt,
		"SUPER_SECRET_API_KEY_99887766",
		secretCode,
		"TOP_SECRET_CIPHER_MATERIAL_42",
		secretDiff,
		"newSecretToken_XYZ123",
		"/remote/api/chat",
		"/remote/api/ping",
		"e2ee-loopback",
	} {
		if strings.Contains(wire, secret) {
			t.Fatalf("ZERO-KNOWLEDGE VIOLATION: relay wire sniffer observed plaintext %q", secret)
		}
	}
}

func TestRelayWireTamperAndReplayRejectedByBridge(t *testing.T) {
	var localHits atomic.Int64
	localMux := http.NewServeMux()
	localMux.HandleFunc("/remote/api/ping", func(w http.ResponseWriter, r *http.Request) {
		localHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	_, _, bridge, mobile, _, cleanup := setupPairedRelayHarness(t, localMux)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Seal one legitimate envelope manually so we can replay it after sending
	rawReq, _ := json.Marshal(&RelayRPCRequest{
		RequestID: "legit-1",
		Method:    http.MethodGet,
		Path:      "/remote/api/ping",
	})
	legitEnv, err := mobile.cipher.Seal(rawReq)
	if err != nil {
		t.Fatal(err)
	}

	// Send legitimate envelope and read encrypted response
	status, err := mobile.SendEnvelope(ctx, legitEnv)
	if err != nil || status != http.StatusAccepted {
		t.Fatalf("send legit envelope failed: status=%d err=%v", status, err)
	}
	frames, err := mobile.PollEnvelopes(ctx, 3*time.Second)
	if err != nil || len(frames) != 1 {
		t.Fatalf("poll legit response failed: len=%d err=%v", len(frames), err)
	}
	if _, err := mobile.cipher.Open(&frames[0]); err != nil {
		t.Fatalf("open legit response failed: %v", err)
	}
	if localHits.Load() != 1 {
		t.Fatalf("expected 1 local hit, got %d", localHits.Load())
	}

	// 1. Replay the EXACT same envelope (same Seq) -> bridge MUST reject without hitting local server
	if status, err := mobile.SendEnvelope(ctx, legitEnv); err != nil || status != http.StatusAccepted {
		t.Fatalf("replay send failed: status=%d err=%v", status, err)
	}

	// 2. Send tampered ciphertext envelope (new Seq, flipped bit) -> bridge MUST reject
	tamperedEnv, err := mobile.cipher.Seal(rawReq)
	if err != nil {
		t.Fatal(err)
	}
	ctBytes, _ := base64.RawURLEncoding.DecodeString(tamperedEnv.Ciphertext)
	ctBytes[len(ctBytes)-1] ^= 0x01
	tamperedEnv.Ciphertext = base64.RawURLEncoding.EncodeToString(ctBytes)
	if status, err := mobile.SendEnvelope(ctx, tamperedEnv); err != nil || status != http.StatusAccepted {
		t.Fatalf("tampered send failed: status=%d err=%v", status, err)
	}

	// Wait until bridge processes and drops both malicious frames
	deadline := time.Now().Add(4 * time.Second)
	for bridge.RejectedFrames() < 2 && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if bridge.RejectedFrames() < 2 {
		t.Fatalf("expected >= 2 rejected frames (replay + tamper), got %d", bridge.RejectedFrames())
	}
	if localHits.Load() != 1 {
		t.Fatalf("replayed/tampered frames must NEVER reach local server; localHits=%d", localHits.Load())
	}

	// 3. Subsequent legitimate RPC must still work smoothly
	resp, err := mobile.DoRPC(ctx, RelayRPCRequest{Method: http.MethodGet, Path: "/remote/api/ping"})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("subsequent legit RPC failed: status=%v err=%v", resp, err)
	}
	if localHits.Load() != 2 {
		t.Fatalf("expected 2 local hits after legit RPC, got %d", localHits.Load())
	}
}

func TestRelayMobileAuthorityFailClosed(t *testing.T) {
	var forbiddenHits atomic.Int64
	var approvedOnceHits atomic.Int64

	localMux := http.NewServeMux()
	localMux.HandleFunc("/api/mission-recovery", func(w http.ResponseWriter, r *http.Request) {
		forbiddenHits.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	localMux.HandleFunc("/api/mission-recovery/continue", func(w http.ResponseWriter, r *http.Request) {
		forbiddenHits.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	localMux.HandleFunc("/remote/api/approve", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			ID       string `json:"id"`
			Decision string `json:"decision"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if strings.EqualFold(payload.Decision, "global") {
			forbiddenHits.Add(1)
		} else {
			approvedOnceHits.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"decision":"once"}`))
	})

	_, _, _, mobile, _, cleanup := setupPairedRelayHarness(t, localMux)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Unauthorized routes must fail-closed (403/404) and NEVER reach the local server
	blockedCases := []struct {
		name   string
		req    RelayRPCRequest
		expect []int
	}{
		{
			name:   "mission-recovery GET",
			req:    RelayRPCRequest{Method: http.MethodGet, Path: "/api/mission-recovery"},
			expect: []int{http.StatusForbidden, http.StatusNotFound},
		},
		{
			name:   "mission-recovery continue POST",
			req:    RelayRPCRequest{Method: http.MethodPost, Path: "/api/mission-recovery/continue", Body: []byte(`{}`)},
			expect: []int{http.StatusForbidden, http.StatusNotFound},
		},
		{
			name:   "remote mission-recovery GET",
			req:    RelayRPCRequest{Method: http.MethodGet, Path: "/remote/api/mission-recovery"},
			expect: []int{http.StatusForbidden, http.StatusNotFound},
		},
		{
			name:   "path traversal to mission-recovery",
			req:    RelayRPCRequest{Method: http.MethodGet, Path: "/remote/api/../api/mission-recovery"},
			expect: []int{http.StatusForbidden, http.StatusNotFound},
		},
		{
			name:   "desktop-only route /api/settings",
			req:    RelayRPCRequest{Method: http.MethodGet, Path: "/api/settings"},
			expect: []int{http.StatusForbidden, http.StatusNotFound},
		},
		{
			name: "approve decision global forbidden",
			req: RelayRPCRequest{
				Method: http.MethodPost,
				Path:   "/remote/api/approve",
				Body:   []byte(`{"id":"ap-1","approve":true,"decision":"global"}`),
			},
			expect: []int{http.StatusForbidden},
		},
		{
			name: "approve scope global forbidden",
			req: RelayRPCRequest{
				Method: http.MethodPost,
				Path:   "/remote/api/approve",
				Body:   []byte(`{"id":"ap-1","approve":true,"scope":"global"}`),
			},
			expect: []int{http.StatusForbidden},
		},
		{
			name: "approve malformed JSON fail-closed",
			req: RelayRPCRequest{
				Method: http.MethodPost,
				Path:   "/remote/api/approve",
				Body:   []byte(`{not-json`),
			},
			expect: []int{http.StatusBadRequest},
		},
	}

	for _, tc := range blockedCases {
		resp, err := mobile.DoRPC(ctx, tc.req)
		if err != nil {
			t.Fatalf("%s: DoRPC error: %v", tc.name, err)
		}
		matched := false
		for _, want := range tc.expect {
			if resp.StatusCode == want {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("%s: got status=%d want one of %v (body=%s)", tc.name, resp.StatusCode, tc.expect, string(resp.Body))
		}
	}

	if forbiddenHits.Load() != 0 {
		t.Fatalf("forbidden route/decision reached local workstation server %d times", forbiddenHits.Load())
	}

	// Legitimate "once" approval succeeds
	okResp, err := mobile.DoRPC(ctx, RelayRPCRequest{
		Method: http.MethodPost,
		Path:   "/remote/api/approve",
		Body:   []byte(`{"id":"ap-1","approve":true,"decision":"once"}`),
	})
	if err != nil || okResp.StatusCode != http.StatusOK {
		t.Fatalf("valid once approval failed: resp=%+v err=%v", okResp, err)
	}
	if approvedOnceHits.Load() != 1 {
		t.Fatalf("expected 1 approved-once hit on local server, got %d", approvedOnceHits.Load())
	}

	// Anti-SSRF: WorkstationBridge must refuse non-loopback LocalTargetURL
	wsKP, _ := GenerateRelayKeyPair()
	mobKP, _ := GenerateRelayKeyPair()
	sid, _ := GenerateRelaySessionID()
	wsKeys, _ := DeriveRelaySessionKeys(wsKP.Private, mobKP.Public, sid, RelayRoleWorkstation)
	wsCipher, _ := NewRelayCipherState(wsKeys, RelayRoleWorkstation)
	if _, err := NewRelayWorkstationBridge(RelayWorkstationBridgeConfig{
		RelayURL:       "https://relay.example.com",
		LocalTargetURL: "http://169.254.169.254",
		RelayAuthToken: strings.Repeat("x", 40),
		Cipher:         wsCipher,
	}); err == nil {
		t.Fatal("bridge must reject non-loopback LocalTargetURL")
	}
}

func TestRelayServerBoundsAndAuth(t *testing.T) {
	srv := NewZeroKnowledgeRelayServer(ZeroKnowledgeRelayConfig{
		MaxQueueDepth:   3,
		MaxFrameBytes:   1024,
		SessionIdleTTL:  50 * time.Millisecond,
		RateLimitPerSec: 1000,
		RateBurst:       1000,
	})
	defer srv.Close()

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// 1. Health check
	healthResp, err := ts.Client().Get(ts.URL + "/relay/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d want=200", healthResp.StatusCode)
	}
	var health map[string]any
	_ = json.NewDecoder(healthResp.Body).Decode(&health)
	if health["zero_knowledge"] != true {
		t.Fatalf("expected zero_knowledge=true, got %+v", health)
	}

	// Setup valid encrypted envelope
	wsKP, _ := GenerateRelayKeyPair()
	mobKP, _ := GenerateRelayKeyPair()
	sid, _ := GenerateRelaySessionID()
	authSecret, _ := GenerateRelayAuthSecret()
	authToken, _ := DeriveRelayAuthToken(authSecret, sid)
	mobKeys, _ := DeriveRelaySessionKeys(mobKP.Private, wsKP.Public, sid, RelayRoleMobile)
	mobCipher, _ := NewRelayCipherState(mobKeys, RelayRoleMobile)

	// 2. Unauthorized send (wrong token) -> 401
	env1, _ := mobCipher.Seal([]byte("frame-1"))
	if err := srv.RegisterSession(sid, authToken); err != nil {
		t.Fatal(err)
	}
	badStatus, err := sendRelayEnvelopeHTTP(context.Background(), ts.Client(), ts.URL, sid, RelayRoleMobile, strings.Repeat("z", 43), env1)
	if err != nil {
		t.Fatal(err)
	}
	if badStatus != http.StatusUnauthorized {
		t.Fatalf("wrong auth token status=%d want=401", badStatus)
	}

	// 3. Oversized frame (> 1024 bytes) -> 413
	bigEnv, _ := mobCipher.Seal(bytes.Repeat([]byte("A"), 2048))
	bigStatus, err := sendRelayEnvelopeHTTP(context.Background(), ts.Client(), ts.URL, sid, RelayRoleMobile, authToken, bigEnv)
	if err != nil {
		t.Fatal(err)
	}
	if bigStatus != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized frame status=%d want=413", bigStatus)
	}

	// 4. Bounded queue (max 3) -> 4th frame gets 429
	for i := 0; i < 3; i++ {
		e, _ := mobCipher.Seal([]byte("small"))
		st, err := sendRelayEnvelopeHTTP(context.Background(), ts.Client(), ts.URL, sid, RelayRoleMobile, authToken, e)
		if err != nil || st != http.StatusAccepted {
			t.Fatalf("queued frame %d failed: st=%d err=%v", i, st, err)
		}
	}
	e4, _ := mobCipher.Seal([]byte("overflow"))
	st4, err := sendRelayEnvelopeHTTP(context.Background(), ts.Client(), ts.URL, sid, RelayRoleMobile, authToken, e4)
	if err != nil || st4 != http.StatusTooManyRequests {
		t.Fatalf("overflow frame status=%d want=429 (err=%v)", st4, err)
	}

	// 5. Idle session expiration cleanup
	time.Sleep(80 * time.Millisecond)
	removed := srv.PurgeExpiredSessions()
	if removed < 1 || srv.ActiveSessionCount() != 0 {
		t.Fatalf("expected expired session purge: removed=%d active=%d", removed, srv.ActiveSessionCount())
	}
}

func TestPushUnifiedPushFCMAndAPNsZeroSecretPolicy(t *testing.T) {
	signingKey := make([]byte, 32)
	_, _ = rand.Read(signingKey)
	tokenMgr, err := NewLockscreenActionTokenManager(signingKey, LockscreenActionTokenDefaultTTL)
	if err != nil {
		t.Fatal(err)
	}

	var (
		mu          sync.Mutex
		upHeaders   http.Header
		upRawBody   string
		fcmHeaders  http.Header
		fcmRawBody  string
		apnsHeaders http.Header
		apnsRawBody string
	)

	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		switch r.URL.Path {
		case "/unifiedpush":
			upHeaders = r.Header.Clone()
			upRawBody = string(raw)
		case "/fcm":
			fcmHeaders = r.Header.Clone()
			fcmRawBody = string(raw)
		case "/apns":
			apnsHeaders = r.Header.Clone()
			apnsRawBody = string(raw)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	}))
	defer mockGateway.Close()

	dispatcher, err := NewPushNotificationDispatcher(tokenMgr, mockGateway.Client())
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// 1. UnifiedPush delivery + Zero-Secret-Push-Policy verification (even with adversarial dirty project input!)
	sentPayload, err := dispatcher.SendApprovalNotification(context.Background(), PushTarget{
		Provider: PushProviderUnifiedPush,
		DeviceID: "pixel-9-pro",
		Endpoint: mockGateway.URL + "/unifiedpush",
	}, ApprovalPushEvent{
		ApprovalID:  "apr-9f8e7d6c5b4a",
		ProjectName: "C:\\Users\\dev\\LocalCode",
		Timestamp:   now,
	})
	if err != nil {
		t.Fatalf("UnifiedPush dispatch failed: %v", err)
	}

	mu.Lock()
	gotUPHeaders := upHeaders
	gotUPBody := upRawBody
	mu.Unlock()

	if gotUPHeaders.Get("TTL") != "120" {
		t.Fatalf("UnifiedPush TTL=%q want=120", gotUPHeaders.Get("TTL"))
	}
	if gotUPHeaders.Get("Urgency") != "high" {
		t.Fatalf("UnifiedPush Urgency=%q want=high", gotUPHeaders.Get("Urgency"))
	}
	if !strings.Contains(gotUPBody, "Approval erforderlich") || !strings.Contains(gotUPBody, "LocalCode") {
		t.Fatalf("UnifiedPush body missing required metadata: %s", gotUPBody)
	}
	if strings.Contains(gotUPBody, `C:\\Users\\dev`) || strings.Contains(gotUPBody, "Users") {
		t.Fatalf("UnifiedPush leaked filesystem path in payload: %s", gotUPBody)
	}

	// Adversarial Zero-Code-Leakage test: attempt to smuggle code, diffs, and API keys via ProjectName
	_, err = dispatcher.SendApprovalNotification(context.Background(), PushTarget{
		Provider: PushProviderUnifiedPush,
		DeviceID: "pixel-9-pro",
		Endpoint: mockGateway.URL + "/unifiedpush",
	}, ApprovalPushEvent{
		ApprovalID:  "apr-adversarial-1",
		ProjectName: "sk-live-secret99999999 @@ -1,3 +1,3 @@ func hack() { return }",
		Timestamp:   now,
	})
	if err != nil {
		t.Fatalf("UnifiedPush adversarial dispatch failed: %v", err)
	}
	mu.Lock()
	advBody := upRawBody
	mu.Unlock()
	for _, forbidden := range []string{"sk-live", "secret99999999", "@@ -1,3", "func hack", "return"} {
		if strings.Contains(advBody, forbidden) {
			t.Fatalf("ZERO-SECRET PUSH VIOLATION: payload leaked %q in %s", forbidden, advBody)
		}
	}

	// Verify tokens embedded in first UnifiedPush payload work for once & reject
	claimsOnce, err := tokenMgr.VerifyAndConsumeActionToken(sentPayload.ApproveOnceToken, "apr-9f8e7d6c5b4a", "pixel-9-pro", now.Add(5*time.Second))
	if err != nil || claimsOnce.Decision != LockscreenDecisionOnce {
		t.Fatalf("UnifiedPush once token verification failed: claims=%+v err=%v", claimsOnce, err)
	}
	claimsReject, err := tokenMgr.VerifyAndConsumeActionToken(sentPayload.RejectToken, "apr-9f8e7d6c5b4a", "pixel-9-pro", now.Add(5*time.Second))
	if err != nil || claimsReject.Decision != LockscreenDecisionReject {
		t.Fatalf("UnifiedPush reject token verification failed: claims=%+v err=%v", claimsReject, err)
	}

	// 2. FCM HTTP v1 delivery
	_, err = dispatcher.SendApprovalNotification(context.Background(), PushTarget{
		Provider:    PushProviderFCMv1,
		DeviceID:    "android-fcm-1",
		Endpoint:    mockGateway.URL + "/fcm",
		DeviceToken: "fcm-reg-token-xyz",
		BearerToken: "oauth2-access-token-abc",
	}, ApprovalPushEvent{
		ApprovalID:  "apr-fcm-01",
		ProjectName: "LocalCode",
		Timestamp:   now,
	})
	if err != nil {
		t.Fatalf("FCM v1 dispatch failed: %v", err)
	}
	mu.Lock()
	gotFCMHeaders, gotFCMBody := fcmHeaders, fcmRawBody
	mu.Unlock()
	if gotFCMHeaders.Get("Authorization") != "Bearer oauth2-access-token-abc" {
		t.Fatalf("FCM Authorization header=%q", gotFCMHeaders.Get("Authorization"))
	}
	if !strings.Contains(gotFCMBody, `"priority":"high"`) || !strings.Contains(gotFCMBody, `"ttl":"120s"`) {
		t.Fatalf("FCM payload missing priority/ttl: %s", gotFCMBody)
	}

	// 3. APNs HTTP/2 JSON delivery
	_, err = dispatcher.SendApprovalNotification(context.Background(), PushTarget{
		Provider:    PushProviderAPNs,
		DeviceID:    "iphone-16",
		Endpoint:    mockGateway.URL + "/apns",
		DeviceToken: "deadbeefcafe",
		BearerToken: "apns-jwt-token",
		APNsTopic:   "com.inetconnector.localcode",
	}, ApprovalPushEvent{
		ApprovalID:  "apr-apns-01",
		ProjectName: "LocalCode",
		Timestamp:   now,
	})
	if err != nil {
		t.Fatalf("APNs dispatch failed: %v", err)
	}
	mu.Lock()
	gotAPNsHeaders, gotAPNsBody := apnsHeaders, apnsRawBody
	mu.Unlock()
	if gotAPNsHeaders.Get("apns-topic") != "com.inetconnector.localcode" || gotAPNsHeaders.Get("apns-priority") != "10" {
		t.Fatalf("APNs headers invalid: topic=%q priority=%q", gotAPNsHeaders.Get("apns-topic"), gotAPNsHeaders.Get("apns-priority"))
	}
	if !strings.Contains(gotAPNsBody, `"category":"LOCALCODE_APPROVAL"`) || !strings.Contains(gotAPNsBody, "Approval erforderlich") {
		t.Fatalf("APNs payload missing category/title: %s", gotAPNsBody)
	}
}

func TestLockscreenActionTokenSecurityAndReplay(t *testing.T) {
	signingKey := make([]byte, 32)
	_, _ = rand.Read(signingKey)
	mgr, err := NewLockscreenActionTokenManager(signingKey, 120*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// TTL > 120s must be rejected
	if _, err := NewLockscreenActionTokenManager(signingKey, 121*time.Second); err == nil {
		t.Fatal("expected error when TTL exceeds strict 120s maximum")
	}

	now := time.Now().UTC().Truncate(time.Second)

	// 1. Valid "once" token succeeds once, replay fails
	onceTok, _, err := mgr.IssueActionToken("apr-100", LockscreenDecisionOnce, "dev-1", now)
	if err != nil {
		t.Fatalf("issue once token: %v", err)
	}
	claims, err := mgr.VerifyAndConsumeActionToken(onceTok, "apr-100", "dev-1", now.Add(10*time.Second))
	if err != nil {
		t.Fatalf("verify once token: %v", err)
	}
	if claims.Decision != LockscreenDecisionOnce || claims.ApprovalID != "apr-100" || claims.DeviceID != "dev-1" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	// Second redemption attempt (Replay) MUST be rejected!
	if _, err := mgr.VerifyAndConsumeActionToken(onceTok, "apr-100", "dev-1", now.Add(11*time.Second)); !errors.Is(err, ErrActionTokenReplayed) {
		t.Fatalf("expected ErrActionTokenReplayed on second use, got %v", err)
	}

	// 2. Valid "reject" token succeeds once, replay fails
	rejTok, _, err := mgr.IssueActionToken("apr-101", LockscreenDecisionReject, "dev-1", now)
	if err != nil {
		t.Fatalf("issue reject token: %v", err)
	}
	rejClaims, err := mgr.VerifyAndConsumeActionToken(rejTok, "apr-101", "dev-1", now.Add(5*time.Second))
	if err != nil || rejClaims.Decision != LockscreenDecisionReject {
		t.Fatalf("verify reject token failed: claims=%+v err=%v", rejClaims, err)
	}
	if _, err := mgr.VerifyAndConsumeActionToken(rejTok, "apr-101", "dev-1", now.Add(6*time.Second)); !errors.Is(err, ErrActionTokenReplayed) {
		t.Fatalf("expected ErrActionTokenReplayed on reject replay, got %v", err)
	}

	// 3. "global" decision MUST be forbidden at issuance AND verification!
	if _, _, err := mgr.IssueActionToken("apr-102", LockscreenActionDecision("global"), "dev-1", now); !errors.Is(err, ErrActionDecisionForbidden) {
		t.Fatalf("issuing global decision must fail with ErrActionDecisionForbidden, got %v", err)
	}
	// Even if someone manually signs a valid HMAC over decision=global, verification MUST still reject it!
	forgedCanonical := "apr-102|global|dev-1|" +
		strconvFormatInt(now.Add(60*time.Second).Unix()) + "|abcdefghijklmnop123456"
	forgedMAC := computeActionTokenMAC(signingKey, forgedCanonical)
	forgedGlobalTok := base64.RawURLEncoding.EncodeToString([]byte(forgedCanonical)) + "." +
		base64.RawURLEncoding.EncodeToString(forgedMAC)
	if _, err := mgr.VerifyAndConsumeActionToken(forgedGlobalTok, "apr-102", "dev-1", now); !errors.Is(err, ErrActionDecisionForbidden) {
		t.Fatalf("verifying global token must fail with ErrActionDecisionForbidden, got %v", err)
	}

	// 4. Expired token (> 120s) MUST be rejected
	expTok, _, err := mgr.IssueActionToken("apr-103", LockscreenDecisionOnce, "dev-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.VerifyAndConsumeActionToken(expTok, "apr-103", "dev-1", now.Add(121*time.Second)); !errors.Is(err, ErrActionTokenExpired) {
		t.Fatalf("expired token must fail with ErrActionTokenExpired, got %v", err)
	}

	// 5. Tampered MAC (both first MAC byte and trailing base64 strict malleability) MUST be rejected
	freshTok, _, err := mgr.IssueActionToken("apr-104", LockscreenDecisionOnce, "dev-1", now)
	if err != nil {
		t.Fatal(err)
	}
	tokParts := strings.Split(freshTok, ".")
	macRaw, err := base64.RawURLEncoding.Strict().DecodeString(tokParts[1])
	if err != nil {
		t.Fatal(err)
	}
	macRaw[0] ^= 0xff
	tamperedMACTok := tokParts[0] + "." + base64.RawURLEncoding.EncodeToString(macRaw)
	if _, err := mgr.VerifyAndConsumeActionToken(tamperedMACTok, "apr-104", "dev-1", now); !errors.Is(err, ErrActionTokenInvalid) {
		t.Fatalf("tampered MAC token must fail with ErrActionTokenInvalid, got %v", err)
	}
	tamperedTrailing := freshTok[:len(freshTok)-1]
	if freshTok[len(freshTok)-1] == 'A' {
		tamperedTrailing += "B"
	} else {
		tamperedTrailing += "A"
	}
	if _, err := mgr.VerifyAndConsumeActionToken(tamperedTrailing, "apr-104", "dev-1", now); !errors.Is(err, ErrActionTokenInvalid) {
		t.Fatalf("tampered trailing base64 token must fail with ErrActionTokenInvalid, got %v", err)
	}

	// 6. Binding mismatch (wrong approvalID or wrong deviceID) MUST be rejected and NOT consume nonce
	if _, err := mgr.VerifyAndConsumeActionToken(freshTok, "apr-wrong", "dev-1", now); !errors.Is(err, ErrActionTokenBindingFail) {
		t.Fatalf("wrong approvalID must fail with ErrActionTokenBindingFail, got %v", err)
	}
	if _, err := mgr.VerifyAndConsumeActionToken(freshTok, "apr-104", "dev-wrong", now); !errors.Is(err, ErrActionTokenBindingFail) {
		t.Fatalf("wrong deviceID must fail with ErrActionTokenBindingFail, got %v", err)
	}

	// 7. Nonce expiry cleanup: after original tokens expire (> 120s), verifying a new token prunes stale nonces
	later := now.Add(200 * time.Second)
	laterTok, _, err := mgr.IssueActionToken("apr-105", LockscreenDecisionOnce, "dev-1", later)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.VerifyAndConsumeActionToken(laterTok, "apr-105", "dev-1", later.Add(time.Second)); err != nil {
		t.Fatalf("verify later token: %v", err)
	}
	if got := mgr.UsedNonceCount(); got != 1 {
		t.Fatalf("expected expired nonces to be pruned leaving 1 active nonce, got %d", got)
	}
}

func strconvFormatInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}
