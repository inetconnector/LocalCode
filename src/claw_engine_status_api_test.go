// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClawStatusAPIIsAdditiveAndSelectable(t *testing.T) {
	project := t.TempDir()
	cfg := defaultConfig()
	cfg.RootProjectDir = project
	cfg.LastProject = project
	cfg.EditingEngine = editingEngineOpenCode
	state := &AppState{Config: cfg, Project: project, Model: "qwen2.5-coder:14b", Ollama: NewOllamaClient()}
	server := NewServer(state)

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/engines/status?engine=claw", nil)
	req.Host = "127.0.0.1"
	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status HTTP %d: %s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Selected     string               `json:"selected"`
		Status       CodingEngineStatus   `json:"status"`
		Engines      []CodingEngineStatus `json:"engines"`
		Experimental []CodingEngineStatus `json:"experimental_engines"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Selected != editingEngineClaw || payload.Status.Engine != editingEngineClaw {
		t.Fatalf("Claw was not selectable: %#v", payload)
	}
	if len(payload.Engines) != 4 {
		t.Fatalf("established engine API contract changed: %#v", payload.Engines)
	}
	if len(payload.Experimental) != 1 || payload.Experimental[0].Engine != editingEngineClaw || payload.Experimental[0].DisplayName != "Claw Code" {
		t.Fatalf("experimental Claw status missing: %#v", payload.Experimental)
	}
}
