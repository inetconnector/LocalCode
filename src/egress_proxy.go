// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// EgressProxyConfig configures a per-run loopback HTTP/HTTPS CONNECT proxy
// that enforces a domain allowlist/denylist and SSRF protection.
type EgressProxyConfig struct {
	AllowedDomains []string                                                         `json:"allowed_domains,omitempty"`
	DeniedDomains  []string                                                         `json:"denied_domains,omitempty"`
	DialTimeout    time.Duration                                                    `json:"-"`
	DialContext    func(ctx context.Context, network, addr string) (net.Conn, error) `json:"-"`
	LookupIP       func(ctx context.Context, host string) ([]net.IP, error)          `json:"-"`
}

// EgressAuditEntry records a single allow/deny decision made by EgressProxy.
type EgressAuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Method    string    `json:"method"`
	Host      string    `json:"host"`
	Port      string    `json:"port,omitempty"`
	Allowed   bool      `json:"allowed"`
	Reason    string    `json:"reason"`
	Rule      string    `json:"rule,omitempty"`
}

// EgressProxy is a loopback-bound HTTP and HTTPS CONNECT proxy enforcing
// domain-level egress policies and blocking private/loopback SSRF targets.
type EgressProxy struct {
	cfg      EgressProxyConfig
	listener net.Listener
	server   *http.Server

	mu    sync.Mutex
	audit []EgressAuditEntry
}

// StartEgressProxy binds a new EgressProxy on 127.0.0.1:0 and starts serving.
func StartEgressProxy(cfg EgressProxyConfig) (*EgressProxy, error) {
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = 15 * time.Second
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen egress proxy on loopback: %w", err)
	}
	p := &EgressProxy{
		cfg:      cfg,
		listener: ln,
	}
	p.server = &http.Server{
		Handler:           p,
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		_ = p.server.Serve(ln)
	}()
	return p, nil
}

// Addr returns the host:port address (127.0.0.1:<port>) the proxy listens on.
func (p *EgressProxy) Addr() string {
	if p == nil || p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// URL returns the proxy URL (http://127.0.0.1:<port>) suitable for HTTP_PROXY / HTTPS_PROXY.
func (p *EgressProxy) URL() string {
	addr := p.Addr()
	if addr == "" {
		return ""
	}
	return "http://" + addr
}

// Close shuts down the proxy server and closes the loopback listener.
func (p *EgressProxy) Close() error {
	if p == nil || p.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := p.server.Shutdown(ctx)
	if err != nil {
		_ = p.server.Close()
	}
	return err
}

// AuditLog returns a snapshot of all recorded egress decisions.
func (p *EgressProxy) AuditLog() []EgressAuditEntry {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]EgressAuditEntry, len(p.audit))
	copy(out, p.audit)
	return out
}

// BlockedDomains returns a sorted, deduplicated list of blocked target hosts.
func (p *EgressProxy) BlockedDomains() []string {
	entries := p.AuditLog()
	seen := make(map[string]bool)
	var out []string
	for _, e := range entries {
		if !e.Allowed && e.Host != "" && !seen[e.Host] {
			seen[e.Host] = true
			out = append(out, e.Host)
		}
	}
	sort.Strings(out)
	return out
}

// AllowedDomainsHit returns a sorted, deduplicated list of allowed target hosts.
func (p *EgressProxy) AllowedDomainsHit() []string {
	entries := p.AuditLog()
	seen := make(map[string]bool)
	var out []string
	for _, e := range entries {
		if e.Allowed && e.Host != "" && !seen[e.Host] {
			seen[e.Host] = true
			out = append(out, e.Host)
		}
	}
	sort.Strings(out)
	return out
}

func (p *EgressProxy) recordAudit(entry EgressAuditEntry) {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	p.mu.Lock()
	p.audit = append(p.audit, entry)
	p.mu.Unlock()
}

// CheckDomain evaluates the target host against DeniedDomains (highest priority),
// SSRF IP/localhost rules, and AllowedDomains (default-deny when empty).
func (p *EgressProxy) CheckDomain(rawHost string) (allowed bool, reason string, rule string) {
	host := normalizeEgressHost(rawHost)
	if host == "" {
		return false, "empty target host", "invalid-host"
	}

	// 1. DeniedDomains always takes precedence over AllowedDomains.
	for _, pattern := range p.cfg.DeniedDomains {
		if matchEgressDomainPattern(host, pattern) {
			return false, fmt.Sprintf("domain %q denied by policy rule %q", host, strings.TrimSpace(pattern)), strings.TrimSpace(pattern)
		}
	}

	// 2. Immediate SSRF guard for localhost and private/loopback IP literals.
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false, fmt.Sprintf("SSRF blocked: localhost target %q is forbidden", host), "ssrf-guard"
	}
	if ip := net.ParseIP(host); ip != nil {
		if isEgressForbiddenIP(ip) {
			return false, fmt.Sprintf("SSRF blocked: private or loopback IP %q is forbidden", ip.String()), "ssrf-guard"
		}
	}

	// 3. AllowedDomains allowlist (default-deny when empty).
	if len(p.cfg.AllowedDomains) == 0 {
		return false, fmt.Sprintf("domain %q blocked: no allowed domains configured", host), "default-deny"
	}
	for _, pattern := range p.cfg.AllowedDomains {
		if matchEgressDomainPattern(host, pattern) {
			return true, fmt.Sprintf("domain %q allowed by rule %q", host, strings.TrimSpace(pattern)), strings.TrimSpace(pattern)
		}
	}
	return false, fmt.Sprintf("domain %q is not in allowlist", host), "not-allowlisted"
}

// ServeHTTP dispatches CONNECT (HTTPS tunnel) and plain HTTP proxy requests.
func (p *EgressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.EqualFold(r.Method, http.MethodConnect) {
		p.handleConnect(w, r)
		return
	}
	p.handleHTTP(w, r)
}

func (p *EgressProxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	host, port := splitHostAndDefaultPort(r.Host, "443")
	normHost := normalizeEgressHost(host)

	allowed, reason, rule := p.CheckDomain(normHost)
	if !allowed {
		p.recordAudit(EgressAuditEntry{
			Method:  http.MethodConnect,
			Host:    normHost,
			Port:    port,
			Allowed: false,
			Reason:  reason,
			Rule:    rule,
		})
		writeEgressForbidden(w, normHost, reason, rule)
		return
	}

	resolvedIP, ssrfReason, err := p.resolveAndValidateHost(r.Context(), normHost)
	if err != nil {
		p.recordAudit(EgressAuditEntry{
			Method:  http.MethodConnect,
			Host:    normHost,
			Port:    port,
			Allowed: false,
			Reason:  ssrfReason,
			Rule:    "ssrf-guard",
		})
		writeEgressForbidden(w, normHost, ssrfReason, "ssrf-guard")
		return
	}

	upstream, err := p.dialVerified(r.Context(), "tcp", normHost, resolvedIP, port)
	if err != nil {
		p.recordAudit(EgressAuditEntry{
			Method:  http.MethodConnect,
			Host:    normHost,
			Port:    port,
			Allowed: true,
			Reason:  fmt.Sprintf("%s (upstream dial failed: %v)", reason, err),
			Rule:    rule,
		})
		http.Error(w, "upstream connection failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer upstream.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy hijacking unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, bufRW, err := hj.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()

	p.recordAudit(EgressAuditEntry{
		Method:  http.MethodConnect,
		Host:    normHost,
		Port:    port,
		Allowed: true,
		Reason:  reason,
		Rule:    rule,
	})

	if _, err := io.WriteString(clientConn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}

	// Flush any buffered bytes from the client before bidirectional copy.
	var clientReader io.Reader = clientConn
	if bufRW != nil && bufRW.Reader.Buffered() > 0 {
		clientReader = io.MultiReader(bufRW.Reader, clientConn)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(upstream, clientReader)
		if tcp, ok := upstream.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, upstream)
		if tcp, ok := clientConn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	wg.Wait()
}

func (p *EgressProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	targetHostPort := r.URL.Host
	if strings.TrimSpace(targetHostPort) == "" {
		targetHostPort = r.Host
	}
	defaultPort := "80"
	if strings.EqualFold(r.URL.Scheme, "https") {
		defaultPort = "443"
	}
	host, port := splitHostAndDefaultPort(targetHostPort, defaultPort)
	normHost := normalizeEgressHost(host)

	allowed, reason, rule := p.CheckDomain(normHost)
	if !allowed {
		p.recordAudit(EgressAuditEntry{
			Method:  r.Method,
			Host:    normHost,
			Port:    port,
			Allowed: false,
			Reason:  reason,
			Rule:    rule,
		})
		writeEgressForbidden(w, normHost, reason, rule)
		return
	}

	resolvedIP, ssrfReason, err := p.resolveAndValidateHost(r.Context(), normHost)
	if err != nil {
		p.recordAudit(EgressAuditEntry{
			Method:  r.Method,
			Host:    normHost,
			Port:    port,
			Allowed: false,
			Reason:  ssrfReason,
			Rule:    "ssrf-guard",
		})
		writeEgressForbidden(w, normHost, ssrfReason, "ssrf-guard")
		return
	}

	outURL := *r.URL
	if outURL.Scheme == "" {
		outURL.Scheme = "http"
	}
	if outURL.Host == "" {
		outURL.Host = net.JoinHostPort(normHost, port)
	}
	outReq, err := http.NewRequestWithContext(r.Context(), r.Method, outURL.String(), r.Body)
	if err != nil {
		http.Error(w, "invalid proxy request: "+err.Error(), http.StatusBadRequest)
		return
	}
	copyProxyHeaders(outReq.Header, r.Header)
	stripHopByHopHeaders(outReq.Header)
	outReq.Host = r.Host

	transport := &http.Transport{
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return p.dialVerified(ctx, network, normHost, resolvedIP, port)
		},
	}
	defer transport.CloseIdleConnections()

	resp, err := transport.RoundTrip(outReq)
	if err != nil {
		p.recordAudit(EgressAuditEntry{
			Method:  r.Method,
			Host:    normHost,
			Port:    port,
			Allowed: true,
			Reason:  fmt.Sprintf("%s (upstream error: %v)", reason, err),
			Rule:    rule,
		})
		http.Error(w, "upstream request failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	p.recordAudit(EgressAuditEntry{
		Method:  r.Method,
		Host:    normHost,
		Port:    port,
		Allowed: true,
		Reason:  reason,
		Rule:    rule,
	})

	copyProxyHeaders(w.Header(), resp.Header)
	stripHopByHopHeaders(w.Header())
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (p *EgressProxy) resolveAndValidateHost(ctx context.Context, host string) (net.IP, string, error) {
	if ip := net.ParseIP(host); ip != nil {
		if isEgressForbiddenIP(ip) {
			reason := fmt.Sprintf("SSRF blocked: private or loopback IP %q is forbidden", ip.String())
			return nil, reason, errors.New(reason)
		}
		return ip, "", nil
	}

	var ips []net.IP
	var err error
	if p.cfg.LookupIP != nil {
		ips, err = p.cfg.LookupIP(ctx, host)
	} else {
		addrs, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
		err = lookupErr
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	if err != nil {
		reason := fmt.Sprintf("DNS resolution failed for %q: %v", host, err)
		return nil, reason, err
	}
	if len(ips) == 0 {
		reason := fmt.Sprintf("DNS resolution returned no addresses for %q", host)
		return nil, reason, errors.New(reason)
	}
	for _, ip := range ips {
		if isEgressForbiddenIP(ip) {
			reason := fmt.Sprintf("SSRF blocked: host %q resolved to forbidden private/loopback IP %s", host, ip.String())
			return nil, reason, errors.New(reason)
		}
	}
	return ips[0], "", nil
}

func (p *EgressProxy) dialVerified(ctx context.Context, network, host string, verifiedIP net.IP, port string) (net.Conn, error) {
	targetAddr := net.JoinHostPort(verifiedIP.String(), port)
	if p.cfg.DialContext != nil {
		// Pass the verified IP address to custom dialer (useful for deterministic testing).
		return p.cfg.DialContext(ctx, network, targetAddr)
	}
	dialer := &net.Dialer{
		Timeout:   p.cfg.DialTimeout,
		KeepAlive: 30 * time.Second,
	}
	return dialer.DialContext(ctx, network, targetAddr)
}

// isEgressForbiddenIP builds on web_tools.go's isForbiddenIP and also blocks
// RFC 6598 Carrier-Grade NAT (100.64.0.0/10) and IPv4-mapped IPv6 addresses
// wrapping private/loopback ranges.
func isEgressForbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
		// RFC 6598 CGNAT 100.64.0.0/10
		if v4[0] == 100 && (v4[1]&0xc0) == 64 {
			return true
		}
	}
	return isForbiddenIP(ip)
}

func normalizeEgressHost(raw string) string {
	h := strings.TrimSpace(raw)
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "[") && strings.HasSuffix(h, "]") {
		h = h[1 : len(h)-1]
	} else if parsedHost, _, err := net.SplitHostPort(h); err == nil {
		h = parsedHost
	}
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	h = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(h)), ".")
	return h
}

func splitHostAndDefaultPort(hostPort, defaultPort string) (string, string) {
	trimmed := strings.TrimSpace(hostPort)
	if trimmed == "" {
		return "", defaultPort
	}
	if h, p, err := net.SplitHostPort(trimmed); err == nil {
		if p == "" {
			p = defaultPort
		}
		return h, p
	}
	return trimmed, defaultPort
}

func matchEgressDomainPattern(host, pattern string) bool {
	h := normalizeEgressHost(host)
	p := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(pattern)), ".")
	if h == "" || p == "" {
		return false
	}
	if p == "*" {
		return true
	}
	if strings.HasPrefix(p, "*.") {
		base := strings.TrimPrefix(p, "*.")
		if base == "" {
			return false
		}
		return strings.HasSuffix(h, "."+base) && len(h) > len(base)+1
	}
	return h == p
}

func writeEgressForbidden(w http.ResponseWriter, host, reason, rule string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-LocalCode-Proxy-Reason", reason)
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":  "egress_blocked",
		"host":   host,
		"reason": reason,
		"rule":   rule,
	})
}

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func copyProxyHeaders(dst, src http.Header) {
	for k, vals := range src {
		for _, v := range vals {
			dst.Add(k, v)
		}
	}
}

func stripHopByHopHeaders(h http.Header) {
	if connHeader := h.Get("Connection"); connHeader != "" {
		for _, field := range strings.Split(connHeader, ",") {
			if f := strings.TrimSpace(field); f != "" {
				h.Del(f)
			}
		}
	}
	for _, key := range hopByHopHeaders {
		h.Del(key)
	}
}
