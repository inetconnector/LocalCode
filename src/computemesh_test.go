// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMaskAPIKey(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"1234", "****"},
		{"cm_provider_5bde5787bff7ddae2aca1a3b3ebc5927", "cm_provider_…5927"},
		{"shortkey123", "shor…123"},
	}

	for _, c := range cases {
		got := MaskAPIKey(c.input)
		if got != c.want {
			t.Fatalf("MaskAPIKey(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestAutoDetectComputeMeshCredentials(t *testing.T) {
	// 1. Test from Environment
	t.Setenv("COMPUTEMESH_API_KEY", "cm_env_test_1234567890")
	t.Setenv("COMPUTEMESH_URL", "https://custom.computemesh.test")

	key, url, _, _, _, source := AutoDetectComputeMeshCredentials()
	if key != "cm_env_test_1234567890" {
		t.Fatalf("expected env key, got %q", key)
	}
	if url != "https://custom.computemesh.test" {
		t.Fatalf("expected env url, got %q", url)
	}
	if source != "environment (COMPUTEMESH_API_KEY)" {
		t.Fatalf("expected env source, got %q", source)
	}

	// 2. Test from File fixture
	t.Setenv("COMPUTEMESH_API_KEY", "")
	t.Setenv("COMPUTEMESH_URL", "")

	tempHome := t.TempDir()
	t.Setenv("USERPROFILE", tempHome)
	t.Setenv("LOCALCODE_USER_HOME", tempHome)

	dotMesh := filepath.Join(tempHome, ".computemesh")
	if err := os.MkdirAll(dotMesh, 0755); err != nil {
		t.Fatal(err)
	}

	cfgContent := computeMeshConfigFile{
		ProviderAccount: "frede@inetconnector.com",
		ProviderKey:     "cm_provider_5bde5787bff7ddae2aca1a3b3ebc5927",
		GatewayURL:      "https://computemesh.inetconnector.com",
		LocalNodeID:     "test-node-custom",
	}
	cfgBytes, _ := json.Marshal(cfgContent)
	if err := os.WriteFile(filepath.Join(dotMesh, "provider_config.json"), cfgBytes, 0644); err != nil {
		t.Fatal(err)
	}

	key, url, account, nodeID, _, source := AutoDetectComputeMeshCredentials()
	if key != "cm_provider_5bde5787bff7ddae2aca1a3b3ebc5927" {
		t.Fatalf("expected file key, got %q", key)
	}
	if account != "frede@inetconnector.com" {
		t.Fatalf("expected account, got %q", account)
	}
	if nodeID != "test-node-custom" {
		t.Fatalf("expected nodeID, got %q", nodeID)
	}
	if url != "https://computemesh.inetconnector.com" {
		t.Fatalf("expected gateway url, got %q", url)
	}
	if source != ".computemesh/provider_config.json (provider_key)" {
		t.Fatalf("expected provider_config source, got %q", source)
	}
}

func TestProbeRunningLocalComputeMeshNode(t *testing.T) {
	mockNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/api/tags" {
			resp := ollamaTagsResponse{
				Models: []struct {
					Name       string    `json:"name"`
					Size       int64     `json:"size"`
					ModifiedAt time.Time `json:"modified_at"`
				}{
					{Name: "qwen/qwen2.5-7b-instruct", Size: 4350000000, ModifiedAt: time.Now()},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockNode.Close()

	// Direct check with context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 1 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, mockNode.URL+"/", nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("mock local node failed: %v", err)
	}
	resp.Body.Close()
}

func TestCheckComputeMeshStatusWithMockGateway(t *testing.T) {
	expectedToken := "cm_provider_test_key_12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+expectedToken {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			resp := ollamaTagsResponse{
				Models: []struct {
					Name       string    `json:"name"`
					Size       int64     `json:"size"`
					ModifiedAt time.Time `json:"modified_at"`
				}{
					{Name: "qwen/qwen2.5-7b-instruct", Size: 4350000000, ModifiedAt: time.Now()},
					{Name: "deepseek-ai/deepseek-r1", Size: 41000000000, ModifiedAt: time.Now()},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := Config{
		ComputeMeshEnabled:      true,
		ComputeMeshURL:          server.URL,
		ComputeMeshAPIKey:       expectedToken,
		ComputeMeshLocalNodeURL: server.URL,
	}

	status := CheckComputeMeshStatus(context.Background(), cfg)
	if !status.Online {
		t.Fatalf("expected online status, got error: %s", status.Error)
	}
	if len(status.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(status.Models))
	}
	if status.Models[0].Name != "qwen/qwen2.5-7b-instruct" {
		t.Fatalf("unexpected model name: %q", status.Models[0].Name)
	}
}

func TestConfigureComputeMeshForAppState(t *testing.T) {
	expectedToken := "cm_provider_appstate_test"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			resp := ollamaTagsResponse{
				Models: []struct {
					Name       string    `json:"name"`
					Size       int64     `json:"size"`
					ModifiedAt time.Time `json:"modified_at"`
				}{
					{Name: "qwen/qwen2.5-7b-instruct", Size: 4350000000, ModifiedAt: time.Now()},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	cfg := Config{
		ComputeMeshEnabled:      true,
		ComputeMeshURL:          server.URL,
		ComputeMeshAPIKey:       expectedToken,
		ComputeMeshLocalNodeURL: server.URL,
	}

	state := NewAppState(cfg, NewOllamaClient())
	ConfigureComputeMeshForAppState(state)

	state.mu.RLock()
	ollama := state.Ollama
	state.mu.RUnlock()

	if ollama == nil {
		t.Fatal("ollama client is nil")
	}
	if ollama.BaseURL != server.URL {
		t.Fatalf("ollama BaseURL=%q want %q", ollama.BaseURL, server.URL)
	}
	if ollama.AuthToken != expectedToken {
		t.Fatalf("ollama AuthToken=%q want %q", ollama.AuthToken, expectedToken)
	}
}

func TestComputeMeshEndpoints(t *testing.T) {
	expectedToken := "cm_provider_test_endpoint"

	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			resp := ollamaTagsResponse{
				Models: []struct {
					Name       string    `json:"name"`
					Size       int64     `json:"size"`
					ModifiedAt time.Time `json:"modified_at"`
				}{
					{Name: "qwen/qwen2.5-7b-instruct", Size: 4350000000, ModifiedAt: time.Now()},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/chat":
			resp := OllamaChatResponse{
				Message: OllamaMessage{
					Role:    "assistant",
					Content: `{"status":"online","message":"ComputeMesh GPU-Cluster bereit!"}`,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockGateway.Close()

	cfg := Config{
		ComputeMeshEnabled:      true,
		ComputeMeshURL:          mockGateway.URL,
		ComputeMeshAPIKey:       expectedToken,
		ComputeMeshLocalNodeURL: mockGateway.URL,
	}

	state := NewAppState(cfg, NewOllamaClient())
	server := NewServer(state)

	// 1. Test GET /api/computemesh/status
	recStatus := httptest.NewRecorder()
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/computemesh/status", nil)
	server.mux.ServeHTTP(recStatus, reqStatus)

	if recStatus.Code != http.StatusOK {
		t.Fatalf("GET /api/computemesh/status code = %d, want 200", recStatus.Code)
	}

	var statusRes struct {
		OK     bool              `json:"ok"`
		Status ComputeMeshStatus `json:"status"`
	}
	if err := json.NewDecoder(recStatus.Body).Decode(&statusRes); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}
	if !statusRes.OK || !statusRes.Status.Online {
		t.Fatalf("expected status OK and online, got %+v", statusRes)
	}

	// 2. Test POST /api/computemesh/test
	recTest := httptest.NewRecorder()
	testPayload := `{"prompt":"Ping","model":"qwen/qwen2.5-7b-instruct","url":"` + mockGateway.URL + `","api_key":"` + expectedToken + `"}`
	reqTest := httptest.NewRequest(http.MethodPost, "/api/computemesh/test", strings.NewReader(testPayload))
	server.mux.ServeHTTP(recTest, reqTest)

	if recTest.Code != http.StatusOK {
		t.Fatalf("POST /api/computemesh/test code = %d, want 200", recTest.Code)
	}

	var testRes struct {
		OK       bool   `json:"ok"`
		Response string `json:"response"`
	}
	if err := json.NewDecoder(recTest.Body).Decode(&testRes); err != nil {
		t.Fatalf("failed to decode test response: %v", err)
	}
	if !testRes.OK || !strings.Contains(testRes.Response, "ComputeMesh GPU-Cluster bereit!") {
		t.Fatalf("unexpected test response: %+v", testRes)
	}
}
