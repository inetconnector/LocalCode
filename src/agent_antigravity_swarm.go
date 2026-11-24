// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SwarmTaskSpec defines one parallel subagent worker in an Antigravity 2.0 swarm wave.
type SwarmTaskSpec struct {
	Name      string
	Role      string
	Wave      int
	Task      string
	Workspace string
	Run       func(ctx context.Context) (summary string, detail string, err error)
}

func (s *AppState) resetSwarmRoster(wave int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.SwarmWave = wave
	s.SwarmAgents = nil
	s.mu.Unlock()
}

func (s *AppState) upsertSwarmAgent(entry SwarmSubagentEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if entry.Wave > s.SwarmWave {
		s.SwarmWave = entry.Wave
	}
	updated := false
	for i := range s.SwarmAgents {
		if s.SwarmAgents[i].ID == entry.ID {
			s.SwarmAgents[i] = entry
			updated = true
			break
		}
	}
	if !updated {
		s.SwarmAgents = append(s.SwarmAgents, entry)
	}
	snapshot := append([]SwarmSubagentEntry(nil), s.SwarmAgents...)
	wave := s.SwarmWave
	project := s.Project
	s.mu.Unlock()
	go syncSwarmRosterToAgentOS(project, wave, snapshot)
}

func (s *AppState) snapshotSwarmRosterLocked() (wave int, active int, agents []SwarmSubagentEntry) {
	if s == nil || len(s.SwarmAgents) == 0 {
		return 0, 0, nil
	}
	agents = make([]SwarmSubagentEntry, len(s.SwarmAgents))
	copy(agents, s.SwarmAgents)
	for _, a := range agents {
		if a.Status == "running" {
			active++
		}
	}
	return s.SwarmWave, active, agents
}

func syncSwarmRosterToAgentOS(project string, wave int, agents []SwarmSubagentEntry) {
	baseURL := strings.TrimSpace(os.Getenv("AGENTOS_URL"))
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8000"
	}
	payload, err := json.Marshal(map[string]any{
		"source":  "localcode",
		"project": project,
		"wave":    wave,
		"agents":  agents,
	})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/v1/swarm/sync", bytes.NewReader(payload))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 900 * time.Millisecond}
	resp, err := client.Do(req)
	if err == nil && resp != nil {
		_ = resp.Body.Close()
	}
}

type swarmWaveResult struct {
	Entry  SwarmSubagentEntry
	Detail string
	Err    error
}

// runParallelSwarmWave executes all provided SwarmTaskSpecs concurrently in
// parallel goroutines (bounded to maxParallel), updates the live Swarm Roster,
// syncs to AgentOS Dashboard, and preserves deterministic input order.
func (s *AppState) runParallelSwarmWave(ctx context.Context, wave int, model string, specs []SwarmTaskSpec, maxParallel int) []swarmWaveResult {
	results := make([]swarmWaveResult, len(specs))
	if len(specs) == 0 {
		return results
	}
	if maxParallel <= 0 || maxParallel > len(specs) {
		maxParallel = len(specs)
	}
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i, spec := range specs {
		i := i
		spec := spec
		if spec.Wave <= 0 {
			spec.Wave = wave
		}
		if spec.Workspace == "" {
			spec.Workspace = "inherit"
		}
		agentID := fmt.Sprintf("lc-w%d-%s-%d", spec.Wave, strings.TrimPrefix(strings.ToLower(spec.Name), "@"), i+1)
		started := time.Now()
		entry := SwarmSubagentEntry{
			ID:        agentID,
			Name:      spec.Name,
			Role:      spec.Role,
			Status:    "running",
			Wave:      spec.Wave,
			Task:      truncateText(spec.Task, 180),
			Workspace: spec.Workspace,
			Model:     model,
			StartedAt: started.Format(time.RFC3339),
		}
		s.upsertSwarmAgent(entry)

		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			activeParallelReadWorkersCount.Add(1)
			defer func() {
				activeParallelReadWorkersCount.Add(-1)
				<-sem
			}()
			summary, detail, err := spec.Run(ctx)
			finished := time.Now()
			entry.CompletedAt = finished.Format(time.RFC3339)
			entry.DurationMs = finished.Sub(started).Milliseconds()
			if err != nil {
				entry.Status = "failed"
				entry.Summary = truncateText(err.Error(), 200)
			} else {
				entry.Status = "completed"
				entry.Summary = truncateText(strings.TrimSpace(summary), 200)
			}
			s.upsertSwarmAgent(entry)
			results[i] = swarmWaveResult{
				Entry:  entry,
				Detail: detail,
				Err:    err,
			}
		}()
	}
	wg.Wait()
	return results
}

// runAutomaticParallelPreflightSwarm implements Google Antigravity 2.0 Wave 1:
// automatically fans out 3 specialized subagents in parallel (@explorer,
// @architect, @qa-planner) with clean isolated scopes and returns a compact
// merged handoff brief for the coordinator.
func (s *AppState) runAutomaticParallelPreflightSwarm(ctx context.Context, project, task, model string, cfg Config) string {
	if s == nil || strings.TrimSpace(project) == "" {
		return ""
	}
	s.resetSwarmRoster(1)
	waveStart := time.Now()

	specs := []SwarmTaskSpec{
		{
			Name:      "@explorer",
			Role:      "explorer",
			Wave:      1,
			Task:      "Map workspace structure, symbols, exports, and DOM contracts",
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				files := collectWorkspaceSourceFiles(project, 3)
				manifest := strings.TrimSpace(buildWorkspaceContractManifest(project))
				repoMap := strings.TrimSpace(BuildFrontierRepoMap(project, 24, 900))
				var d strings.Builder
				d.WriteString(fmt.Sprintf("Source files detected: %d\n", len(files)))
				if len(files) > 0 {
					limit := len(files)
					if limit > 15 {
						limit = 15
					}
					d.WriteString("Key files: " + strings.Join(files[:limit], ", ") + "\n")
				}
				if repoMap != "" {
					d.WriteString("Symbol map:\n" + truncateText(repoMap, 700) + "\n")
				}
				if manifest != "" {
					d.WriteString("Live contract manifest:\n" + truncateText(manifest, 700) + "\n")
				}
				return fmt.Sprintf("%d Quelldateien & Symbol-Graph kartiert", len(files)), d.String(), nil
			},
		},
		{
			Name:      "@architect",
			Role:      "architect",
			Wave:      1,
			Task:      "Analyze task requirements, target modules, and runtime invariants",
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				targets := orderedTaskTargetFiles(project, task)
				intent := classifyTaskIntent(task)
				var d strings.Builder
				d.WriteString(fmt.Sprintf("Intent: %s | Offline/Network=%t | Sandbox=%s\n", intent.Kind, cfg.NetworkEnabled, cfg.SandboxMode))
				if len(targets) > 0 {
					d.WriteString("Target modules: " + strings.Join(targets, ", ") + "\n")
				} else {
					d.WriteString("Target modules: dynamic modular decomposition (HTML/CSS/JS or project-idiomatic layout)\n")
				}
				if !cfg.NetworkEnabled {
					d.WriteString("Constraint: Offline mode — zero external CDN/npm/pip dependencies; pure self-contained modules.\n")
				}
				return fmt.Sprintf("Route %s · %d Zielmodul(e) geplant", intent.Kind, len(targets)), d.String(), nil
			},
		},
		{
			Name:      "@qa-planner",
			Role:      "qa_planner",
			Wave:      1,
			Task:      "Discover local compilers/test runners and verify zero-Jest test plan",
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				runners := []string{}
				for _, toolName := range []string{"node", "python", "go", "cargo", "dotnet"} {
					if info := discoverTool(project, toolName, cfg, false); info.Path != "" {
						runners = append(runners, toolName)
					}
				}
				testFile := findProjectTestFile(project)
				hasJest := false
				if _, err := os.Stat(filepath.Join(project, "node_modules", "jest")); err == nil {
					hasJest = true
				}
				var d strings.Builder
				d.WriteString("Detected local runners: " + strings.Join(runners, ", ") + "\n")
				if testFile != "" {
					d.WriteString("Existing test file: " + testFile + "\n")
				}
				if !hasJest {
					d.WriteString("Node test policy: node_modules/jest is NOT installed — use strictly Node built-in `node:test` + `node:assert/strict` (NEVER jest/vitest/mocha).\n")
				}
				return fmt.Sprintf("%d lokale Runner (%s)", len(runners), strings.Join(runners, ", ")), d.String(), nil
			},
		},
	}

	results := s.runParallelSwarmWave(ctx, 1, model, specs, 3)
	elapsedMs := time.Since(waveStart).Milliseconds()

	var brief strings.Builder
	var cardDetail strings.Builder
	brief.WriteString("\n\n[ANTIGRAVITY 2.0 · PARALLEL SUBAGENT SWARM WAVE 1]\n")
	for _, r := range results {
		statusMark := "✓"
		if r.Err != nil {
			statusMark = "✗"
		}
		cardDetail.WriteString(fmt.Sprintf("%s %s (%s · %dms): %s\n", statusMark, r.Entry.Name, r.Entry.Role, r.Entry.DurationMs, r.Entry.Summary))
		if strings.TrimSpace(r.Detail) != "" {
			brief.WriteString(fmt.Sprintf("### %s (%s)\n%s\n", r.Entry.Name, r.Entry.Role, truncateText(strings.TrimSpace(r.Detail), 650)))
		}
	}

	s.AddEvent(UIEvent{
		Type:    "tool_result",
		Action:  "swarm_wave",
		Message: localizeConfigText(cfg, fmt.Sprintf("Antigravity 2.0 · Wave 1 Parallel-Swarm (%d Subagents · %dms)", len(results), elapsedMs), fmt.Sprintf("Antigravity 2.0 · Wave 1 Parallel Swarm (%d subagents · %dms)", len(results), elapsedMs)),
		Detail:  strings.TrimSpace(cardDetail.String()),
	})

	return truncateText(brief.String(), 2200)
}

// runAutomaticParallelVerifierSwarm implements Google Antigravity 2.0 Wave 3:
// spawns 3 independent adversarial verifier subagents in parallel right before
// finish (@verifier-syntax, @verifier-rule7, @verifier-integrity).
func (s *AppState) runAutomaticParallelVerifierSwarm(ctx context.Context, project, model string, cfg Config, changedPaths map[string]bool) []string {
	if s == nil || strings.TrimSpace(project) == "" || len(changedPaths) == 0 {
		return nil
	}
	paths := make([]string, 0, len(changedPaths))
	for p, ok := range changedPaths {
		if ok && strings.TrimSpace(p) != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)

	waveStart := time.Now()
	var issuesMu sync.Mutex
	var allIssues []string
	addIssue := func(msg string) {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			return
		}
		issuesMu.Lock()
		allIssues = append(allIssues, msg)
		issuesMu.Unlock()
	}

	specs := []SwarmTaskSpec{
		{
			Name:      "@verifier-rule7",
			Role:      "auditor",
			Wave:      3,
			Task:      "Parallel Rule-7 Zero-Stub, Zero-Mock & Offline-Jest Audit",
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				found := 0
				if stubPath, stubHit := detectPendingStubFile(project, changedPaths); stubPath != "" {
					addIssue(fmt.Sprintf("Rule-7 Zero-Stub Verstoß in `%s`: %s.", stubPath, stubHit))
					found++
				}
				for _, rel := range paths {
					full, err := ensureWithinRoot(project, rel)
					if err != nil {
						continue
					}
					data, err := os.ReadFile(full)
					if err != nil {
						continue
					}
					if jestErr := detectJestAPIInNodeTest(project, rel, string(data)); jestErr != "" {
						addIssue(jestErr)
						found++
					}
					if reqErr := detectUnguardedRequireInDualModule(project, rel, string(data)); reqErr != "" {
						addIssue(reqErr)
						found++
					}
				}
				if found == 0 {
					return fmt.Sprintf("%d Dateien 100%% Rule-7 / Zero-Mock sauber", len(paths)), "", nil
				}
				return fmt.Sprintf("%d Rule-7/Jest-Verstöße erkannt", found), "", nil
			},
		},
		{
			Name:      "@verifier-integrity",
			Role:      "verifier",
			Wave:      3,
			Task:      "Parallel Cross-File Contract, DOM-ID & WebApp Integrity Check",
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				webIssues := verifyWebAppIntegrity(ctx, project, cfg, true)
				for _, issue := range webIssues {
					addIssue(issue)
				}
				if len(webIssues) == 0 {
					return "Cross-File & DOM-Integrität bestanden", "", nil
				}
				return fmt.Sprintf("%d Integritätsmängel gefunden", len(webIssues)), strings.Join(webIssues, "\n"), nil
			},
		},
		{
			Name:      "@verifier-syntax",
			Role:      "verifier",
			Wave:      3,
			Task:      "Parallel JS/JSON Syntax Verification across modified files",
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				checked := 0
				bad := 0
				for _, rel := range paths {
					ext := strings.ToLower(filepath.Ext(rel))
					full, err := ensureWithinRoot(project, rel)
					if err != nil {
						continue
					}
					data, err := os.ReadFile(full)
					if err != nil {
						continue
					}
					if ext == ".json" {
						checked++
						var probe any
						if jErr := json.Unmarshal(data, &probe); jErr != nil {
							addIssue(fmt.Sprintf("Ungültiges JSON in `%s`: %v", rel, jErr))
							bad++
						}
					} else if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
						checked++
						if esmErr := detectClassicScriptESMSyntax(project, rel, string(data)); esmErr != "" {
							addIssue(esmErr)
							bad++
						}
					}
				}
				if bad == 0 {
					return fmt.Sprintf("%d Dateien syntaktisch verifiziert", checked), "", nil
				}
				return fmt.Sprintf("%d Syntaxfehler in %d Dateien", bad, checked), "", nil
			},
		},
	}

	results := s.runParallelSwarmWave(ctx, 3, model, specs, 3)
	elapsedMs := time.Since(waveStart).Milliseconds()

	var cardDetail strings.Builder
	for _, r := range results {
		mark := "✓"
		if r.Err != nil || strings.Contains(r.Entry.Summary, "Verstöße") || strings.Contains(r.Entry.Summary, "mängel") || strings.Contains(r.Entry.Summary, "fehler") {
			mark = "⚠"
		}
		cardDetail.WriteString(fmt.Sprintf("%s %s (%s · %dms): %s\n", mark, r.Entry.Name, r.Entry.Role, r.Entry.DurationMs, r.Entry.Summary))
	}

	s.AddEvent(UIEvent{
		Type:    "tool_result",
		Action:  "swarm_wave",
		Message: localizeConfigText(cfg, fmt.Sprintf("Antigravity 2.0 · Wave 3 Adversarial Verifier-Swarm (%d Prüfer · %dms)", len(results), elapsedMs), fmt.Sprintf("Antigravity 2.0 · Wave 3 Adversarial Verifier Swarm (%d verifiers · %dms)", len(results), elapsedMs)),
		Detail:  strings.TrimSpace(cardDetail.String()),
	})

	// Deduplicate issues preserving deterministic order.
	seen := map[string]bool{}
	unique := make([]string, 0, len(allIssues))
	for _, iss := range allIssues {
		if !seen[iss] {
			seen[iss] = true
			unique = append(unique, iss)
		}
	}
	return unique
}

// executeSpawnSubagentsAction executes dynamic coordinator-requested parallel
// read-only subagents (`action: "spawn_subagents"`).
func (s *AppState) executeSpawnSubagentsAction(ctx context.Context, project, model string, cfg Config, action AgentAction) (string, error) {
	tasks := make([]string, 0, len(action.Args)+1)
	for _, arg := range action.Args {
		if trimmed := strings.TrimSpace(arg); trimmed != "" {
			tasks = append(tasks, trimmed)
		}
	}
	if len(tasks) == 0 && strings.TrimSpace(action.Task) != "" {
		for _, part := range strings.Split(action.Task, ";") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				tasks = append(tasks, trimmed)
			}
		}
	}
	if len(tasks) == 0 {
		tasks = []string{strings.TrimSpace(action.Message)}
	}
	if len(tasks) > 4 {
		tasks = tasks[:4]
	}

	specs := make([]SwarmTaskSpec, 0, len(tasks))
	for i, t := range tasks {
		t := t
		role := "researcher"
		lower := strings.ToLower(t)
		switch {
		case strings.Contains(lower, "test") || strings.Contains(lower, "verif") || strings.Contains(lower, "prüf"):
			role = "verifier"
		case strings.Contains(lower, "arch") || strings.Contains(lower, "plan"):
			role = "architect"
		case strings.Contains(lower, "review") || strings.Contains(lower, "audit"):
			role = "reviewer"
		}
		specs = append(specs, SwarmTaskSpec{
			Name:      fmt.Sprintf("@worker-%d", i+1),
			Role:      role,
			Wave:      2,
			Task:      t,
			Workspace: "inherit",
			Run: func(ctx context.Context) (string, string, error) {
				report, err := runReadOnlySubagent(project, cfg, t)
				if err != nil {
					return "", "", err
				}
				return fmt.Sprintf("Analyse abgeschlossen (%d Zeichen)", len(report)), report, nil
			},
		})
	}

	results := s.runParallelSwarmWave(ctx, 2, model, specs, 4)
	var out strings.Builder
	out.WriteString(fmt.Sprintf("PARALLEL SUBAGENT SWARM RESULTS (%d workers)\n\n", len(results)))
	for _, r := range results {
		out.WriteString(fmt.Sprintf("=== %s (%s · %s · %dms) ===\n", r.Entry.Name, r.Entry.Role, r.Entry.Status, r.Entry.DurationMs))
		if r.Err != nil {
			out.WriteString("ERROR: " + r.Err.Error() + "\n\n")
		} else {
			out.WriteString(truncateText(r.Detail, 3500) + "\n\n")
		}
	}
	return out.String(), nil
}
