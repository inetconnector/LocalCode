//go:build windows

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"strings"
	"testing"
	"time"
	"unsafe"
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

func TestWindowsMicaCStringAndDesktopListWindows(t *testing.T) {
	if got := cStringPtrToGoString(0); got != "" {
		t.Fatalf("cStringPtrToGoString(0)=%q", got)
	}
	buf := append([]byte("mica-k0-test-version"), 0)
	if got := cStringPtrToGoString(uintptr(unsafe.Pointer(&buf[0]))); got != "mica-k0-test-version" {
		t.Fatalf("cStringPtrToGoString=%q", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = DesktopListWindows(ctx, defaultConfig())
}
