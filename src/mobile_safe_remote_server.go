// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func newMobileSafeRemoteServer(state *AppState) *RemoteServer {
	remote := NewRemoteServer(state)
	remote.mux.HandleFunc("/remote/api/project-action", remote.withAuth(remote.handleRemoteProjectAction))
	remote.mux.HandleFunc("/remote/api/project-delete-preview", remote.withAuth(remote.handleRemoteProjectDeletePreview))
	return remote
}

func startMobileSafeRemoteHTTPServer(state *AppState, cfg Config) ([]string, error) {
	if !cfg.RemoteEnabled {
		return nil, nil
	}
	port := cfg.RemotePort
	if port <= 0 {
		port = 32146
	}
	bindHost := strings.TrimSpace(cfg.RemoteBindHost)
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(bindHost, fmt.Sprintf("%d", port)))
	if err != nil && port != 0 {
		ln, err = net.Listen("tcp", net.JoinHostPort(bindHost, "0"))
	}
	if err != nil {
		return nil, err
	}
	urls := remoteURLsForListener(ln.Addr(), bindHost)
	state.mu.Lock()
	state.RemoteListenAddr = ln.Addr().String()
	state.RemoteURLs = append([]string(nil), urls...)
	state.mu.Unlock()
	server := &http.Server{
		Handler:           newMobileSafeRemoteServer(state),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          log.New(os.Stderr, "remote http: ", log.LstdFlags),
	}
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("remote HTTP server error: %v", err)
		}
	}()
	return urls, nil
}

func startMobileSafeProductionRemoteServer(state *AppState, cfg Config) ([]string, error) {
	if !cfg.RemoteEnabled {
		return nil, nil
	}
	bindHost := strings.TrimSpace(cfg.RemoteBindHost)
	if bindHost == "" {
		bindHost = "127.0.0.1"
	}
	if !remoteRequiresTLS(bindHost) {
		return startMobileSafeRemoteHTTPServer(state, cfg)
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

	remote := newMobileSafeRemoteServer(state)
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
