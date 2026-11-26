//go:build windows

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"
)

func TestWindowsPipeFallbackPTY(t *testing.T) {
	p, err := startWindowsPipeFallback(t.TempDir(), 80, 24)
	if err != nil {
		t.Fatalf("startWindowsPipeFallback: %v", err)
	}
	defer func() { _ = p.Close() }()

	if p.BackendName() != "pipe-fallback" {
		t.Fatalf("BackendName=%q", p.BackendName())
	}
	if err := p.Resize(100, 30); err != nil {
		t.Fatalf("Resize err=%v", err)
	}
	if _, err := p.Write([]byte("echo pipe_ok_marker\r\nexit 0\r\n")); err != nil {
		t.Fatalf("Write err=%v", err)
	}

	buf := make([]byte, 4096)
	var out strings.Builder
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, rerr := p.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
		}
		if rerr != nil {
			break
		}
	}
	code, _ := p.Wait()
	if code != 0 || !strings.Contains(out.String(), "pipe_ok_marker") {
		t.Fatalf("pipe fallback code=%d out=%q", code, out.String())
	}
}
