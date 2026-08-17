// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	remoteMDNSServiceType = "_localcode._tcp.local."
	remoteMDNSInstance    = "LocalCode._localcode._tcp.local."
	remoteMDNSHost        = "localcode.local."
)

var remoteTLSMu sync.Mutex

func remoteRequiresTLS(bindHost string) bool {
	host := strings.Trim(strings.ToLower(strings.TrimSpace(bindHost)), "[]")
	if host == "" || host == "localhost" {
		return false
	}
	if host == "0.0.0.0" || host == "::" {
		return true
	}
	ip := net.ParseIP(host)
	return ip == nil || !ip.IsLoopback()
}

func remoteTLSCertificatePaths() (string, string) {
	base := filepath.Join(appDataDir(), "remote-tls")
	return filepath.Join(base, "server.crt"), filepath.Join(base, "server.key")
}

func desiredRemoteTLSIPs(bindHost string) []net.IP {
	seen := map[string]bool{"127.0.0.1": true}
	out := []net.IP{net.ParseIP("127.0.0.1")}
	add := func(value string) {
		ip := net.ParseIP(strings.Trim(strings.TrimSpace(value), "[]"))
		if ip == nil {
			return
		}
		key := ip.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, ip)
		}
	}
	if bindHost != "0.0.0.0" && bindHost != "::" && bindHost != "[::]" {
		add(bindHost)
	}
	for _, value := range activeLANIPv4Addresses() {
		add(value)
	}
	return out
}

func certificateCoversIPs(cert *x509.Certificate, ips []net.IP) bool {
	if cert == nil {
		return false
	}
	for _, wanted := range ips {
		covered := false
		for _, existing := range cert.IPAddresses {
			if existing.Equal(wanted) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func certificateFingerprint(cert *x509.Certificate) string {
	if cert == nil || len(cert.Raw) == 0 {
		return ""
	}
	sum := sha256.Sum256(cert.Raw)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func loadRemoteTLSCertificate(certPath, keyPath string, wantedIPs []net.IP) (tls.Certificate, string, bool) {
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil || len(pair.Certificate) == 0 {
		return tls.Certificate{}, "", false
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || time.Now().Add(14*24*time.Hour).After(leaf.NotAfter) || time.Now().Before(leaf.NotBefore) || !certificateCoversIPs(leaf, wantedIPs) {
		return tls.Certificate{}, "", false
	}
	pair.Leaf = leaf
	return pair, certificateFingerprint(leaf), true
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func ensureRemoteTLSCertificate(bindHost string) (tls.Certificate, string, error) {
	remoteTLSMu.Lock()
	defer remoteTLSMu.Unlock()

	certPath, keyPath := remoteTLSCertificatePaths()
	wantedIPs := desiredRemoteTLSIPs(bindHost)
	if pair, fingerprint, ok := loadRemoteTLSCertificate(certPath, keyPath, wantedIPs); ok {
		return pair, fingerprint, nil
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	serialLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, serialLimit)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "LocalCode Remote"},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.AddDate(2, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:              []string{"localhost", "localcode.local"},
		IPAddresses:           wantedIPs,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writePrivateFile(certPath, certPEM); err != nil {
		return tls.Certificate{}, "", err
	}
	if err := writePrivateFile(keyPath, keyPEM); err != nil {
		return tls.Certificate{}, "", err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	pair.Leaf = leaf
	return pair, certificateFingerprint(leaf), nil
}

func secureRemoteURLsForListener(addr net.Addr, bindHost string) []string {
	_, port, _ := net.SplitHostPort(addr.String())
	if port == "" {
		port = "32146"
	}
	bindHost = strings.TrimSpace(bindHost)
	var hosts []string
	if bindHost == "" || bindHost == "0.0.0.0" || bindHost == "::" || bindHost == "[::]" {
		hosts = activeLANIPv4Addresses()
	} else {
		hosts = append(hosts, strings.Trim(bindHost, "[]"))
	}
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1"}
	}
	urls := make([]string, 0, len(hosts))
	for _, host := range hosts {
		urls = append(urls, "https://"+net.JoinHostPort(host, port)+"/remote")
	}
	return urls
}

func startProductionRemoteServer(state *AppState, cfg Config) ([]string, error) {
	if !cfg.RemoteEnabled {
		return nil, nil
	}
	bindHost := strings.TrimSpace(cfg.RemoteBindHost)
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	if !remoteRequiresTLS(bindHost) {
		return startRemoteHTTPServer(state, cfg)
	}

	port := cfg.RemotePort
	if port <= 0 {
		port = 32146
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, strconv.Itoa(port)))
	if err != nil && port != 0 {
		ln, err = net.Listen("tcp", net.JoinHostPort(bindHost, "0"))
	}
	if err != nil {
		return nil, err
	}
	_, actualPortText, splitErr := net.SplitHostPort(ln.Addr().String())
	if splitErr != nil {
		_ = ln.Close()
		return nil, splitErr
	}
	actualPort, err := strconv.Atoi(actualPortText)
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("invalid remote listen port %q: %w", actualPortText, err)
	}
	if err := ensureRemoteFirewallRule(actualPort); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("LAN Remote firewall rule: %w", err)
	}
	pair, fingerprint, err := ensureRemoteTLSCertificate(bindHost)
	if err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("LAN Remote TLS certificate: %w", err)
	}
	urls := secureRemoteURLsForListener(ln.Addr(), bindHost)
	state.mu.Lock()
	state.RemoteListenAddr = ln.Addr().String()
	state.RemoteURLs = append([]string(nil), urls...)
	state.mu.Unlock()

	remote := NewRemoteServer(state)
	registerRemoteDiscoveryRoute(remote, fingerprint, urls)
	server := &http.Server{
		Handler:           remote,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(os.Stderr, "remote https: ", log.LstdFlags),
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{pair},
		},
	}
	tlsListener := tls.NewListener(ln, server.TLSConfig)
	if err := startRemoteMDNS(actualPort, bindHost, fingerprint, urls); err != nil {
		log.Printf("LocalCode Remote mDNS advertisement unavailable: %v", err)
	}
	go func() {
		if err := server.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
			log.Printf("remote HTTPS server error: %v", err)
		}
	}()
	return urls, nil
}

func registerRemoteDiscoveryRoute(remote *RemoteServer, fingerprint string, urls []string) {
	remote.mux.HandleFunc("/remote/api/discovery", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"app":             "LocalCode Remote",
			"version":         version,
			"service":         remoteMDNSServiceType,
			"tls":             true,
			"tls_fingerprint": fingerprint,
			"remote_urls":     append([]string(nil), urls...),
		})
	})
}

func startRemoteMDNS(port int, bindHost, fingerprint string, urls []string) error {
	if !remoteRequiresTLS(bindHost) {
		return nil
	}
	group := &net.UDPAddr{IP: net.ParseIP("224.0.0.251"), Port: 5353}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return err
	}
	_ = conn.SetReadBuffer(64 << 10)
	ips := desiredRemoteMDNSIPs(bindHost)
	packet := buildRemoteMDNSResponse(port, ips, fingerprint, urls)
	if len(packet) == 0 {
		_ = conn.Close()
		return fmt.Errorf("could not construct mDNS response")
	}
	_, _ = conn.WriteToUDP(packet, group)
	go serveRemoteMDNS(conn, group, packet)
	return nil
}

func serveRemoteMDNS(conn *net.UDPConn, group *net.UDPAddr, packet []byte) {
	defer conn.Close()
	buf := make([]byte, 9000)
	lastAnnouncement := time.Now()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, peer, readErr := conn.ReadFromUDP(buf)
		if readErr == nil && n > 0 && remoteMDNSQueryMatches(buf[:n]) {
			_, _ = conn.WriteToUDP(packet, peer)
		}
		if time.Since(lastAnnouncement) >= time.Minute {
			_, _ = conn.WriteToUDP(packet, group)
			lastAnnouncement = time.Now()
		}
	}
}

func desiredRemoteMDNSIPs(bindHost string) []net.IP {
	var values []string
	if bindHost == "" || bindHost == "0.0.0.0" || bindHost == "::" || bindHost == "[::]" {
		values = activeLANIPv4Addresses()
	} else {
		values = []string{strings.Trim(bindHost, "[]")}
	}
	out := []net.IP{}
	for _, value := range values {
		if ip := net.ParseIP(value).To4(); ip != nil && !ip.IsLoopback() {
			out = append(out, ip)
		}
	}
	return out
}

func dnsAppendName(dst []byte, name string) []byte {
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 {
			return nil
		}
		dst = append(dst, byte(len(label)))
		dst = append(dst, label...)
	}
	return append(dst, 0)
}

func dnsAppendRecord(dst []byte, name string, typ, class uint16, ttl uint32, data []byte) []byte {
	dst = dnsAppendName(dst, name)
	if dst == nil {
		return nil
	}
	var fixed [10]byte
	binary.BigEndian.PutUint16(fixed[0:2], typ)
	binary.BigEndian.PutUint16(fixed[2:4], class)
	binary.BigEndian.PutUint32(fixed[4:8], ttl)
	binary.BigEndian.PutUint16(fixed[8:10], uint16(len(data)))
	dst = append(dst, fixed[:]...)
	return append(dst, data...)
}

func dnsTXT(values ...string) []byte {
	out := []byte{}
	for _, value := range values {
		if len(value) > 255 {
			value = value[:255]
		}
		out = append(out, byte(len(value)))
		out = append(out, value...)
	}
	return out
}

func remotePairingURI(urlValue, fingerprint string) string {
	q := url.Values{}
	q.Set("url", urlValue)
	q.Set("fp", fingerprint)
	return "localcode://pair?" + q.Encode()
}

func buildRemoteMDNSResponse(port int, ips []net.IP, fingerprint string, urls []string) []byte {
	if port <= 0 || port > 65535 {
		return nil
	}
	validIPs := []net.IP{}
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			validIPs = append(validIPs, append(net.IP(nil), v4...))
		}
	}
	sort.Slice(validIPs, func(i, j int) bool { return bytes.Compare(validIPs[i], validIPs[j]) < 0 })
	answerCount := 3 + len(validIPs)
	packet := make([]byte, 12)
	binary.BigEndian.PutUint16(packet[2:4], 0x8400)
	binary.BigEndian.PutUint16(packet[6:8], uint16(answerCount))

	ptr := dnsAppendName(nil, remoteMDNSInstance)
	packet = dnsAppendRecord(packet, remoteMDNSServiceType, 12, 1, 120, ptr)

	srv := make([]byte, 6)
	binary.BigEndian.PutUint16(srv[4:6], uint16(port))
	srv = append(srv, dnsAppendName(nil, remoteMDNSHost)...)
	packet = dnsAppendRecord(packet, remoteMDNSInstance, 33, 0x8001, 120, srv)

	primaryURL := ""
	if len(urls) > 0 {
		primaryURL = urls[0]
	}
	txt := dnsTXT(
		"path=/remote",
		"tls=1",
		"version="+version,
		"fp="+fingerprint,
		"uri="+remotePairingURI(primaryURL, fingerprint),
	)
	packet = dnsAppendRecord(packet, remoteMDNSInstance, 16, 0x8001, 120, txt)
	for _, ip := range validIPs {
		packet = dnsAppendRecord(packet, remoteMDNSHost, 1, 0x8001, 120, []byte(ip.To4()))
	}
	return packet
}

func dnsReadName(packet []byte, offset int) (string, int, bool) {
	labels := []string{}
	start := offset
	jumped := false
	seen := 0
	for offset < len(packet) && seen < 128 {
		seen++
		length := int(packet[offset])
		if length == 0 {
			offset++
			if !jumped {
				start = offset
			}
			return strings.Join(labels, ".") + ".", start, true
		}
		if length&0xc0 == 0xc0 {
			if offset+1 >= len(packet) {
				return "", 0, false
			}
			ptr := int(packet[offset]&0x3f)<<8 | int(packet[offset+1])
			if ptr >= len(packet) {
				return "", 0, false
			}
			if !jumped {
				start = offset + 2
				jumped = true
			}
			offset = ptr
			continue
		}
		offset++
		if length > 63 || offset+length > len(packet) {
			return "", 0, false
		}
		labels = append(labels, string(packet[offset:offset+length]))
		offset += length
		if !jumped {
			start = offset
		}
	}
	return "", 0, false
}

func remoteMDNSQueryMatches(packet []byte) bool {
	if len(packet) < 12 {
		return false
	}
	questions := int(binary.BigEndian.Uint16(packet[4:6]))
	offset := 12
	for i := 0; i < questions; i++ {
		name, next, ok := dnsReadName(packet, offset)
		if !ok || next+4 > len(packet) {
			return false
		}
		lower := strings.ToLower(name)
		if lower == remoteMDNSServiceType || lower == strings.ToLower(remoteMDNSInstance) || lower == remoteMDNSHost || lower == "_services._dns-sd._udp.local." {
			return true
		}
		offset = next + 4
	}
	return false
}
