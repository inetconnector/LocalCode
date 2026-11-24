// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	relayDefaultPollTimeout   = 2 * time.Second
	relayMaxLocalResponseSize = 512 << 10 // safely inside 768 KiB plaintext envelope cap
	relayApproveMaxBodyBytes  = 16 << 10
)

// RelayRPCRequest is the plaintext HTTP-like request carried exclusively inside AES-256-GCM.
type RelayRPCRequest struct {
	RequestID string            `json:"req_id"`
	Method    string            `json:"method"`
	Path      string            `json:"path"`
	Query     string            `json:"query,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      []byte            `json:"body,omitempty"`
}

// RelayRPCResponse is the plaintext HTTP-like response carried exclusively inside AES-256-GCM.
type RelayRPCResponse struct {
	RequestID  string            `json:"req_id"`
	StatusCode int               `json:"status"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       []byte            `json:"body,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// allowedMobileRelayRoutes defines the strict fail-closed allowlist of /remote/api/* routes
// reachable over the E2EE outbound relay. Mission recovery and desktop-only routes are excluded.
var allowedMobileRelayRoutes = map[string]string{
	"/remote/api/ping":                      http.MethodGet,
	"/remote/api/status":                    http.MethodGet,
	"/remote/api/projects":                  http.MethodGet,
	"/remote/api/select-project":            http.MethodPost,
	"/remote/api/threads":                   http.MethodGet,
	"/remote/api/new-chat":                  http.MethodPost,
	"/remote/api/select-chat":               http.MethodPost,
	"/remote/api/snapshot":                  http.MethodGet,
	"/remote/api/chat":                      http.MethodPost,
	"/remote/api/steer":                     http.MethodPost,
	"/remote/api/providers-status":          http.MethodGet,
	"/remote/api/verify-frame":              http.MethodPost,
	"/remote/api/apk-update":                http.MethodGet,
	"/remote/api/apk-update/download":       http.MethodGet,
	"/remote/api/approve":                   http.MethodPost,
	"/remote/api/lockscreen-approve":        http.MethodPost,
	"/remote/api/stop":                      http.MethodPost,
	"/remote/api/unpair":                    http.MethodPost,
	"/remote/api/project-action":            http.MethodPost,
	"/remote/api/project-delete-preview":    http.MethodGet,
	"/remote/api/project-quarantine":        http.MethodGet,
	"/remote/api/project-quarantine-action": http.MethodPost,
	"/remote/api/editing-engine":            http.MethodPost,
}

// ValidateMobileRelayRequest enforces fail-closed mobile authority boundaries before any
// decrypted relay frame may touch the local loopback LocalCode server.
func ValidateMobileRelayRequest(req *RelayRPCRequest) (int, string) {
	if req == nil {
		return http.StatusBadRequest, "empty relay rpc request"
	}
	rawPath := req.Path
	if rawPath == "" || !strings.HasPrefix(rawPath, "/") {
		return http.StatusForbidden, "invalid request path"
	}
	lowerRaw := strings.ToLower(rawPath)
	if strings.Contains(rawPath, "..") ||
		strings.Contains(rawPath, "\\") ||
		strings.ContainsRune(rawPath, '\x00') ||
		strings.Contains(lowerRaw, "%2e") ||
		strings.Contains(lowerRaw, "%2f") ||
		strings.Contains(lowerRaw, "%5c") {
		return http.StatusForbidden, "path traversal forbidden over mobile relay"
	}
	cleaned := path.Clean(rawPath)
	if cleaned != rawPath {
		return http.StatusForbidden, "non-canonical path forbidden over mobile relay"
	}

	// Explicit fail-closed block for mission recovery control endpoints.
	if strings.HasPrefix(lowerRaw, "/api/mission-recovery") ||
		strings.HasPrefix(lowerRaw, "/remote/api/mission-recovery") ||
		strings.Contains(lowerRaw, "mission-recovery") {
		return http.StatusForbidden, "mission recovery endpoints are forbidden over mobile relay"
	}

	// Only /remote/api/* endpoints may ever be accessed through the mobile relay.
	if !strings.HasPrefix(cleaned, "/remote/api/") {
		return http.StatusForbidden, "only /remote/api/* endpoints are permitted over mobile relay"
	}

	allowedMethod, ok := allowedMobileRelayRoutes[cleaned]
	if !ok {
		return http.StatusNotFound, "unknown remote api route"
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method != allowedMethod {
		return http.StatusMethodNotAllowed, "method not allowed for remote api route"
	}

	// Strict fail-closed inspection of /remote/api/approve payloads: NEVER allow global persistence.
	if cleaned == "/remote/api/approve" {
		if len(req.Body) == 0 || len(req.Body) > relayApproveMaxBodyBytes {
			return http.StatusBadRequest, "invalid approval payload size"
		}
		var payload struct {
			ID       string `json:"id"`
			Approve  bool   `json:"approve"`
			Decision string `json:"decision"`
			Scope    string `json:"scope"`
		}
		if err := json.Unmarshal(req.Body, &payload); err != nil {
			return http.StatusBadRequest, "malformed approval json payload"
		}
		decision := strings.ToLower(strings.TrimSpace(payload.Decision))
		scope := strings.ToLower(strings.TrimSpace(payload.Scope))
		if decision == "global" || scope == "global" {
			return http.StatusForbidden, "global approval persistence is forbidden over mobile relay"
		}
		switch decision {
		case "", "once", "reject", "project":
		default:
			return http.StatusBadRequest, "invalid approval decision"
		}
	}

	return http.StatusOK, ""
}

// RelayWorkstationBridgeConfig configures an outbound-only workstation bridge to a zero-knowledge relay.
type RelayWorkstationBridgeConfig struct {
	RelayURL       string
	LocalTargetURL string
	LocalAuthToken string
	RelayAuthToken string
	Cipher         *RelayCipherState
	HTTPClient     *http.Client
	LocalClient    *http.Client
	PollTimeout    time.Duration
}

// RelayWorkstationBridge connects purely outbound from behind NAT/firewalls to the zero-knowledge relay,
// decrypts incoming mobile RPC frames, enforces strict mobile authority limits, proxies to the local
// loopback LocalCode server, and returns AES-256-GCM encrypted responses.
type RelayWorkstationBridge struct {
	relayURL       string
	localTargetURL string
	localAuthToken string
	relayAuthToken string
	cipher         *RelayCipherState
	httpClient     *http.Client
	localClient    *http.Client
	pollTimeout    time.Duration

	rejectedFrames atomic.Int64
	handledFrames  atomic.Int64

	mu     sync.Mutex
	cancel context.CancelFunc
	doneCh chan struct{}
}

// NewRelayWorkstationBridge validates configuration (including strict loopback local target) and creates the bridge.
func NewRelayWorkstationBridge(cfg RelayWorkstationBridgeConfig) (*RelayWorkstationBridge, error) {
	cleanRelay, err := validateRelayEndpointURL(cfg.RelayURL)
	if err != nil {
		return nil, err
	}
	cleanLocal, err := validateLocalLoopbackURL(cfg.LocalTargetURL)
	if err != nil {
		return nil, err
	}
	if cfg.Cipher == nil || cfg.Cipher.Role() != RelayRoleWorkstation {
		return nil, ErrRelayInvalidRole
	}
	if len(cfg.RelayAuthToken) < 32 {
		return nil, errors.New("relay bridge: relay auth token too short")
	}
	pollTimeout := cfg.PollTimeout
	if pollTimeout <= 0 {
		pollTimeout = relayDefaultPollTimeout
	}
	relayClient := cfg.HTTPClient
	if relayClient == nil {
		relayClient = &http.Client{Timeout: pollTimeout + 10*time.Second}
	}
	localClient := cfg.LocalClient
	if localClient == nil {
		localClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &RelayWorkstationBridge{
		relayURL:       cleanRelay,
		localTargetURL: cleanLocal,
		localAuthToken: cfg.LocalAuthToken,
		relayAuthToken: cfg.RelayAuthToken,
		cipher:         cfg.Cipher,
		httpClient:     relayClient,
		localClient:    localClient,
		pollTimeout:    pollTimeout,
	}, nil
}

func validateLocalLoopbackURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return "", errors.New("relay bridge: invalid local target url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", errors.New("relay bridge: unsupported local target scheme")
	}
	if !isRelayLoopbackHost(u.Hostname()) {
		return "", errors.New("relay bridge: local target must be a loopback address")
	}
	return trimmed, nil
}

// RejectedFrames returns the count of dropped frames that failed decryption, replay check, or parsing.
func (b *RelayWorkstationBridge) RejectedFrames() int64 {
	return b.rejectedFrames.Load()
}

// HandledFrames returns the count of authenticated frames processed by the bridge.
func (b *RelayWorkstationBridge) HandledFrames() int64 {
	return b.handledFrames.Load()
}

// Start runs the outbound relay poll loop in a background goroutine.
func (b *RelayWorkstationBridge) Start(parent context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		return errors.New("relay bridge already running")
	}
	ctx, cancel := context.WithCancel(parent)
	doneCh := make(chan struct{})
	b.cancel = cancel
	b.doneCh = doneCh
	go func() {
		defer close(doneCh)
		_ = b.Run(ctx)
	}()
	return nil
}

// Stop cancels the outbound poll loop and waits for clean shutdown.
func (b *RelayWorkstationBridge) Stop() {
	b.mu.Lock()
	cancel := b.cancel
	doneCh := b.doneCh
	b.cancel = nil
	b.doneCh = nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if doneCh != nil {
		<-doneCh
	}
}

// Run executes the outbound long-polling loop until ctx is canceled.
func (b *RelayWorkstationBridge) Run(ctx context.Context) error {
	backoff := 50 * time.Millisecond
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		envelopes, err := pollRelayHTTP(ctx, b.httpClient, b.relayURL, b.cipher.SessionID(), RelayRoleWorkstation, b.relayAuthToken, b.pollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			if backoff < 2*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = 50 * time.Millisecond
		for i := range envelopes {
			b.processEnvelope(ctx, &envelopes[i])
		}
	}
}

func (b *RelayWorkstationBridge) processEnvelope(ctx context.Context, env *RelayEnvelope) {
	plaintext, err := b.cipher.Open(env)
	if err != nil {
		b.rejectedFrames.Add(1)
		return
	}
	var rpcReq RelayRPCRequest
	if err := json.Unmarshal(plaintext, &rpcReq); err != nil {
		b.rejectedFrames.Add(1)
		return
	}
	b.handledFrames.Add(1)

	rpcResp := b.dispatchLocal(ctx, &rpcReq)
	respBytes, err := json.Marshal(rpcResp)
	if err != nil {
		return
	}
	outEnv, err := b.cipher.Seal(respBytes)
	if err != nil {
		return
	}
	_, _ = sendRelayEnvelopeHTTP(ctx, b.httpClient, b.relayURL, b.cipher.SessionID(), RelayRoleWorkstation, b.relayAuthToken, outEnv)
}

func (b *RelayWorkstationBridge) dispatchLocal(ctx context.Context, req *RelayRPCRequest) *RelayRPCResponse {
	if status, msg := ValidateMobileRelayRequest(req); status != http.StatusOK {
		body, _ := json.Marshal(map[string]string{"error": msg})
		return &RelayRPCResponse{
			RequestID:  req.RequestID,
			StatusCode: status,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       body,
			Error:      msg,
		}
	}

	target := b.localTargetURL + req.Path
	if req.Query != "" {
		target += "?" + req.Query
	}
	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, target, bodyReader)
	if err != nil {
		return &RelayRPCResponse{
			RequestID:  req.RequestID,
			StatusCode: http.StatusBadRequest,
			Error:      "failed to construct local request",
		}
	}
	for k, v := range req.Headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		switch lk {
		case "content-type", "accept", "x-localcode-device", "authorization":
			httpReq.Header.Set(k, v)
		}
	}
	if b.localAuthToken != "" && httpReq.Header.Get("Authorization") == "" {
		httpReq.Header.Set("Authorization", "Bearer "+b.localAuthToken)
	}

	resp, err := b.localClient.Do(httpReq)
	if err != nil {
		return &RelayRPCResponse{
			RequestID:  req.RequestID,
			StatusCode: http.StatusBadGateway,
			Error:      "local workstation server unreachable",
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, relayMaxLocalResponseSize+1))
	if err != nil {
		return &RelayRPCResponse{
			RequestID:  req.RequestID,
			StatusCode: http.StatusBadGateway,
			Error:      "failed reading local response",
		}
	}
	if len(respBody) > relayMaxLocalResponseSize {
		return &RelayRPCResponse{
			RequestID:  req.RequestID,
			StatusCode: http.StatusBadGateway,
			Error:      "local response exceeds encrypted frame size limit",
		}
	}

	headers := map[string]string{}
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		headers["Content-Type"] = ct
	}
	return &RelayRPCResponse{
		RequestID:  req.RequestID,
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       respBody,
	}
}

// RelayMobileClient is the mobile endpoint counterpart that communicates over a zero-knowledge relay.
type RelayMobileClient struct {
	relayURL       string
	relayAuthToken string
	cipher         *RelayCipherState
	httpClient     *http.Client
	reqCounter     atomic.Uint64
}

// NewRelayMobileClient creates an E2EE mobile client bound to a paired RelayCipherState.
func NewRelayMobileClient(relayURL, relayAuthToken string, cipherState *RelayCipherState, httpClient *http.Client) (*RelayMobileClient, error) {
	cleanRelay, err := validateRelayEndpointURL(relayURL)
	if err != nil {
		return nil, err
	}
	if cipherState == nil || cipherState.Role() != RelayRoleMobile {
		return nil, ErrRelayInvalidRole
	}
	if len(relayAuthToken) < 32 {
		return nil, errors.New("relay mobile client: relay auth token too short")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &RelayMobileClient{
		relayURL:       cleanRelay,
		relayAuthToken: relayAuthToken,
		cipher:         cipherState,
		httpClient:     httpClient,
	}, nil
}

// SendEnvelope transmits a raw RelayEnvelope to the relay (useful for both normal RPC and negative tests).
func (m *RelayMobileClient) SendEnvelope(ctx context.Context, env *RelayEnvelope) (int, error) {
	return sendRelayEnvelopeHTTP(ctx, m.httpClient, m.relayURL, m.cipher.SessionID(), RelayRoleMobile, m.relayAuthToken, env)
}

// PollEnvelopes polls opaque envelopes queued for the mobile role.
func (m *RelayMobileClient) PollEnvelopes(ctx context.Context, timeout time.Duration) ([]RelayEnvelope, error) {
	return pollRelayHTTP(ctx, m.httpClient, m.relayURL, m.cipher.SessionID(), RelayRoleMobile, m.relayAuthToken, timeout)
}

// DoRPC encrypts req, sends it via the zero-knowledge relay, and waits for the matching encrypted response.
func (m *RelayMobileClient) DoRPC(ctx context.Context, req RelayRPCRequest) (*RelayRPCResponse, error) {
	if req.RequestID == "" {
		req.RequestID = fmt.Sprintf("rpc-%d-%d", time.Now().UTC().UnixNano(), m.reqCounter.Add(1))
	}
	rawReq, err := json.Marshal(&req)
	if err != nil {
		return nil, err
	}
	env, err := m.cipher.Seal(rawReq)
	if err != nil {
		return nil, err
	}
	status, err := m.SendEnvelope(ctx, env)
	if err != nil {
		return nil, err
	}
	if status != http.StatusAccepted {
		return nil, fmt.Errorf("relay send rejected with status %d", status)
	}

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pollTimeout := 2 * time.Second
		if dl, ok := ctx.Deadline(); ok {
			if rem := time.Until(dl); rem > 0 && rem < pollTimeout {
				pollTimeout = rem
			}
		}
		frames, err := m.PollEnvelopes(ctx, pollTimeout)
		if err != nil {
			return nil, err
		}
		for i := range frames {
			plaintext, err := m.cipher.Open(&frames[i])
			if err != nil {
				return nil, err
			}
			var rpcResp RelayRPCResponse
			if err := json.Unmarshal(plaintext, &rpcResp); err != nil {
				return nil, err
			}
			if rpcResp.RequestID == req.RequestID {
				return &rpcResp, nil
			}
		}
	}
}

func sendRelayEnvelopeHTTP(ctx context.Context, client *http.Client, relayURL, sid string, role RelayRole, authToken string, env *RelayEnvelope) (int, error) {
	body, err := json.Marshal(env)
	if err != nil {
		return 0, err
	}
	endpoint := fmt.Sprintf("%s/relay/v1/sessions/%s/send", relayURL, url.PathEscape(sid))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("X-Relay-Proof", SignRelayRequestProof(authToken, sid, role, http.MethodPost, time.Now()))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

func pollRelayHTTP(ctx context.Context, client *http.Client, relayURL, sid string, role RelayRole, authToken string, timeout time.Duration) ([]RelayEnvelope, error) {
	endpoint := fmt.Sprintf("%s/relay/v1/sessions/%s/poll?role=%s&timeout=%s",
		relayURL, url.PathEscape(sid), url.QueryEscape(string(role)), url.QueryEscape(timeout.String()))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("X-Relay-Proof", SignRelayRequestProof(authToken, sid, role, http.MethodGet, time.Now()))

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("relay poll status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	var payload struct {
		Frames []RelayEnvelope `json:"frames"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Frames, nil
}
