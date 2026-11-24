// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	LockscreenActionTokenDefaultTTL = 120 * time.Second
	LockscreenActionTokenMaxTTL     = 120 * time.Second
	lockscreenNonceBytes            = 16
	pushApprovalTitle               = "Approval erforderlich"
	pushMaxProjectNameLen           = 48
	pushMaxShortIDLen               = 12
)

// PushProvider selects the push notification transport backend.
type PushProvider string

const (
	PushProviderUnifiedPush PushProvider = "unifiedpush"
	PushProviderFCMv1       PushProvider = "fcm_v1"
	PushProviderAPNs        PushProvider = "apns"
)

// LockscreenActionDecision restricts lockscreen push decisions strictly to once or reject.
type LockscreenActionDecision string

const (
	LockscreenDecisionOnce   LockscreenActionDecision = "once"
	LockscreenDecisionReject LockscreenActionDecision = "reject"
)

var (
	ErrActionDecisionForbidden = errors.New("push action: only 'once' or 'reject' decisions are permitted (global is forbidden)")
	ErrActionTokenInvalid      = errors.New("push action: invalid or tampered lockscreen action token")
	ErrActionTokenExpired      = errors.New("push action: lockscreen action token expired")
	ErrActionTokenReplayed     = errors.New("push action: lockscreen action token already consumed")
	ErrActionTokenBindingFail  = errors.New("push action: token approval_id or device_id binding mismatch")
	ErrPushInvalidTarget       = errors.New("push: invalid push target configuration")
	ErrPushDeliveryFailed      = errors.New("push: upstream push gateway returned error")
)

var (
	safePushIdentifierRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
	safeProjectCharRe    = regexp.MustCompile(`[^A-Za-z0-9 ._+-]+`)
	secretLikePatternRe  = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|ghp_[A-Za-z0-9]{8,}|aiza[0-9a-z_-]{16,}|akia[0-9a-z]{12,}|bearer\s+\S+|-----begin\s+|password\s*=|secret\s*=|token\s*=|@@\s*-\d+)`)
)

// LockscreenActionClaims holds the verified claims of a consumed single-use lockscreen action token.
type LockscreenActionClaims struct {
	ApprovalID string
	Decision   LockscreenActionDecision
	DeviceID   string
	ExpiresAt  time.Time
	Nonce      string
}

// LockscreenActionTokenManager issues and consumes HMAC-SHA256 signed single-use lockscreen action tokens.
type LockscreenActionTokenManager struct {
	signingKey []byte
	ttl        time.Duration
	mu         sync.Mutex
	usedNonces map[string]int64 // nonce -> expiresAt unix seconds
}

// NewLockscreenActionTokenManager initializes a token manager with a >= 32-byte HMAC key and TTL (capped at 120s).
func NewLockscreenActionTokenManager(signingKey []byte, ttl time.Duration) (*LockscreenActionTokenManager, error) {
	if len(signingKey) < 32 {
		return nil, errors.New("push action: signing key must be at least 32 bytes")
	}
	if ttl <= 0 {
		ttl = LockscreenActionTokenDefaultTTL
	}
	if ttl > LockscreenActionTokenMaxTTL {
		return nil, fmt.Errorf("push action: ttl %v exceeds strict maximum %v", ttl, LockscreenActionTokenMaxTTL)
	}
	return &LockscreenActionTokenManager{
		signingKey: append([]byte(nil), signingKey...),
		ttl:        ttl,
		usedNonces: make(map[string]int64),
	}, nil
}

// TTL returns the configured lifetime of issued lockscreen action tokens.
func (m *LockscreenActionTokenManager) TTL() time.Duration {
	return m.ttl
}

// IssueActionToken creates a single-use HMAC-SHA256 signed token over:
// approval_id|decision|device_id|expires_at|nonce
// Strictly permits only "once" or "reject" (NEVER "global").
func (m *LockscreenActionTokenManager) IssueActionToken(approvalID string, decision LockscreenActionDecision, deviceID string, now time.Time) (string, time.Time, error) {
	if err := validateActionDecision(string(decision)); err != nil {
		return "", time.Time{}, err
	}
	approvalID = strings.TrimSpace(approvalID)
	deviceID = strings.TrimSpace(deviceID)
	if !safePushIdentifierRe.MatchString(approvalID) || strings.ContainsRune(approvalID, '|') {
		return "", time.Time{}, errors.New("push action: invalid approval_id")
	}
	if !safePushIdentifierRe.MatchString(deviceID) || strings.ContainsRune(deviceID, '|') {
		return "", time.Time{}, errors.New("push action: invalid device_id")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	nonceBytes := make([]byte, lockscreenNonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonceBytes); err != nil {
		return "", time.Time{}, fmt.Errorf("push action: generate nonce: %w", err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	expiresAt := now.Add(m.ttl).Truncate(time.Second)

	canonical := fmt.Sprintf("%s|%s|%s|%d|%s", approvalID, decision, deviceID, expiresAt.Unix(), nonce)
	mac := computeActionTokenMAC(m.signingKey, canonical)
	token := base64.RawURLEncoding.EncodeToString([]byte(canonical)) + "." + base64.RawURLEncoding.EncodeToString(mac)
	return token, expiresAt, nil
}

// VerifyAndConsumeActionToken verifies the HMAC-SHA256 signature in constant time, enforces strict
// decision allowlist ("once"/"reject", never "global"), checks expiration and optional ID bindings,
// and atomically consumes the nonce to block replays.
func (m *LockscreenActionTokenManager) VerifyAndConsumeActionToken(token, expectedApprovalID, expectedDeviceID string, now time.Time) (*LockscreenActionClaims, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, ErrActionTokenInvalid
	}
	payloadBytes, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return nil, ErrActionTokenInvalid
	}
	providedMAC, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || len(providedMAC) != sha256.Size {
		return nil, ErrActionTokenInvalid
	}

	canonical := string(payloadBytes)
	expectedMAC := computeActionTokenMAC(m.signingKey, canonical)
	if !hmac.Equal(providedMAC, expectedMAC) {
		return nil, ErrActionTokenInvalid
	}

	fields := strings.Split(canonical, "|")
	if len(fields) != 5 {
		return nil, ErrActionTokenInvalid
	}
	approvalID, decisionRaw, deviceID, expRaw, nonce := fields[0], fields[1], fields[2], fields[3], fields[4]

	if err := validateActionDecision(decisionRaw); err != nil {
		return nil, err
	}
	if !safePushIdentifierRe.MatchString(approvalID) || !safePushIdentifierRe.MatchString(deviceID) || len(nonce) < 16 {
		return nil, ErrActionTokenInvalid
	}
	expUnix, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		return nil, ErrActionTokenInvalid
	}
	nowUnix := now.Unix()
	if nowUnix > expUnix {
		return nil, ErrActionTokenExpired
	}
	// Reject tokens minted with a future expiration beyond the maximum allowed TTL (+5s clock tolerance).
	if expUnix > nowUnix+int64(LockscreenActionTokenMaxTTL.Seconds())+5 {
		return nil, ErrActionTokenInvalid
	}

	if expectedApprovalID != "" && subtle.ConstantTimeCompare([]byte(approvalID), []byte(expectedApprovalID)) != 1 {
		return nil, ErrActionTokenBindingFail
	}
	if expectedDeviceID != "" && subtle.ConstantTimeCompare([]byte(deviceID), []byte(expectedDeviceID)) != 1 {
		return nil, ErrActionTokenBindingFail
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for usedNonce, usedExp := range m.usedNonces {
		if usedExp < nowUnix {
			delete(m.usedNonces, usedNonce)
		}
	}
	if _, alreadyUsed := m.usedNonces[nonce]; alreadyUsed {
		return nil, ErrActionTokenReplayed
	}
	m.usedNonces[nonce] = expUnix

	return &LockscreenActionClaims{
		ApprovalID: approvalID,
		Decision:   LockscreenActionDecision(decisionRaw),
		DeviceID:   deviceID,
		ExpiresAt:  time.Unix(expUnix, 0).UTC(),
		Nonce:      nonce,
	}, nil
}

// UsedNonceCount returns the current number of non-expired consumed nonces tracked for replay protection.
func (m *LockscreenActionTokenManager) UsedNonceCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.usedNonces)
}

func validateActionDecision(decision string) error {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case string(LockscreenDecisionOnce), string(LockscreenDecisionReject):
		return nil
	default:
		return ErrActionDecisionForbidden
	}
}

func computeActionTokenMAC(key []byte, canonical string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("localcode-lockscreen-action-v1|"))
	_, _ = mac.Write([]byte(canonical))
	return mac.Sum(nil)
}

// PushTarget describes a registered mobile push destination (UnifiedPush, FCM v1, or APNs).
type PushTarget struct {
	Provider    PushProvider
	DeviceID    string
	Endpoint    string // UnifiedPush URL, or optional override URL for FCM/APNs
	DeviceToken string // FCM registration token or APNs hex device token
	ProjectID   string // FCM project ID (for default FCM v1 URL)
	BearerToken string // FCM OAuth2 access token or APNs provider JWT
	APNsTopic   string // APNs bundle identifier (apns-topic)
	APNsSandbox bool
}

// ApprovalPushEvent carries ONLY minimal non-secret metadata for an approval notification.
// By design this struct contains no source code, diffs, prompts, tool arguments, or secrets.
type ApprovalPushEvent struct {
	ApprovalID  string
	ProjectName string
	Timestamp   time.Time
}

// ZeroSecretPushPayload is the strictly sanitized notification body delivered to push gateways.
type ZeroSecretPushPayload struct {
	Version          int    `json:"v"`
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	Body             string `json:"body"`
	Project          string `json:"project"`
	ApprovalID       string `json:"approval_id"`
	ShortID          string `json:"short_id"`
	Timestamp        string `json:"timestamp"`
	ExpiresAt        string `json:"expires_at"`
	ApproveOnceToken string `json:"approve_once_token"`
	RejectToken      string `json:"reject_token"`
}

// PushNotificationDispatcher delivers zero-secret approval alerts across UnifiedPush, FCM HTTP v1, and APNs.
type PushNotificationDispatcher struct {
	tokens     *LockscreenActionTokenManager
	httpClient *http.Client
	nowFn      func() time.Time
}

// NewPushNotificationDispatcher creates a dispatcher using HTTP/2-capable client transport and action token signing.
func NewPushNotificationDispatcher(tokens *LockscreenActionTokenManager, httpClient *http.Client) (*PushNotificationDispatcher, error) {
	if tokens == nil {
		return nil, errors.New("push: action token manager is required")
	}
	if httpClient == nil {
		tr := &http.Transport{
			ForceAttemptHTTP2:   true,
			MaxIdleConns:        16,
			IdleConnTimeout:     60 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		}
		httpClient = &http.Client{
			Transport: tr,
			Timeout:   15 * time.Second,
		}
	}
	return &PushNotificationDispatcher{
		tokens:     tokens,
		httpClient: httpClient,
		nowFn:      func() time.Time { return time.Now().UTC() },
	}, nil
}

// BuildZeroSecretPayload constructs the minimal zero-secret push payload with signed once/reject action tokens.
func (d *PushNotificationDispatcher) BuildZeroSecretPayload(target PushTarget, ev ApprovalPushEvent) (*ZeroSecretPushPayload, time.Time, error) {
	approvalID := strings.TrimSpace(ev.ApprovalID)
	if !safePushIdentifierRe.MatchString(approvalID) {
		return nil, time.Time{}, errors.New("push: invalid approval_id")
	}
	deviceID := strings.TrimSpace(target.DeviceID)
	if !safePushIdentifierRe.MatchString(deviceID) {
		return nil, time.Time{}, errors.New("push: invalid device_id")
	}

	now := ev.Timestamp.UTC()
	if now.IsZero() {
		now = d.nowFn().UTC()
	}

	onceToken, expOnce, err := d.tokens.IssueActionToken(approvalID, LockscreenDecisionOnce, deviceID, now)
	if err != nil {
		return nil, time.Time{}, err
	}
	rejectToken, _, err := d.tokens.IssueActionToken(approvalID, LockscreenDecisionReject, deviceID, now)
	if err != nil {
		return nil, time.Time{}, err
	}

	project := sanitizePushProjectName(ev.ProjectName)
	shortID := computePushShortID(approvalID)

	payload := &ZeroSecretPushPayload{
		Version:          1,
		Kind:             "approval_required",
		Title:            pushApprovalTitle,
		Body:             fmt.Sprintf("%s · %s (#%s)", pushApprovalTitle, project, shortID),
		Project:          project,
		ApprovalID:       approvalID,
		ShortID:          shortID,
		Timestamp:        now.Format(time.RFC3339),
		ExpiresAt:        expOnce.Format(time.RFC3339),
		ApproveOnceToken: onceToken,
		RejectToken:      rejectToken,
	}
	return payload, expOnce, nil
}

// SendApprovalNotification dispatches a zero-secret approval alert to UnifiedPush, FCM HTTP v1, or APNs.
func (d *PushNotificationDispatcher) SendApprovalNotification(ctx context.Context, target PushTarget, ev ApprovalPushEvent) (*ZeroSecretPushPayload, error) {
	payload, expiresAt, err := d.BuildZeroSecretPayload(target, ev)
	if err != nil {
		return nil, err
	}

	switch target.Provider {
	case PushProviderUnifiedPush:
		err = d.sendUnifiedPush(ctx, target, payload)
	case PushProviderFCMv1:
		err = d.sendFCMv1(ctx, target, payload)
	case PushProviderAPNs:
		err = d.sendAPNs(ctx, target, payload, expiresAt)
	default:
		return nil, fmt.Errorf("%w: unsupported provider %q", ErrPushInvalidTarget, target.Provider)
	}
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (d *PushNotificationDispatcher) sendUnifiedPush(ctx context.Context, target PushTarget, payload *ZeroSecretPushPayload) error {
	endpoint, err := validatePushEndpointURL(target.Endpoint)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	ttlSecs := int(d.tokens.TTL().Seconds())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("TTL", strconv.Itoa(ttlSecs))
	req.Header.Set("Urgency", "high")
	req.Header.Set("Topic", "lc-"+payload.ShortID)
	return d.executePushRequest(req)
}

func (d *PushNotificationDispatcher) sendFCMv1(ctx context.Context, target PushTarget, payload *ZeroSecretPushPayload) error {
	if strings.TrimSpace(target.BearerToken) == "" {
		return fmt.Errorf("%w: fcm oauth2 bearer token required", ErrPushInvalidTarget)
	}
	if strings.TrimSpace(target.DeviceToken) == "" {
		return fmt.Errorf("%w: fcm device token required", ErrPushInvalidTarget)
	}
	endpoint := strings.TrimSpace(target.Endpoint)
	if endpoint == "" {
		if strings.TrimSpace(target.ProjectID) == "" {
			return fmt.Errorf("%w: fcm project_id or endpoint required", ErrPushInvalidTarget)
		}
		endpoint = fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", url.PathEscape(target.ProjectID))
	}
	endpoint, err := validatePushEndpointURL(endpoint)
	if err != nil {
		return err
	}

	ttlStr := fmt.Sprintf("%ds", int(d.tokens.TTL().Seconds()))
	fcmBody := map[string]any{
		"message": map[string]any{
			"token": target.DeviceToken,
			"notification": map[string]string{
				"title": payload.Title,
				"body":  payload.Body,
			},
			"android": map[string]any{
				"priority": "high",
				"ttl":      ttlStr,
			},
			"data": map[string]string{
				"kind":               payload.Kind,
				"project":            payload.Project,
				"approval_id":        payload.ApprovalID,
				"short_id":           payload.ShortID,
				"timestamp":          payload.Timestamp,
				"expires_at":         payload.ExpiresAt,
				"approve_once_token": payload.ApproveOnceToken,
				"reject_token":       payload.RejectToken,
			},
		},
	}
	raw, err := json.Marshal(fcmBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(target.BearerToken))
	return d.executePushRequest(req)
}

func (d *PushNotificationDispatcher) sendAPNs(ctx context.Context, target PushTarget, payload *ZeroSecretPushPayload, expiresAt time.Time) error {
	if strings.TrimSpace(target.BearerToken) == "" {
		return fmt.Errorf("%w: apns bearer jwt required", ErrPushInvalidTarget)
	}
	if strings.TrimSpace(target.APNsTopic) == "" {
		return fmt.Errorf("%w: apns-topic required", ErrPushInvalidTarget)
	}
	if strings.TrimSpace(target.DeviceToken) == "" {
		return fmt.Errorf("%w: apns device token required", ErrPushInvalidTarget)
	}
	endpoint := strings.TrimSpace(target.Endpoint)
	if endpoint == "" {
		host := "https://api.push.apple.com"
		if target.APNsSandbox {
			host = "https://api.sandbox.push.apple.com"
		}
		endpoint = fmt.Sprintf("%s/3/device/%s", host, url.PathEscape(target.DeviceToken))
	}
	endpoint, err := validatePushEndpointURL(endpoint)
	if err != nil {
		return err
	}

	apnsBody := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": payload.Title,
				"body":  payload.Body,
			},
			"category":        "LOCALCODE_APPROVAL",
			"sound":           "default",
			"mutable-content": 1,
		},
		"localcode": map[string]string{
			"kind":               payload.Kind,
			"project":            payload.Project,
			"approval_id":        payload.ApprovalID,
			"short_id":           payload.ShortID,
			"timestamp":          payload.Timestamp,
			"expires_at":         payload.ExpiresAt,
			"approve_once_token": payload.ApproveOnceToken,
			"reject_token":       payload.RejectToken,
		},
	}
	raw, err := json.Marshal(apnsBody)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("authorization", "bearer "+strings.TrimSpace(target.BearerToken))
	req.Header.Set("apns-topic", strings.TrimSpace(target.APNsTopic))
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-expiration", strconv.FormatInt(expiresAt.UTC().Unix(), 10))
	return d.executePushRequest(req)
}

func (d *PushNotificationDispatcher) executePushRequest(req *http.Request) error {
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPushDeliveryFailed, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: status=%d body=%q", ErrPushDeliveryFailed, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func validatePushEndpointURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: invalid push endpoint url", ErrPushInvalidTarget)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return trimmed, nil
	case "http":
		if isRelayLoopbackHost(u.Hostname()) {
			return trimmed, nil
		}
		return "", fmt.Errorf("%w: plaintext http push endpoint only permitted on loopback", ErrPushInvalidTarget)
	default:
		return "", fmt.Errorf("%w: unsupported push url scheme %q", ErrPushInvalidTarget, scheme)
	}
}

// sanitizePushProjectName guarantees Zero-Secret-Push-Policy: strips paths, newlines,
// code/diff fragments, and secret-like strings so push notifications only ever carry a safe label.
func sanitizePushProjectName(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "LocalCode Project"
	}
	// Reject multiline or secret/diff-looking inputs outright.
	if strings.ContainsAny(trimmed, "\r\n\t`{}[]();<>") || secretLikePatternRe.MatchString(trimmed) {
		return "LocalCode Project"
	}
	// Keep only the final path segment if a filesystem path was supplied.
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	base := filepath.Base(normalized)
	if base == "." || base == "/" || base == "" {
		return "LocalCode Project"
	}
	cleaned := strings.TrimSpace(safeProjectCharRe.ReplaceAllString(base, ""))
	if cleaned == "" || secretLikePatternRe.MatchString(cleaned) {
		return "LocalCode Project"
	}
	if len(cleaned) > pushMaxProjectNameLen {
		cleaned = strings.TrimSpace(cleaned[:pushMaxProjectNameLen])
	}
	if cleaned == "" {
		return "LocalCode Project"
	}
	return cleaned
}

func computePushShortID(approvalID string) string {
	cleaned := strings.TrimSpace(approvalID)
	if len(cleaned) <= pushMaxShortIDLen && safePushIdentifierRe.MatchString(cleaned) {
		return cleaned
	}
	sum := sha256.Sum256([]byte(cleaned))
	return fmt.Sprintf("%x", sum[:6])
}
