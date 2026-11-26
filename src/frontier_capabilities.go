// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// DiffHunkDecision records a user's accept/reject/comment decision on a unified diff hunk.
type DiffHunkDecision struct {
	DiffID    string    `json:"diff_id"`
	HunkKey   string    `json:"hunk_key"`
	Status    string    `json:"status"` // accepted | rejected | pending
	Comment   string    `json:"comment,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Server) handleDiffHunk(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		diffID := strings.TrimSpace(r.URL.Query().Get("diff_id"))
		s.state.mu.RLock()
		out := make([]DiffHunkDecision, 0, len(s.state.DiffHunkDecisions))
		for _, dec := range s.state.DiffHunkDecisions {
			if diffID == "" || dec.DiffID == diffID {
				out = append(out, dec)
			}
		}
		s.state.mu.RUnlock()
		sort.Slice(out, func(i, j int) bool {
			if out[i].DiffID != out[j].DiffID {
				return out[i].DiffID < out[j].DiffID
			}
			return out[i].HunkKey < out[j].HunkKey
		})
		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{
			"ok":        true,
			"decisions": out,
		})
	case http.MethodPost:
		var req struct {
			DiffID      string `json:"diffId"`
			DiffIDSnake string `json:"diff_id"`
			HunkKey     string `json:"hunkKey"`
			HunkKeySnk  string `json:"hunk_key"`
			Status      string `json:"status"`
			Comment     string `json:"comment"`
		}
		if err := readJSON(io.LimitReader(r.Body, 32*1024), &req); err != nil {
			http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
			return
		}
		diffID := strings.TrimSpace(req.DiffID)
		if diffID == "" {
			diffID = strings.TrimSpace(req.DiffIDSnake)
		}
		hunkKey := strings.TrimSpace(req.HunkKey)
		if hunkKey == "" {
			hunkKey = strings.TrimSpace(req.HunkKeySnk)
		}
		status := strings.ToLower(strings.TrimSpace(req.Status))
		if diffID == "" || hunkKey == "" {
			http.Error(w, "diffId and hunkKey are required", http.StatusBadRequest)
			return
		}
		switch status {
		case "accepted", "rejected", "pending":
		default:
			http.Error(w, "status must be accepted, rejected, or pending", http.StatusBadRequest)
			return
		}
		dec := DiffHunkDecision{
			DiffID:    diffID,
			HunkKey:   hunkKey,
			Status:    status,
			Comment:   truncateText(strings.TrimSpace(req.Comment), 4000),
			UpdatedAt: time.Now().UTC(),
		}
		key := diffID + "::" + hunkKey
		s.state.mu.Lock()
		if s.state.DiffHunkDecisions == nil {
			s.state.DiffHunkDecisions = make(map[string]DiffHunkDecision)
		}
		s.state.DiffHunkDecisions[key] = dec
		s.state.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = writeJSON(w, map[string]any{
			"ok":       true,
			"decision": dec,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

var (
	repoMapGoRe = regexp.MustCompile(`(?m)^(?:func\s+(?:\([^)]+\)\s*)?|type\s+)([A-Za-z_][A-Za-z0-9_]*)(?:\s+(?:struct|interface)|\([^)]*\))?`)
	repoMapPyRe = regexp.MustCompile(`(?m)^(?:async\s+def|def|class)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	repoMapRsRe = regexp.MustCompile(`(?m)^(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?(?:fn|struct|enum|trait)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	repoMapJSRe = regexp.MustCompile(`(?m)^(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function|class)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
)

// BuildFrontierRepoMap generates a compact, deterministic multi-language symbol map
// (Go, Python, Rust, JS/TS) ranked by source priority and bounded by maxChars.
func BuildFrontierRepoMap(projectRoot string, maxFiles, maxChars int) string {
	root := strings.TrimSpace(projectRoot)
	if root == "" {
		return ""
	}
	if maxFiles <= 0 {
		maxFiles = 40
	}
	if maxChars <= 0 {
		maxChars = 6000
	}
	files := collectWorkspaceSourceFiles(root, 4)
	if len(files) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("REPO MAP (symbol index):\n")
	count := 0
	for _, rel := range files {
		if count >= maxFiles || b.Len() >= maxChars {
			break
		}
		if strings.Contains(strings.ToLower(rel), "test") {
			continue
		}
		full, err := ensureWithinRoot(root, rel)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(full)
		if err != nil || len(raw) == 0 || len(raw) > 256*1024 || !isProbablyText(raw) {
			continue
		}
		text := string(raw)
		ext := strings.ToLower(filepath.Ext(rel))
		var rx *regexp.Regexp
		switch ext {
		case ".go":
			rx = repoMapGoRe
		case ".py":
			rx = repoMapPyRe
		case ".rs":
			rx = repoMapRsRe
		case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
			rx = repoMapJSRe
		default:
			continue
		}
		matches := rx.FindAllStringSubmatch(text, 12)
		if len(matches) == 0 {
			continue
		}
		seen := map[string]bool{}
		var syms []string
		for _, m := range matches {
			if len(m) >= 2 && m[1] != "" && !seen[m[1]] {
				seen[m[1]] = true
				syms = append(syms, m[1])
			}
		}
		if len(syms) == 0 {
			continue
		}
		line := fmt.Sprintf("- %s: %s\n", rel, strings.Join(syms, ", "))
		if b.Len()+len(line) > maxChars {
			break
		}
		b.WriteString(line)
		count++
	}
	if count == 0 {
		return ""
	}
	return b.String()
}

// RunPostEditDiagnostics executes fast, bounded, injection-free syntax/type checks on a
// freshly edited workspace file and returns concise compiler diagnostics (or "" if clean).
func RunPostEditDiagnostics(ctx context.Context, projectRoot, relPath string) string {
	full, err := ensureWithinRoot(projectRoot, relPath)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(full))
	diagCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch ext {
	case ".py":
		pyExe, err := exec.LookPath("python")
		if err != nil {
			pyExe, err = exec.LookPath("python3")
		}
		if err != nil {
			return ""
		}
		cmd = exec.CommandContext(diagCtx, pyExe, "-m", "py_compile", full)
	case ".js", ".mjs", ".cjs":
		nodeExe, err := exec.LookPath("node")
		if err != nil {
			return ""
		}
		cmd = exec.CommandContext(diagCtx, nodeExe, "--check", full)
	case ".go":
		goExe, err := exec.LookPath("go")
		if err != nil {
			return ""
		}
		cmd = exec.CommandContext(diagCtx, goExe, "vet", "./"+filepath.ToSlash(filepath.Dir(relPath)))
		cmd.Dir = projectRoot
	case ".json":
		raw, err := os.ReadFile(full)
		if err != nil {
			return ""
		}
		var v any
		if jErr := json.Unmarshal(raw, &v); jErr != nil {
			return fmt.Sprintf("POST-EDIT DIAGNOSTIC (%s): JSON syntax error: %v", relPath, jErr)
		}
		return ""
	default:
		return ""
	}

	if cmd.Dir == "" {
		cmd.Dir = projectRoot
	}
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return ""
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = runErr.Error()
	}
	return fmt.Sprintf("POST-EDIT DIAGNOSTIC (%s):\n%s", relPath, truncateText(msg, 2000))
}

// executeAgentPTYAction lets the coding agent directly control persistent PTY sessions
// (start, send input, read buffered output, list sessions, kill) inside workspace boundaries.
func (s *AppState) executeAgentPTYAction(ctx context.Context, project string, a AgentAction) (string, error) {
	mgr := s.EnsurePTYManager()
	switch a.Action {
	case "pty_list":
		sessions := mgr.ListSessions()
		raw, _ := json.MarshalIndent(sessions, "", "  ")
		return string(raw), nil
	case "pty_start":
		cols := a.Width
		if cols <= 0 {
			cols = 100
		}
		rows := a.Height
		if rows <= 0 {
			rows = 28
		}
		subDir := strings.TrimSpace(a.Path)
		if subDir == "" {
			subDir = "."
		}
		sess, err := mgr.CreateSessionInDir(project, subDir, cols, rows)
		if err != nil {
			return "", err
		}
		if cmdText := strings.TrimSpace(a.Command); cmdText != "" {
			if wErr := mgr.Write(sess.ID, []byte(cmdText+"\r\n")); wErr != nil {
				return "", wErr
			}
		}
		raw, _ := json.Marshal(map[string]any{
			"pty_id": sess.ID,
			"cols":   cols,
			"rows":   rows,
			"status": "running",
		})
		return string(raw), nil
	case "pty_send":
		ptyID := strings.TrimSpace(a.Target)
		if ptyID == "" {
			return "", errors.New("pty_send requires target (pty_id)")
		}
		input := a.Text
		if input == "" {
			input = a.Command
		}
		if err := mgr.Write(ptyID, []byte(input)); err != nil {
			return "", err
		}
		return fmt.Sprintf("sent %d bytes to PTY %s", len(input), ptyID), nil
	case "pty_read":
		ptyID := strings.TrimSpace(a.Target)
		if ptyID == "" {
			return "", errors.New("pty_read requires target (pty_id)")
		}
		ch, unsub, err := mgr.ReadEvents(ptyID)
		if err != nil {
			return "", err
		}
		defer unsub()
		var b strings.Builder
		deadline := time.NewTimer(600 * time.Millisecond)
		defer deadline.Stop()
		for {
			select {
			case <-ctx.Done():
				return truncateText(b.String(), 12000), ctx.Err()
			case <-deadline.C:
				if b.Len() == 0 {
					return "(no new PTY output within 600ms)", nil
				}
				return truncateText(b.String(), 12000), nil
			case ev, ok := <-ch:
				if !ok {
					return truncateText(b.String(), 12000), nil
				}
				if ev.Data != "" {
					b.WriteString(ev.Data)
					if b.Len() >= 12000 {
						return truncateText(b.String(), 12000), nil
					}
				}
			}
		}
	case "pty_kill":
		ptyID := strings.TrimSpace(a.Target)
		if ptyID == "" {
			return "", errors.New("pty_kill requires target (pty_id)")
		}
		if err := mgr.Close(ptyID); err != nil {
			return "", err
		}
		return fmt.Sprintf("closed PTY %s", ptyID), nil
	default:
		return "", fmt.Errorf("unknown PTY action %q", a.Action)
	}
}

// executeAgentSpawnSubagent dispatches a governed read-only subagent mission inside the workspace.
func (s *AppState) executeAgentSpawnSubagent(ctx context.Context, project string, a AgentAction) (string, error) {
	taskPrompt := strings.TrimSpace(a.Task)
	if taskPrompt == "" {
		taskPrompt = strings.TrimSpace(a.Message)
	}
	if taskPrompt == "" {
		return "", errors.New("spawn_subagent requires a non-empty task description")
	}
	role := strings.TrimSpace(a.Name)
	if role == "" {
		role = "explorer"
	}
	missionID := "sub-" + newID()
	res, err := s.RunReadOnlyMission(ctx, AgentReadOnlyMissionRequest{
		MissionID: missionID,
		Objective: taskPrompt,
		Project:   project,
		Tasks: []AgentTaskProposal{
			{
				ID:        "task-1",
				Role:      role,
				Objective: taskPrompt,
			},
		},
	})
	if err != nil {
		return "", err
	}
	raw, _ := json.MarshalIndent(map[string]any{
		"mission_id": res.MissionID,
		"state":      res.State,
		"reason":     res.Reason,
		"run":        res.Run,
	}, "", "  ")
	return truncateText(string(raw), 16000), nil
}
