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

func TestLlamaCppBackend(t *testing.T) {
	expectedToken := "bearer-token-123"

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+expectedToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"qwen2.5-coder:14b","created":1700000000}]}`))
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			resp := map[string]any{
				"choices": []map[string]any{
					{
						"message": map[string]string{
							"role":    "assistant",
							"content": `{"status":"ok","thought":"ready"}`,
						},
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	backend := NewLlamaCppBackend(mockServer.URL, expectedToken)

	if backend.BackendType() != InferenceBackendLlamaCpp {
		t.Fatalf("backend type=%q want %q", backend.BackendType(), InferenceBackendLlamaCpp)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Health
	online, _, err := backend.Health(ctx)
	if err != nil || !online {
		t.Fatalf("Health check failed: %v, online=%v", err, online)
	}

	// 2. Tags
	models, err := backend.Tags(ctx)
	if err != nil || len(models) != 1 {
		t.Fatalf("Tags failed: %v, models=%+v", err, models)
	}
	if models[0].Name != "qwen2.5-coder:14b" {
		t.Fatalf("model name=%q want qwen2.5-coder:14b", models[0].Name)
	}

	// 3. Chat
	reply, err := backend.Chat(ctx, "qwen2.5-coder:14b", []OllamaMessage{{Role: "user", Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if reply != `{"status":"ok","thought":"ready"}` {
		t.Fatalf("unexpected Chat reply: %q", reply)
	}
}
