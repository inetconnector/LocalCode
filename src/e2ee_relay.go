// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	relayProtocolVersion      = 1
	relayDomainSeparation     = "localcode-e2ee-relay-v1"
	relayKeyBytes             = 32
	relayNonceBytes           = 12
	relayAuthSecretBytes      = 32
	relayReplayWindowSize     = 128
	relayMaxPlaintextBytes    = 768 << 10 // fits cleanly under 1 MiB base64 JSON envelope
	relayProofMaxClockSkewSec = 60
)

// RelayRole identifies which endpoint owns a cipher state or relay mailbox.
type RelayRole string

const (
	RelayRoleWorkstation RelayRole = "workstation"
	RelayRoleMobile      RelayRole = "mobile"
)

const (
	relayDirWorkstationToMobile = "workstation->mobile"
	relayDirMobileToWorkstation = "mobile->workstation"
)

var (
	ErrRelayInvalidKey       = errors.New("relay: invalid X25519 key")
	ErrRelayInvalidSessionID = errors.New("relay: invalid session id")
	ErrRelayInvalidRole      = errors.New("relay: invalid role")
	ErrRelayDecryptFailed    = errors.New("relay: authenticated decryption failed")
	ErrRelayReplayDetected   = errors.New("relay: replay or stale sequence detected")
	ErrRelaySeqExhausted     = errors.New("relay: sequence counter exhausted")
	ErrRelayInvalidEnvelope  = errors.New("relay: invalid encrypted envelope")
	ErrRelayInvalidPairing   = errors.New("relay: invalid pairing payload")
	ErrRelayFingerprintFail  = errors.New("relay: pairing fingerprint mismatch")
	ErrRelayPlaintextTooBig  = errors.New("relay: plaintext exceeds maximum frame size")
)

var relaySessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// RelayKeyPair holds an X25519 keypair for one endpoint of an E2EE relay session.
type RelayKeyPair struct {
	Private *ecdh.PrivateKey
	Public  *ecdh.PublicKey
}

// GenerateRelayKeyPair creates a fresh X25519 keypair using crypto/rand.
func GenerateRelayKeyPair() (*RelayKeyPair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate X25519 keypair: %w", err)
	}
	return &RelayKeyPair{
		Private: priv,
		Public:  priv.PublicKey(),
	}, nil
}

// ParseRelayPublicKey parses a raw 32-byte X25519 public key and rejects all-zero keys.
func ParseRelayPublicKey(raw []byte) (*ecdh.PublicKey, error) {
	if len(raw) != relayKeyBytes {
		return nil, ErrRelayInvalidKey
	}
	allZero := byte(0)
	for _, b := range raw {
		allZero |= b
	}
	if allZero == 0 {
		return nil, ErrRelayInvalidKey
	}
	pub, err := ecdh.X25519().NewPublicKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayInvalidKey, err)
	}
	return pub, nil
}

// GenerateRelaySessionID creates a 192-bit URL-safe random session identifier.
func GenerateRelaySessionID() (string, error) {
	buf := make([]byte, 24)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// GenerateRelayAuthSecret creates a 256-bit shared authentication secret for relay HTTP auth.
func GenerateRelayAuthSecret() ([]byte, error) {
	buf := make([]byte, relayAuthSecretBytes)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return nil, fmt.Errorf("generate relay auth secret: %w", err)
	}
	return buf, nil
}

// ValidateRelaySessionID checks that a session ID conforms to strict safe syntax.
func ValidateRelaySessionID(sessionID string) error {
	if !relaySessionIDPattern.MatchString(sessionID) {
		return ErrRelayInvalidSessionID
	}
	return nil
}

// HKDFExtractSHA256 implements RFC 5869 Section 2.2 (HKDF-Extract with SHA-256).
func HKDFExtractSHA256(salt, ikm []byte) []byte {
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write(ikm)
	return mac.Sum(nil)
}

// HKDFExpandSHA256 implements RFC 5869 Section 2.3 (HKDF-Expand with SHA-256).
func HKDFExpandSHA256(prk, info []byte, length int) ([]byte, error) {
	if len(prk) < sha256.Size {
		return nil, errors.New("hkdf: PRK shorter than SHA-256 hash length")
	}
	if length <= 0 || length > 255*sha256.Size {
		return nil, errors.New("hkdf: invalid output length")
	}
	n := (length + sha256.Size - 1) / sha256.Size
	okm := make([]byte, 0, n*sha256.Size)
	var prev []byte
	for i := 1; i <= n; i++ {
		mac := hmac.New(sha256.New, prk)
		if len(prev) > 0 {
			_, _ = mac.Write(prev)
		}
		_, _ = mac.Write(info)
		_, _ = mac.Write([]byte{byte(i)})
		prev = mac.Sum(nil)
		okm = append(okm, prev...)
	}
	return okm[:length], nil
}

// DeriveHKDFSHA256 performs RFC 5869 HKDF-Extract followed by HKDF-Expand.
func DeriveHKDFSHA256(ikm, salt, info []byte, length int) ([]byte, error) {
	prk := HKDFExtractSHA256(salt, ikm)
	return HKDFExpandSHA256(prk, info, length)
}

// RelaySessionKeys holds the derived directional AES-256-GCM keys and session safety fingerprint.
type RelaySessionKeys struct {
	SessionID              string
	WorkstationToMobileKey []byte
	MobileToWorkstationKey []byte
	SafetyFingerprint      string
}

// DeriveRelaySessionKeys performs X25519 ECDH + HKDF-SHA256 with domain separation
// ("localcode-e2ee-relay-v1") to derive distinct directional AES-256 keys for
// workstation->mobile and mobile->workstation traffic.
func DeriveRelaySessionKeys(localPriv *ecdh.PrivateKey, peerPub *ecdh.PublicKey, sessionID string, localRole RelayRole) (*RelaySessionKeys, error) {
	if localPriv == nil || peerPub == nil {
		return nil, ErrRelayInvalidKey
	}
	if err := ValidateRelaySessionID(sessionID); err != nil {
		return nil, err
	}
	if localRole != RelayRoleWorkstation && localRole != RelayRoleMobile {
		return nil, ErrRelayInvalidRole
	}

	sharedSecret, err := localPriv.ECDH(peerPub)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayInvalidKey, err)
	}
	allZero := byte(0)
	for _, b := range sharedSecret {
		allZero |= b
	}
	if allZero == 0 {
		return nil, ErrRelayInvalidKey
	}

	var workstationPub, mobilePub []byte
	if localRole == RelayRoleWorkstation {
		workstationPub = localPriv.PublicKey().Bytes()
		mobilePub = peerPub.Bytes()
	} else {
		workstationPub = peerPub.Bytes()
		mobilePub = localPriv.PublicKey().Bytes()
	}

	// Transcript-bound salt ensures both endpoints agree on session ID and exact endpoint roles.
	saltHash := sha256.New()
	_, _ = saltHash.Write([]byte(relayDomainSeparation))
	_, _ = saltHash.Write([]byte("|salt|"))
	_, _ = saltHash.Write([]byte(sessionID))
	_, _ = saltHash.Write([]byte("|"))
	_, _ = saltHash.Write(workstationPub)
	_, _ = saltHash.Write([]byte("|"))
	_, _ = saltHash.Write(mobilePub)
	salt := saltHash.Sum(nil)

	prk := HKDFExtractSHA256(salt, sharedSecret)
	w2mInfo := []byte(relayDomainSeparation + "|" + relayDirWorkstationToMobile + "|" + sessionID)
	m2wInfo := []byte(relayDomainSeparation + "|" + relayDirMobileToWorkstation + "|" + sessionID)

	w2mKey, err := HKDFExpandSHA256(prk, w2mInfo, relayKeyBytes)
	if err != nil {
		return nil, err
	}
	m2wKey, err := HKDFExpandSHA256(prk, m2wInfo, relayKeyBytes)
	if err != nil {
		return nil, err
	}

	return &RelaySessionKeys{
		SessionID:              sessionID,
		WorkstationToMobileKey: w2mKey,
		MobileToWorkstationKey: m2wKey,
		SafetyFingerprint:      ComputeRelaySessionFingerprint(sessionID, workstationPub, mobilePub),
	}, nil
}

// ComputeRelaySessionFingerprint returns a 64-char hex SHA-256 safety fingerprint binding both X25519 keys.
func ComputeRelaySessionFingerprint(sessionID string, workstationPub, mobilePub []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(relayDomainSeparation + "|session-fp|"))
	_, _ = h.Write([]byte(sessionID))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write(workstationPub)
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write(mobilePub)
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeRelayPairingFingerprint computes a cryptographic SHA-256 fingerprint over the pairing parameters.
func ComputeRelayPairingFingerprint(relayURL, sessionID string, pubBytes, authSecret []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(relayDomainSeparation + "|pair-fp|"))
	_, _ = h.Write([]byte(strings.TrimRight(strings.TrimSpace(relayURL), "/")))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(sessionID))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write(pubBytes)
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write(authSecret)
	return hex.EncodeToString(h.Sum(nil))
}

// ReplayWindow is a 128-frame sliding anti-replay window.
// Callers MUST call Check before decryption and Commit only after AEAD verification succeeds.
type ReplayWindow struct {
	maxSeq uint64
	bitsLo uint64 // offsets [0..63] below maxSeq
	bitsHi uint64 // offsets [64..127] below maxSeq
}

// NewReplayWindow returns a fresh empty sliding replay window.
func NewReplayWindow() *ReplayWindow {
	return &ReplayWindow{}
}

// Check verifies that seq has not been seen and is not too old without mutating state.
func (w *ReplayWindow) Check(seq uint64) error {
	if seq == 0 {
		return ErrRelayReplayDetected
	}
	if w.maxSeq == 0 || seq > w.maxSeq {
		return nil
	}
	diff := w.maxSeq - seq
	if diff >= relayReplayWindowSize {
		return ErrRelayReplayDetected
	}
	if diff < 64 {
		if (w.bitsLo & (uint64(1) << diff)) != 0 {
			return ErrRelayReplayDetected
		}
		return nil
	}
	if (w.bitsHi & (uint64(1) << (diff - 64))) != 0 {
		return ErrRelayReplayDetected
	}
	return nil
}

// Commit marks seq as accepted after authenticated decryption has succeeded.
func (w *ReplayWindow) Commit(seq uint64) {
	if seq == 0 {
		return
	}
	if w.maxSeq == 0 {
		w.maxSeq = seq
		w.bitsLo = 1
		w.bitsHi = 0
		return
	}
	if seq > w.maxSeq {
		shift := seq - w.maxSeq
		switch {
		case shift >= relayReplayWindowSize:
			w.bitsLo = 1
			w.bitsHi = 0
		case shift >= 64:
			w.bitsHi = w.bitsLo << (shift - 64)
			w.bitsLo = 1
		default:
			w.bitsHi = (w.bitsHi << shift) | (w.bitsLo >> (64 - shift))
			w.bitsLo = (w.bitsLo << shift) | 1
		}
		w.maxSeq = seq
		return
	}
	diff := w.maxSeq - seq
	if diff >= relayReplayWindowSize {
		return
	}
	if diff < 64 {
		w.bitsLo |= uint64(1) << diff
	} else {
		w.bitsHi |= uint64(1) << (diff - 64)
	}
}

// CheckAndAccept atomically validates and marks seq as seen.
func (w *ReplayWindow) CheckAndAccept(seq uint64) error {
	if err := w.Check(seq); err != nil {
		return err
	}
	w.Commit(seq)
	return nil
}

// RelayEnvelope is the opaque encrypted wire frame forwarded by the zero-knowledge relay.
type RelayEnvelope struct {
	Version    int    `json:"v"`
	SessionID  string `json:"sid"`
	SenderRole string `json:"from"`
	Direction  string `json:"dir"`
	Seq        uint64 `json:"seq"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ct"`
	Timestamp  int64  `json:"ts"`
}

// RelayCipherState manages directional AES-256-GCM encryption/decryption with monotonic nonces and replay defense.
type RelayCipherState struct {
	mu          sync.Mutex
	sessionID   string
	role        RelayRole
	sendDir     string
	recvDir     string
	sendAEAD    cipher.AEAD
	recvAEAD    cipher.AEAD
	sendSeq     uint64
	recvWindow  *ReplayWindow
	fingerprint string
}

// NewRelayCipherState initializes AES-256-GCM directional ciphers for the given role.
func NewRelayCipherState(keys *RelaySessionKeys, role RelayRole) (*RelayCipherState, error) {
	if keys == nil {
		return nil, ErrRelayInvalidKey
	}
	if err := ValidateRelaySessionID(keys.SessionID); err != nil {
		return nil, err
	}
	var sendKey, recvKey []byte
	var sendDir, recvDir string
	switch role {
	case RelayRoleWorkstation:
		sendKey = keys.WorkstationToMobileKey
		recvKey = keys.MobileToWorkstationKey
		sendDir = relayDirWorkstationToMobile
		recvDir = relayDirMobileToWorkstation
	case RelayRoleMobile:
		sendKey = keys.MobileToWorkstationKey
		recvKey = keys.WorkstationToMobileKey
		sendDir = relayDirMobileToWorkstation
		recvDir = relayDirWorkstationToMobile
	default:
		return nil, ErrRelayInvalidRole
	}
	sendAEAD, err := newAES256GCM(sendKey)
	if err != nil {
		return nil, err
	}
	recvAEAD, err := newAES256GCM(recvKey)
	if err != nil {
		return nil, err
	}
	return &RelayCipherState{
		sessionID:   keys.SessionID,
		role:        role,
		sendDir:     sendDir,
		recvDir:     recvDir,
		sendAEAD:    sendAEAD,
		recvAEAD:    recvAEAD,
		recvWindow:  NewReplayWindow(),
		fingerprint: keys.SafetyFingerprint,
	}, nil
}

func newAES256GCM(key []byte) (cipher.AEAD, error) {
	if len(key) != relayKeyBytes {
		return nil, ErrRelayInvalidKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func buildRelayNonce(direction string, seq uint64) [relayNonceBytes]byte {
	var nonce [relayNonceBytes]byte
	if direction == relayDirWorkstationToMobile {
		nonce[0], nonce[1], nonce[2], nonce[3] = 0x01, 'W', '>', 'M'
	} else {
		nonce[0], nonce[1], nonce[2], nonce[3] = 0x02, 'M', '>', 'W'
	}
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce
}

func buildRelayAAD(sessionID, direction string, seq uint64) []byte {
	return []byte(fmt.Sprintf("%s|aad|%d:%s|%s|%d", relayDomainSeparation, len(sessionID), sessionID, direction, seq))
}

// SessionID returns the session identifier bound to this cipher state.
func (c *RelayCipherState) SessionID() string { return c.sessionID }

// Role returns the endpoint role bound to this cipher state.
func (c *RelayCipherState) Role() RelayRole { return c.role }

// SafetyFingerprint returns the SHA-256 fingerprint binding both X25519 keys.
func (c *RelayCipherState) SafetyFingerprint() string { return c.fingerprint }

// Seal encrypts plaintext with AES-256-GCM using a strictly monotonic 64-bit sequence counter
// and associated data binding (SessionID + Direction + Seq).
func (c *RelayCipherState) Seal(plaintext []byte) (*RelayEnvelope, error) {
	if len(plaintext) > relayMaxPlaintextBytes {
		return nil, ErrRelayPlaintextTooBig
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sendSeq == math.MaxUint64 {
		return nil, ErrRelaySeqExhausted
	}
	c.sendSeq++
	seq := c.sendSeq
	nonce := buildRelayNonce(c.sendDir, seq)
	aad := buildRelayAAD(c.sessionID, c.sendDir, seq)
	ct := c.sendAEAD.Seal(nil, nonce[:], plaintext, aad)

	return &RelayEnvelope{
		Version:    relayProtocolVersion,
		SessionID:  c.sessionID,
		SenderRole: string(c.role),
		Direction:  c.sendDir,
		Seq:        seq,
		Nonce:      base64.RawURLEncoding.EncodeToString(nonce[:]),
		Ciphertext: base64.RawURLEncoding.EncodeToString(ct),
		Timestamp:  time.Now().UTC().Unix(),
	}, nil
}

// Open authenticates and decrypts an incoming RelayEnvelope, enforcing direction,
// deterministic nonce, associated data, and the sliding anti-replay window.
func (c *RelayCipherState) Open(env *RelayEnvelope) ([]byte, error) {
	if env == nil {
		return nil, ErrRelayInvalidEnvelope
	}
	if env.Version != relayProtocolVersion {
		return nil, ErrRelayInvalidEnvelope
	}
	if subtle.ConstantTimeCompare([]byte(env.SessionID), []byte(c.sessionID)) != 1 {
		return nil, ErrRelayInvalidEnvelope
	}
	if env.Direction != c.recvDir {
		return nil, ErrRelayInvalidEnvelope
	}
	expectedSender := string(RelayRoleMobile)
	if c.role == RelayRoleMobile {
		expectedSender = string(RelayRoleWorkstation)
	}
	if env.SenderRole != expectedSender {
		return nil, ErrRelayInvalidEnvelope
	}
	if env.Seq == 0 {
		return nil, ErrRelayReplayDetected
	}

	nonceBytes, err := base64.RawURLEncoding.Strict().DecodeString(env.Nonce)
	if err != nil || len(nonceBytes) != relayNonceBytes {
		return nil, ErrRelayInvalidEnvelope
	}
	expectedNonce := buildRelayNonce(c.recvDir, env.Seq)
	if subtle.ConstantTimeCompare(nonceBytes, expectedNonce[:]) != 1 {
		return nil, ErrRelayInvalidEnvelope
	}
	ctBytes, err := base64.RawURLEncoding.Strict().DecodeString(env.Ciphertext)
	if err != nil || len(ctBytes) < c.recvAEAD.Overhead() {
		return nil, ErrRelayInvalidEnvelope
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check replay window BEFORE decryption without advancing state on unauthenticated input.
	if err := c.recvWindow.Check(env.Seq); err != nil {
		return nil, err
	}

	aad := buildRelayAAD(c.sessionID, c.recvDir, env.Seq)
	plaintext, err := c.recvAEAD.Open(nil, expectedNonce[:], ctBytes, aad)
	if err != nil {
		return nil, ErrRelayDecryptFailed
	}

	// Commit sequence ONLY after AES-256-GCM tag verification succeeds.
	c.recvWindow.Commit(env.Seq)
	return plaintext, nil
}

// RelayPairingPayload represents the parsed contents of a localcode://relay-pair URI.
type RelayPairingPayload struct {
	RelayURL      string
	SessionID     string
	PeerPublicKey *ecdh.PublicKey
	PeerPublicRaw []byte
	AuthSecret    []byte
	Fingerprint   string
}

// VerifyFingerprint checks expectedFP against the payload's cryptographic fingerprint in constant time.
func (p *RelayPairingPayload) VerifyFingerprint(expectedFP string) bool {
	if p == nil {
		return false
	}
	computed := ComputeRelayPairingFingerprint(p.RelayURL, p.SessionID, p.PeerPublicRaw, p.AuthSecret)
	a := strings.ToLower(strings.TrimSpace(computed))
	b := strings.ToLower(strings.TrimSpace(expectedFP))
	return len(a) == sha256.Size*2 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// GenerateRelayPairingURI creates a localcode://relay-pair URI with cryptographic fingerprint.
func GenerateRelayPairingURI(relayURL, sessionID string, workstationPub *ecdh.PublicKey, authSecret []byte) (string, string, error) {
	cleanURL, err := validateRelayEndpointURL(relayURL)
	if err != nil {
		return "", "", err
	}
	if err := ValidateRelaySessionID(sessionID); err != nil {
		return "", "", err
	}
	if workstationPub == nil || len(workstationPub.Bytes()) != relayKeyBytes {
		return "", "", ErrRelayInvalidKey
	}
	if len(authSecret) < 16 {
		return "", "", fmt.Errorf("%w: auth secret too short", ErrRelayInvalidPairing)
	}

	pubBytes := workstationPub.Bytes()
	fp := ComputeRelayPairingFingerprint(cleanURL, sessionID, pubBytes, authSecret)
	q := url.Values{}
	q.Set("relay", cleanURL)
	q.Set("sid", sessionID)
	q.Set("pub", base64.RawURLEncoding.EncodeToString(pubBytes))
	q.Set("auth", base64.RawURLEncoding.EncodeToString(authSecret))
	q.Set("fp", fp)

	u := &url.URL{
		Scheme:   "localcode",
		Host:     "relay-pair",
		RawQuery: q.Encode(),
	}
	return u.String(), fp, nil
}

// ParseRelayPairingURI parses and cryptographically validates a localcode://relay-pair URI.
func ParseRelayPairingURI(rawURI string) (*RelayPairingPayload, error) {
	u, err := url.Parse(strings.TrimSpace(rawURI))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayInvalidPairing, err)
	}
	if !strings.EqualFold(u.Scheme, "localcode") || !strings.EqualFold(u.Host, "relay-pair") {
		return nil, fmt.Errorf("%w: invalid scheme or host", ErrRelayInvalidPairing)
	}
	q := u.Query()
	cleanURL, err := validateRelayEndpointURL(q.Get("relay"))
	if err != nil {
		return nil, err
	}
	sid := strings.TrimSpace(q.Get("sid"))
	if err := ValidateRelaySessionID(sid); err != nil {
		return nil, err
	}
	pubRaw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(q.Get("pub")))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid pub encoding", ErrRelayInvalidPairing)
	}
	pubKey, err := ParseRelayPublicKey(pubRaw)
	if err != nil {
		return nil, err
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(q.Get("auth")))
	if err != nil || len(authSecret) < 16 {
		return nil, fmt.Errorf("%w: invalid auth secret", ErrRelayInvalidPairing)
	}

	expectedFP := ComputeRelayPairingFingerprint(cleanURL, sid, pubRaw, authSecret)
	if providedFP := strings.TrimSpace(q.Get("fp")); providedFP != "" {
		if subtle.ConstantTimeCompare([]byte(strings.ToLower(providedFP)), []byte(expectedFP)) != 1 {
			return nil, ErrRelayFingerprintFail
		}
	}

	return &RelayPairingPayload{
		RelayURL:      cleanURL,
		SessionID:     sid,
		PeerPublicKey: pubKey,
		PeerPublicRaw: append([]byte(nil), pubRaw...),
		AuthSecret:    append([]byte(nil), authSecret...),
		Fingerprint:   expectedFP,
	}, nil
}

func validateRelayEndpointURL(raw string) (string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("%w: invalid relay url", ErrRelayInvalidPairing)
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		return trimmed, nil
	case "http":
		if isRelayLoopbackHost(u.Hostname()) {
			return trimmed, nil
		}
		return "", fmt.Errorf("%w: plaintext http relay only permitted on loopback", ErrRelayInvalidPairing)
	default:
		return "", fmt.Errorf("%w: unsupported relay url scheme %q", ErrRelayInvalidPairing, scheme)
	}
}

func isRelayLoopbackHost(host string) bool {
	h := strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// DeriveRelayAuthToken derives the 256-bit relay HTTP session auth token from authSecret and sessionID.
// This token authenticates transport requests to the relay and is completely separate from E2EE keys.
func DeriveRelayAuthToken(authSecret []byte, sessionID string) (string, error) {
	if len(authSecret) < 16 {
		return "", fmt.Errorf("%w: auth secret too short", ErrRelayInvalidPairing)
	}
	if err := ValidateRelaySessionID(sessionID); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, authSecret)
	_, _ = mac.Write([]byte(relayDomainSeparation + "|http-auth|" + sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// SignRelayRequestProof produces a timestamped HMAC-SHA256 proof binding sessionID, role, HTTP method, and timestamp.
func SignRelayRequestProof(relayAuthToken, sessionID string, role RelayRole, method string, now time.Time) string {
	ts := strconv.FormatInt(now.UTC().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(relayAuthToken))
	_, _ = mac.Write([]byte(relayDomainSeparation + "|req|" + sessionID + "|" + string(role) + "|" + strings.ToUpper(method) + "|" + ts))
	return ts + "." + hex.EncodeToString(mac.Sum(nil))
}

// VerifyRelayRequestProof verifies a timestamped HMAC-SHA256 request proof in constant time.
func VerifyRelayRequestProof(relayAuthToken, sessionID string, role RelayRole, method, proof string, now time.Time) bool {
	if relayAuthToken == "" || proof == "" {
		return false
	}
	parts := strings.Split(strings.TrimSpace(proof), ".")
	if len(parts) != 2 {
		return false
	}
	ts, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	skew := now.UTC().Unix() - ts
	if skew < -relayProofMaxClockSkewSec || skew > relayProofMaxClockSkewSec {
		return false
	}
	providedMAC, err := hex.DecodeString(parts[1])
	if err != nil || len(providedMAC) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(relayAuthToken))
	_, _ = mac.Write([]byte(relayDomainSeparation + "|req|" + sessionID + "|" + string(role) + "|" + strings.ToUpper(method) + "|" + parts[0]))
	expectedMAC := mac.Sum(nil)
	return hmac.Equal(providedMAC, expectedMAC)
}
