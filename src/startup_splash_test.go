// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStartupSplashLifecycleAndAuthorization(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "de"
	splash, err := startStartupSplash(cfg, "test-version")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(splash.Close)

	resp, err := http.Get(splash.URL())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "LocalCode") || !strings.Contains(string(body), "Erneut versuchen") {
		t.Fatalf("startup page status=%d body=%s", resp.StatusCode, body)
	}

	unauthorized, err := http.Get(splash.baseURL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusNotFound {
		t.Fatalf("unauthorized status=%d", unauthorized.StatusCode)
	}

	splash.Update(BootstrapProgress{Percent: 42, Stage: "Prüfung", Detail: "Ollama"})
	state := readStartupState(t, splash)
	if state.Percent != 42 || state.Stage != "Prüfung" || state.Detail != "Ollama" || state.Error != "" {
		t.Fatalf("unexpected progress state: %+v", state)
	}

	splash.Fail(errors.New("download failed"))
	state = readStartupState(t, splash)
	if !strings.Contains(state.Error, "download failed") || state.Done {
		t.Fatalf("unexpected failure state: %+v", state)
	}

	req, _ := http.NewRequest(http.MethodPost, splash.baseURL+"/api/action?token="+splash.token, strings.NewReader(`{"action":"retry"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("action status=%d", resp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if action := splash.WaitAction(ctx); action != "retry" {
		t.Fatalf("action=%q", action)
	}

	splash.Complete("http://127.0.0.1:32145")
	state = readStartupState(t, splash)
	if !state.Done || state.Percent != 100 || state.TargetURL != "http://127.0.0.1:32145" {
		t.Fatalf("unexpected completed state: %+v", state)
	}
}

func readStartupState(t *testing.T, splash *startupSplash) startupSplashState {
	t.Helper()
	resp, err := http.Get(splash.baseURL + "/api/state?token=" + splash.token)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("state status=%d", resp.StatusCode)
	}
	var state startupSplashState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestPullWithProgressDecodesOllamaStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/pull" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = io.WriteString(w, "{\"status\":\"pulling manifest\",\"completed\":0,\"total\":100}\n")
		_, _ = io.WriteString(w, "{\"status\":\"downloading\",\"completed\":50,\"total\":100}\n")
		_, _ = io.WriteString(w, "{\"status\":\"success\",\"completed\":100,\"total\":100}\n")
	}))
	defer server.Close()

	client := NewOllamaClient()
	client.BaseURL = server.URL
	var updates []int64
	err := client.PullWithProgress(context.Background(), "example:latest", func(_ string, completed, _ int64) {
		updates = append(updates, completed)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 3 || updates[0] != 0 || updates[1] != 50 || updates[2] != 100 {
		t.Fatalf("updates=%v", updates)
	}
}

func TestSchemaNineMigrationSeparatesSetupDownloadsFromAgentNetwork(t *testing.T) {
	cfg := normalizeConfig(Config{SchemaVersion: 9, NetworkEnabled: false})
	if cfg.SchemaVersion != 10 {
		t.Fatalf("schema=%d", cfg.SchemaVersion)
	}
	if cfg.NetworkEnabled {
		t.Fatal("agent network preference must remain disabled")
	}
	if !cfg.SetupDownloadsEnabled {
		t.Fatal("automatic setup downloads must be enabled during schema 9 migration")
	}
}

func TestStartupSplashRejectsHostAndInvalidRequests(t *testing.T) {
	cfg := defaultConfig()
	splash, err := startStartupSplash(cfg, `<b>6.4</b>`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(splash.Close)

	req, _ := http.NewRequest(http.MethodGet, splash.URL(), nil)
	req.Host = "example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign Host status=%d", resp.StatusCode)
	}

	resp, err = http.Get(splash.URL())
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	text := string(page)
	if !strings.Contains(text, "&lt;b&gt;6.4&lt;/b&gt;") || strings.Contains(text, "&amp;lt;b&amp;gt;") {
		t.Fatalf("version escaping is incorrect: %s", text)
	}

	badMethod, _ := http.NewRequest(http.MethodPost, splash.baseURL+"/api/state?token="+splash.token, nil)
	resp, err = http.DefaultClient.Do(badMethod)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bad method status=%d", resp.StatusCode)
	}

	badAction, _ := http.NewRequest(http.MethodPost, splash.baseURL+"/api/action?token="+splash.token, strings.NewReader(`{"action":"unknown"}`))
	resp, err = http.DefaultClient.Do(badAction)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad action status=%d", resp.StatusCode)
	}
}

func TestStartupSplashProgressResetAndCancelledWait(t *testing.T) {
	cfg := defaultConfig()
	cfg.Language = "en"
	splash, err := startStartupSplash(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(splash.Close)

	splash.Update(BootstrapProgress{Percent: 120, Stage: " Ready ", Detail: " done "})
	splash.Update(BootstrapProgress{Percent: 20, Stage: "Older"})
	state := readStartupState(t, splash)
	if state.Percent != 100 || state.Stage != "Older" || state.Detail != "" {
		t.Fatalf("state=%+v", state)
	}
	if got := state.String(); !strings.Contains(got, "100%") {
		t.Fatalf("String()=%q", got)
	}

	splash.Fail(errors.New("failure"))
	splash.Update(BootstrapProgress{Percent: -5, Stage: "Recovered"})
	state = readStartupState(t, splash)
	if state.Percent != 0 || state.Error != "" || state.Stage != "Recovered" {
		t.Fatalf("recovered state=%+v", state)
	}
	splash.Fail(errors.New("failure"))
	splash.Reset()
	state = readStartupState(t, splash)
	if state.Percent != 1 || state.Error != "" || !strings.Contains(state.Stage, "Restarting") {
		t.Fatalf("reset state=%+v", state)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if action := splash.WaitAction(ctx); action != "exit" {
		t.Fatalf("cancelled action=%q", action)
	}
}

func TestStartupSplashNilReceiverAndHeaderAuthorization(t *testing.T) {
	var splash *startupSplash
	if splash.URL() != "" {
		t.Fatal("nil splash URL must be empty")
	}
	if splash.authorized(nil) {
		t.Fatal("nil splash must not authorize requests")
	}
	splash.Update(BootstrapProgress{})
	splash.Reset()
	splash.Fail(nil)
	splash.Fail(errors.New("ignored"))
	splash.Complete("http://example.invalid")
	splash.Close()
	if action := splash.WaitAction(context.Background()); action != "exit" {
		t.Fatalf("nil splash action=%q", action)
	}

	cfg := defaultConfig()
	running, err := startStartupSplash(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(running.Close)
	if running.authorized(nil) {
		t.Fatal("nil request must not be authorized")
	}
	wrongPath, err := http.Get(running.baseURL + "/missing?token=" + running.token)
	if err != nil {
		t.Fatal(err)
	}
	_ = wrongPath.Body.Close()
	if wrongPath.StatusCode != http.StatusNotFound {
		t.Fatalf("wrong path status=%d", wrongPath.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, running.baseURL+"/api/state", nil)
	req.Header.Set("X-LocalCode-Startup-Token", running.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("header authorization status=%d", resp.StatusCode)
	}

	invalid, _ := http.NewRequest(http.MethodPost, running.baseURL+"/api/action?token="+running.token, strings.NewReader("{"))
	resp, err = http.DefaultClient.Do(invalid)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid JSON status=%d", resp.StatusCode)
	}
}

func TestStartupSplashActionQueueDoesNotBlock(t *testing.T) {
	splash, err := startStartupSplash(defaultConfig(), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(splash.Close)
	for _, action := range []string{"retry", "continue", "exit"} {
		req, _ := http.NewRequest(http.MethodPost, splash.baseURL+"/api/action?token="+splash.token, strings.NewReader(`{"action":"`+action+`"}`))
		resp, callErr := http.DefaultClient.Do(req)
		if callErr != nil {
			t.Fatal(callErr)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("action %s status=%d", action, resp.StatusCode)
		}
	}
}
