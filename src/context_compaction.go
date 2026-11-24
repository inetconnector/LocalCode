// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// compactedContext is deliberately factual. It is working state for the next
// model call, not a user-facing answer and never contains hidden reasoning.
type compactedContext struct {
	Summary         string   `json:"summary"`
	OriginalTask    string   `json:"original_task"`
	Focus           string   `json:"focus,omitempty"`
	Decisions       []string `json:"decisions"`
	ProjectFacts    []string `json:"project_facts"`
	FilesRead       []string `json:"files_read"`
	FilesChanged    []string `json:"files_changed"`
	Commands        []string `json:"commands"`
	Failures        []string `json:"failures"`
	OpenItems       []string `json:"open_items"`
	NextRecommended string   `json:"next_recommended_action"`
}

var compactionSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary":                 map[string]any{"type": "string"},
		"original_task":           map[string]any{"type": "string"},
		"decisions":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"project_facts":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"files_read":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"files_changed":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"commands":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"failures":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"open_items":              map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"next_recommended_action": map[string]any{"type": "string"},
	},
	"required":             []string{"summary", "original_task", "decisions", "project_facts", "files_read", "files_changed", "commands", "failures", "open_items", "next_recommended_action"},
	"additionalProperties": false,
}

func estimateMessageTokens(messages []OllamaMessage) int {
	chars := 0
	for _, message := range messages {
		chars += utf8.RuneCountInString(message.Role) + utf8.RuneCountInString(message.Content) + utf8.RuneCountInString(message.Thinking) + 12
		for _, image := range message.Images {
			// Image payloads are not normally present in the coding loop, but reserve
			// a bounded estimate if a caller includes them.
			chars += minInt(len(image)/8, 4096)
		}
	}
	// Four characters per token is a conservative language-agnostic estimate
	// for mixed German, source code, JSON and command output.
	return chars/4 + len(messages)*4
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isLocalOllamaURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	if u == "" {
		return true
	}
	return isLocalHostURL(u) && strings.Contains(u, ":11434")
}

func effectiveCompactionContextLength(cfg Config) int {
	if isLocalOllamaURL(cfg.OllamaURL) {
		return computeDynamicLocalContext(cfg, detectLocalVRAMMB())
	}
	contextLength := cfg.ContextLength
	if contextLength < 4096 {
		contextLength = 32768
	}
	return contextLength
}

func shouldCompactMessages(messages []OllamaMessage, cfg Config) bool {
	if !cfg.ContextCompactionEnabled || cfg.ContextLength < 4096 {
		return false
	}
	effectiveCtx := effectiveCompactionContextLength(cfg)
	threshold := effectiveCtx * cfg.ContextCompactionThresholdPercent / 100
	if threshold <= 0 {
		threshold = effectiveCtx * 68 / 100
	}
	return estimateMessageTokens(messages) >= threshold
}

func postCompactionTokenTarget(cfg Config) int {
	contextLength := effectiveCompactionContextLength(cfg)
	target := contextLength * 55 / 100
	if target < 2048 {
		target = 2048
	}
	return target
}

func contextToolResultLimit(cfg Config) int {
	contextLength := cfg.ContextLength
	if contextLength < 4096 {
		contextLength = 32768
	}
	limit := contextLength * 2
	if limit < 8000 {
		limit = 8000
	}
	if limit > 60000 {
		limit = 60000
	}
	return limit
}

func renderCompactedState(state compactedContext) string {
	var b strings.Builder
	b.WriteString("KOMPRIMIERTER ARBEITSKONTEXT\n")
	b.WriteString("Dieser Zustand ersetzt ältere Gesprächsteile. Behandle ihn als verbindliche Arbeitsakte. Er enthält keine Gedankenkette.\n\n")
	writeField := func(title, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		b.WriteString(title + ":\n" + value + "\n\n")
	}
	writeList := func(title string, values []string) {
		if len(values) == 0 {
			return
		}
		b.WriteString(title + ":\n")
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				b.WriteString("- " + value + "\n")
			}
		}
		b.WriteByte('\n')
	}
	writeField("Ursprüngliche Aufgabe", state.OriginalTask)
	writeField("Fokus der Komprimierung", state.Focus)
	writeField("Zusammenfassung", state.Summary)
	writeList("Entscheidungen und Nutzerfreigaben", state.Decisions)
	writeList("Projektfakten", state.ProjectFacts)
	writeList("Gelesene Dateien", state.FilesRead)
	writeList("Geänderte Dateien", state.FilesChanged)
	writeList("Befehle und Prüfungen", state.Commands)
	writeList("Fehler und Diagnosen", state.Failures)
	writeList("Offene Punkte", state.OpenItems)
	writeField("Nächste empfohlene Aktion", state.NextRecommended)
	return strings.TrimSpace(b.String())
}

func deterministicContextSummary(messages []OllamaMessage, originalTask string) compactedContext {
	return deterministicContextSummaryWithFocus(messages, originalTask, "")
}

func deterministicContextSummaryWithFocus(messages []OllamaMessage, originalTask, focusInstruction string) compactedContext {
	focus := strings.TrimSpace(focusInstruction)
	state := compactedContext{
		OriginalTask: truncateText(strings.TrimSpace(originalTask), 2400),
		Focus:        truncateText(focus, 600),
	}
	var focusKeywords []string
	if focus != "" {
		for _, tok := range strings.Fields(strings.ToLower(focus)) {
			tok = strings.Trim(tok, ".,;:!?\"'()[]{}`")
			if len(tok) >= 3 {
				focusKeywords = append(focusKeywords, tok)
			}
		}
	}
	matchesFocus := func(text string) bool {
		if len(focusKeywords) == 0 {
			return false
		}
		lower := strings.ToLower(text)
		for _, kw := range focusKeywords {
			if strings.Contains(lower, kw) {
				return true
			}
		}
		return false
	}

	var facts, focusedFacts, commands, failures, decisions, open []string
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		lower := strings.ToLower(content)
		if focus != "" && message.Role != "system" && matchesFocus(content) {
			focusedFacts = append(focusedFacts, fmt.Sprintf("[%s] %s", message.Role, truncateText(content, 500)))
		}
		switch {
		case message.Role == "user" && strings.Contains(lower, "antwort des nutzers"):
			decisions = append(decisions, truncateText(content, 800))
		case strings.Contains(lower, "tool result"):
			if strings.Contains(lower, "error:") || strings.Contains(lower, "status: fehler") || strings.Contains(lower, "exitcode: 1") {
				failures = append(failures, truncateText(content, 1000))
			} else {
				commands = append(commands, truncateText(content, 600))
			}
		case message.Role == "assistant":
			var action AgentAction
			if json.Unmarshal([]byte(content), &action) == nil {
				facts = append(facts, action.Action+": "+truncateText(action.Message, 240))
				if action.Action == "ask_user" {
					open = append(open, truncateText(action.Message, 240))
				}
			}
		}
	}
	if focus != "" {
		state.Summary = fmt.Sprintf("Ältere Nachrichten wurden gezielt mit Fokus %q lokal verdichtet. Die zuletzt erhaltenen Nachrichten bleiben zusätzlich unverändert im Kontext.", truncateText(focus, 200))
		combinedFacts := append(tailStrings(focusedFacts, 8), tailStrings(facts, 4)...)
		state.ProjectFacts = tailStrings(combinedFacts, 10)
	} else {
		state.Summary = "Ältere Nachrichten wurden lokal verdichtet. Die zuletzt erhaltenen Nachrichten bleiben zusätzlich unverändert im Kontext."
		state.ProjectFacts = tailStrings(facts, 8)
	}
	state.Decisions = tailStrings(decisions, 4)
	state.Commands = tailStrings(commands, 6)
	state.Failures = tailStrings(failures, 4)
	state.OpenItems = tailStrings(open, 4)
	state.NextRecommended = "Mit der nächsten sicheren, überprüfbaren Werkzeugaktion fortfahren und keine bereits geklärte Frage wiederholen."
	return state
}

func tailStrings(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return append([]string(nil), values[len(values)-n:]...)
}

// CompactMessagesWithFocus compacts a message history with an optional focus
// instruction (used by `/compact <focus>` as well as automated compaction).
// Unlike automatic threshold compaction, calling CompactMessagesWithFocus
// directly always compacts whenever len(messages) >= 3.
func CompactMessagesWithFocus(ctx context.Context, messages []OllamaMessage, focusInstruction string, cfg Config, client *OllamaClient, model string, originalTask string) ([]OllamaMessage, bool) {
	result, compacted, _, _, _, _ := compactMessagesCore(ctx, client, model, messages, cfg, originalTask, focusInstruction, true)
	return result, compacted
}

func (s *AppState) CompactMessagesWithFocus(ctx context.Context, model string, messages []OllamaMessage, cfg Config, originalTask string, focusInstruction string) ([]OllamaMessage, bool) {
	return s.compactAgentMessagesInternal(ctx, model, messages, cfg, originalTask, focusInstruction, true)
}

func (s *AppState) compactAgentMessages(ctx context.Context, model string, messages []OllamaMessage, cfg Config, originalTask string) ([]OllamaMessage, bool) {
	return s.compactAgentMessagesInternal(ctx, model, messages, cfg, originalTask, "", false)
}

func (s *AppState) compactAgentMessagesInternal(ctx context.Context, model string, messages []OllamaMessage, cfg Config, originalTask, focusInstruction string, force bool) ([]OllamaMessage, bool) {
	effCfg := cfg
	var client *OllamaClient
	if s != nil && s.Ollama != nil {
		client = s.Ollama
		if strings.TrimSpace(effCfg.OllamaURL) == "" && strings.TrimSpace(s.Ollama.BaseURL) != "" {
			effCfg.OllamaURL = s.Ollama.BaseURL
		}
	}
	result, compacted, mode, before, after, keep := compactMessagesCore(ctx, client, model, messages, effCfg, originalTask, focusInstruction, force)
	if !compacted {
		return result, false
	}
	if s != nil {
		detail := fmt.Sprintf("Modus: %s\nGeschätzte Tokens: %d → %d\nBeibehaltene aktuelle Nachrichten: %d", mode, before, after, keep)
		if f := strings.TrimSpace(focusInstruction); f != "" {
			detail += "\nFokus: " + truncateText(f, 300)
		}
		s.AddEvent(UIEvent{
			Type:    "context_compacted",
			Message: "Kontext komprimiert",
			Detail:  detail,
		})
	}
	return result, true
}

func compactMessagesCore(ctx context.Context, client *OllamaClient, model string, messages []OllamaMessage, cfg Config, originalTask, focusInstruction string, force bool) ([]OllamaMessage, bool, string, int, int, int) {
	if len(messages) < 3 {
		return messages, false, "", 0, 0, 0
	}
	if !force && (!shouldCompactMessages(messages, cfg) || len(messages) < 4) {
		return messages, false, "", 0, 0, 0
	}
	before := estimateMessageTokens(messages)
	keep := cfg.ContextCompactionKeepRecent
	if keep < 4 {
		if force {
			keep = 4
		} else {
			keep = 12
		}
	}
	if keep > len(messages)-2 {
		keep = len(messages) - 2
	}
	if keep < 1 {
		keep = 1
	}

	focus := strings.TrimSpace(focusInstruction)
	state := deterministicContextSummaryWithFocus(messages, originalTask, focus)
	mode := "deterministisch"
	effectiveCtx := effectiveCompactionContextLength(cfg)
	if client != nil && strings.TrimSpace(model) != "" && before <= effectiveCtx && effectiveCtx >= minLocalContextTokens {
		focusBlock := ""
		if focus != "" {
			focusBlock = "\nFOKUS-ANWEISUNG (HÖCHSTE PRIORITÄT):\n" +
				"Priorisiere bei der Verdichtung sämtliche Details, Dateipfade, Entscheidungen, Fehler und offenen Punkte rund um: " +
				truncateText(focus, 800) +
				"\nKürze themenfremde Historie deutlich stärker.\n"
		}
		prompt := `Verdichte den folgenden Verlauf eines Software-Agenten in einen belastbaren Arbeitszustand.
Bewahre ausschließlich überprüfbare Fakten und ausdrücklich getroffene Entscheidungen. Erfinde nichts.
Unbedingt erhalten: ursprüngliche Nutzeraufgabe, Nutzerfreigaben/Absagen, Projektstruktur und Buildsystem, gelesene/geänderte Dateien, ausgeführte Befehle mit Ergebnis, genaue Fehlerdiagnosen, Git-Zustand, offene Blocker und die nächste sinnvolle Aktion.
Nicht aufnehmen: Gedankenkette, Smalltalk, redundante Statusmeldungen oder wiederholte Anweisungen.
Der Zustand muss reichen, damit ein neuer Agentenlauf ohne ältere Nachrichten korrekt fortsetzen kann.` + focusBlock + `

ORIGINAL TASK:
` + truncateText(originalTask, 2400) + "\n\nTRANSCRIPT:\n" + truncateText(renderMessagesForCompaction(messages), effectiveCtx*2)

		compactionMessages := []OllamaMessage{
			{Role: "system", Content: "Du erstellst eine faktische, strukturierte Arbeitszusammenfassung für einen Coding-Agenten. Keine Gedankenkette."},
			{Role: "user", Content: prompt},
		}
		if ctx == nil {
			ctx = context.Background()
		}
		compactionCtx, cancel := context.WithTimeout(ctx, minDuration(45*time.Second, timeDurationSeconds(cfg.ModelTimeout)))
		defer cancel()
		content, err := client.Chat(compactionCtx, model, compactionMessages, compactionSchema)
		if err == nil {
			var parsed compactedContext
			if json.Unmarshal([]byte(content), &parsed) == nil && strings.TrimSpace(parsed.Summary) != "" {
				if strings.TrimSpace(parsed.OriginalTask) == "" {
					parsed.OriginalTask = truncateText(originalTask, 2400)
				}
				if focus != "" && strings.TrimSpace(parsed.Focus) == "" {
					parsed.Focus = truncateText(focus, 600)
				}
				state = parsed
				mode = "modellgestützt"
			}
		}
	}

	target := postCompactionTokenTarget(cfg)
	result := buildCompactedMessages(messages, state, keep)
	after := estimateMessageTokens(result)
	for after > target && keep > 4 {
		keep--
		result = buildCompactedMessages(messages, state, keep)
		after = estimateMessageTokens(result)
	}
	if after > target {
		result = truncateCompactedRecentMessages(result, target)
		after = estimateMessageTokens(result)
	}
	return result, true, mode, before, after, keep
}

func buildCompactedMessages(messages []OllamaMessage, state compactedContext, keep int) []OllamaMessage {
	result := make([]OllamaMessage, 0, keep+2)
	result = append(result, messages[0])
	result = append(result, OllamaMessage{Role: "user", Content: renderCompactedState(state)})
	result = append(result, messages[len(messages)-keep:]...)
	return result
}

func truncateCompactedRecentMessages(messages []OllamaMessage, target int) []OllamaMessage {
	if estimateMessageTokens(messages) <= target || len(messages) <= 2 {
		return messages
	}
	// First trim older recent messages while preserving the very latest tool/read_file
	// message so an agent that just read a file or test error can actually see it.
	for _, limit := range []int{16000, 10000, 6000, 3500, 2000, 1200} {
		candidate := append([]OllamaMessage(nil), messages...)
		lastIdx := len(candidate) - 1
		for i := 2; i < len(candidate); i++ {
			effLimit := limit
			if i == lastIdx && effLimit < 4500 {
				effLimit = 4500
			}
			if utf8.RuneCountInString(candidate[i].Content) > effLimit {
				candidate[i].Content = truncateText(candidate[i].Content, effLimit) + "\n\n[Kontextbudget: diese neuere Nachricht wurde gekürzt; vollständige Ausgabe steht im Ausgabenbereich.]"
			}
			if utf8.RuneCountInString(candidate[i].Thinking) > 0 {
				candidate[i].Thinking = ""
			}
		}
		if estimateMessageTokens(candidate) <= target {
			return candidate
		}
		messages = candidate
	}
	for estimateMessageTokens(messages) > target && len(messages) > 4 {
		messages = append(append([]OllamaMessage(nil), messages[:2]...), messages[3:]...)
	}
	if estimateMessageTokens(messages) > target {
		candidate := append([]OllamaMessage(nil), messages...)
		for i := 2; i < len(candidate); i++ {
			candidate[i].Content = truncateText(candidate[i].Content, 600) + "\n\n[Kontextbudget: stark gekürzt; vollständige Ausgabe steht im Ausgabenbereich.]"
			candidate[i].Thinking = ""
		}
		return candidate
	}
	return messages
}

func renderMessagesForCompaction(messages []OllamaMessage) string {
	var b strings.Builder
	for i, message := range messages {
		fmt.Fprintf(&b, "[%d] %s:\n%s\n\n", i+1, strings.ToUpper(message.Role), truncateText(message.Content, 24000))
	}
	return truncateText(b.String(), 420000)
}

func minDuration(a, b time.Duration) time.Duration {
	if b <= 0 || a < b {
		return a
	}
	return b
}

func timeDurationSeconds(seconds int) time.Duration {
	if seconds <= 0 {
		return 90 * time.Second
	}
	return time.Duration(seconds) * time.Second
}
