// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalOllamaPacmanCloneSmoke(t *testing.T) {
	if os.Getenv("LOCALCODE_RUN_LOCAL_MODEL_PACMAN_TEST") != "1" {
		t.Skip("set LOCALCODE_RUN_LOCAL_MODEL_PACMAN_TEST=1 to run the local Ollama Pac-Man generation smoke")
	}
	t.Setenv("LOCALCODE_CONFIG_HOME", t.TempDir())
	t.Setenv("LOCALCODE_CACHE_HOME", t.TempDir())
	t.Setenv("LOCALCODE_USER_HOME", t.TempDir())

	model := strings.TrimSpace(os.Getenv("LOCALCODE_PACMAN_MODEL"))
	if model == "" {
		model = defaultCodingModel
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	ollama := NewOllamaClient()
	ollama.BaseURL = "http://127.0.0.1:11434"
	if err := ollama.Discover(ctx); err != nil {
		t.Fatalf("Ollama discovery failed: %v", err)
	}
	models, err := ollama.Tags(ctx)
	if err != nil {
		t.Fatalf("Ollama model inventory failed: %v", err)
	}
	found := false
	for _, candidate := range models {
		if candidate.Name == model {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("required local model %q is not installed; available models: %#v", model, models)
	}

	root := t.TempDir()
	project := filepath.Join(root, "pacman-smoke")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.RootProjectDir = root
	cfg.LastProject = project
	cfg.LastModel = model
	cfg.OllamaDefaultModel = model
	cfg.EditingEngine = editingEngineNative
	cfg.ApprovalMode = "dangerous"
	cfg.SandboxMode = "project"
	cfg.NetworkEnabled = false
	cfg.WebSearchProvider = "disabled"
	cfg.MaxAgentSteps = 80
	cfg.CommandTimeout = 120
	cfg.ModelTimeout = 360
	cfg.AutoStateUpdate = false
	cfg.CreateProjectDocs = false
	cfg.HookBeforeTask = ""
	cfg.HookAfterTask = ""
	cfg.HookBeforeTool = ""
	cfg.HookAfterTool = ""
	state := NewAppState(cfg, ollama)
	defer state.Close()

	task := strings.Join([]string{
		"Erstelle einen vollständigen Pac-Man-Klon als einzelne Datei index.html im aktuellen Projekt.",
		"Die Datei muss ein spielbares HTML5-Canvas-Spiel enthalten: Maze/Wände, Pac-Man, Pellets, mindestens zwei Gegner, Score, Lives, Start/Win/Game-over-Zustand, Reset/Restart und Tastatursteuerung.",
		"Keine README-only Lösung und keine Platzhalter. Prüfe danach mindestens, dass index.html existiert und die zentrale Spiellogik enthalten ist.",
	}, " ")
	if err := state.StartAgent(task, model, nil); err != nil {
		t.Fatalf("StartAgent failed: %v", err)
	}

	deadline := time.Now().Add(18 * time.Minute)
	for {
		state.mu.RLock()
		running := state.Running
		pending := state.Pending != nil
		summary := state.LastSummary
		events := append([]UIEvent(nil), state.Events...)
		state.mu.RUnlock()
		if pending {
			t.Fatalf("unexpected approval prompt in dangerous-mode smoke:\n%s", summarizeEventsForPacmanSmoke(events))
		}
		if !running {
			if strings.TrimSpace(summary) == "" {
				t.Fatalf("agent stopped without final summary:\n%s", summarizeEventsForPacmanSmoke(events))
			}
			break
		}
		if time.Now().After(deadline) {
			_ = state.StopAgent()
			t.Fatalf("Pac-Man smoke timed out:\n%s", summarizeEventsForPacmanSmoke(events))
		}
		time.Sleep(2 * time.Second)
	}

	data, err := os.ReadFile(filepath.Join(project, "index.html"))
	if err != nil {
		t.Fatalf("index.html was not created: %v\n%s", err, summarizeEventsForPacmanSmoke(snapshotEventsForPacmanSmoke(state)))
	}
	content := strings.ToLower(string(data))
	for _, marker := range []string{"<canvas", "pac", "ghost", "pellet", "score", "lives", "restart"} {
		if !strings.Contains(content, marker) {
			t.Fatalf("index.html is missing marker %q:\n%s\n\nEvents:\n%s", marker, truncateText(string(data), 4000), summarizeEventsForPacmanSmoke(snapshotEventsForPacmanSmoke(state)))
		}
	}
}

func snapshotEventsForPacmanSmoke(state *AppState) []UIEvent {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return append([]UIEvent(nil), state.Events...)
}

func summarizeEventsForPacmanSmoke(events []UIEvent) string {
	if len(events) > 20 {
		events = events[len(events)-20:]
	}
	var lines []string
	for _, event := range events {
		line := strings.TrimSpace(event.Type + " " + event.Action + " " + event.Message)
		if event.Detail != "" {
			line += ": " + truncateText(strings.ReplaceAll(event.Detail, "\n", " "), 500)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
