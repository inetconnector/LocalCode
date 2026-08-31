// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunDoctorDiagnostics(t *testing.T) {
	cfg := defaultConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	report := RunDoctorDiagnostics(ctx, cfg)

	if len(report.Items) == 0 {
		t.Fatal("expected diagnostic items in doctor report, got 0")
	}

	if report.OS == "" || report.Arch == "" {
		t.Fatalf("missing OS/Arch in report: %+v", report)
	}

	foundComputeMesh := false
	foundOllama := false
	foundGit := false

	for _, item := range report.Items {
		if item.Name == "ComputeMesh Decentralized Cluster" {
			foundComputeMesh = true
		}
		if item.Name == "Local Ollama Engine" {
			foundOllama = true
		}
		if item.Name == "Git & Isolated Worktrees" {
			foundGit = true
		}
	}

	if !foundComputeMesh {
		t.Fatal("missing ComputeMesh diagnostic item")
	}
	if !foundOllama {
		t.Fatal("missing Ollama diagnostic item")
	}
	if !foundGit {
		t.Fatal("missing Git diagnostic item")
	}
}

func TestDoctorEndpoint(t *testing.T) {
	cfg := defaultConfig()
	state := NewAppState(cfg, NewOllamaClient())
	server := NewServer(state)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/doctor", nil)
	server.mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/doctor code = %d, want 200", rec.Code)
	}

	var report DoctorReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("failed to decode DoctorReport: %v", err)
	}

	if len(report.Items) == 0 {
		t.Fatalf("expected items in doctor response, got %+v", report)
	}
}
