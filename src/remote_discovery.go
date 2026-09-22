// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultUDPDiscoveryPort is the dedicated UDP broadcast port for LocalCode auto-discovery.
	DefaultUDPDiscoveryPort = 32147
)

// UDPDiscoveryPayload represents the discovery announcement sent in response to a broadcast probe.
type UDPDiscoveryPayload struct {
	App             string   `json:"app"`
	InstanceName    string   `json:"instance_name"`
	Hostname        string   `json:"hostname"`
	Version         string   `json:"version"`
	Port            int      `json:"port"`
	URL             string   `json:"url"`
	RemoteURLs      []string `json:"remote_urls"`
	TLS             bool     `json:"tls"`
	TLSFingerprint  string   `json:"tls_fingerprint"`
	ActiveProjects  []string `json:"active_projects"`
	RunningProjects []string `json:"running_projects"`
}

// startRemoteUDPDiscovery starts a UDP broadcast listener on DefaultUDPDiscoveryPort.
// It answers incoming discovery probes with metadata about this LocalCode instance.
func startRemoteUDPDiscovery(remotePort int, bindHost, fingerprint string, urls []string, state *AppState) (func(), error) {
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: DefaultUDPDiscoveryPort,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		// If standard discovery port is occupied (e.g. multiple local instances), try ephemeral port or log
		log.Printf("UDP discovery port %d unavailable: %v", DefaultUDPDiscoveryPort, err)
		return func() {}, err
	}

	_ = conn.SetReadBuffer(64 << 10)

	var (
		stopOnce sync.Once
		stopCh   = make(chan struct{})
	)

	closer := func() {
		stopOnce.Do(func() {
			close(stopCh)
			_ = conn.Close()
		})
	}

	hostname, _ := os.Hostname()
	if strings.TrimSpace(hostname) == "" {
		hostname = "LocalCode-PC"
	}

	primaryURL := ""
	if len(urls) > 0 {
		primaryURL = urls[0]
	}

	go func() {
		buf := make([]byte, 4096)
		for {
			select {
			case <-stopCh:
				return
			default:
			}

			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, peer, readErr := conn.ReadFromUDP(buf)
			if readErr != nil {
				if nErr, ok := readErr.(net.Error); ok && nErr.Timeout() {
					continue
				}
				select {
				case <-stopCh:
					return
				default:
					continue
				}
			}

			if n <= 0 || peer == nil {
				continue
			}

			// Accept any broadcast ping or JSON query
			reqText := strings.TrimSpace(string(buf[:n]))
			if reqText != "" && !strings.Contains(reqText, "discover") && !strings.Contains(reqText, "localcode") && !strings.Contains(reqText, "ping") && !strings.HasPrefix(reqText, "{") {
				// Still respond if non-empty or standard query
			}

			// Collect active project names and running project paths
			var projectNames []string
			var runningProjects []string

			if state != nil {
				state.mu.RLock()
				for _, p := range state.Threads {
					if p != nil && p.Project != "" {
						name := filepath.Base(p.Project)
						if name != "" && name != "." && !containsString(projectNames, name) {
							projectNames = append(projectNames, name)
						}
					}
				}
				runningProjects = state.GetRunningProjectsLocked()
				state.mu.RUnlock()
			}

			if len(projectNames) == 0 && state != nil && state.Project != "" {
				projectNames = append(projectNames, filepath.Base(state.Project))
			}

			payload := UDPDiscoveryPayload{
				App:             "LocalCode Remote",
				InstanceName:    hostname,
				Hostname:        hostname,
				Version:         version,
				Port:            remotePort,
				URL:             primaryURL,
				RemoteURLs:      append([]string(nil), urls...),
				TLS:             true,
				TLSFingerprint:  fingerprint,
				ActiveProjects:  projectNames,
				RunningProjects: runningProjects,
			}

			respData, encErr := json.Marshal(payload)
			if encErr != nil {
				continue
			}

			_, _ = conn.WriteToUDP(respData, peer)
		}
	}()

	return closer, nil
}
