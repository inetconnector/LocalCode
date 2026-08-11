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

func shouldCompactMessages(messages []OllamaMessage, cfg Config) bool {
	if !cfg.ContextCompactionEnabled || cfg.ContextLength < 4096 {
		return false
	}
	threshold := cfg.ContextLength * cfg.ContextCompactionThresholdPercent / 100
	if threshold <= 0 {
		threshold = cfg.ContextLength * 68 / 100
	}
	return estimateMessageTokens(messages) >= threshold
}

func postCompactionTokenTarget(cfg Config) int {
	contextLength := cfg.ContextLength
	if contextLength < 4096 {
		contextLength = 32768
	}
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
	state := compactedContext{OriginalTask: strings.TrimSpace(originalTask)}
	var facts, commands, failures, decisions, open []string
	for _, message := range messages {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		lower := strings.ToLower(content)
		switch {
		case message.Role == "user" && strings.Contains(lower, "antwort des nutzers"):
			decisions = append(decisions, truncateText(content, 1200))
		case strings.Contains(lower, "tool result"):
			if strings.Contains(lower, "error:") || strings.Contains(lower, "status: fehler") || strings.Contains(lower, "exitcode: 1") {
				failures = append(failures, truncateText(content, 1800))
			} else {
				commands = append(commands, truncateText(content, 1400))
			}
		case message.Role == "assistant":
			var action AgentAction
			if json.Unmarshal([]byte(content), &action) == nil {
				facts = append(facts, action.Action+": "+truncateText(action.Message, 500))
				if action.Action == "ask_user" {
					open = append(open, action.Message)
				}
			}
		}
	}
	state.Summary = "Ältere Nachrichten wurden lokal verdichtet. Die zuletzt erhaltenen Nachrichten bleiben zusätzlich unverändert im Kontext."
	state.Decisions = tailStrings(decisions, 8)
	state.ProjectFacts = tailStrings(facts, 16)
	state.Commands = tailStrings(commands, 12)
	state.Failures = tailStrings(failures, 8)
	state.OpenItems = tailStrings(open, 6)
	state.NextRecommended = "Mit der nächsten sicheren, überprüfbaren Werkzeugaktion fortfahren und keine bereits geklärte Frage wiederholen."
	return state
}

func tailStrings(values []string, n int) []string {
	if len(values) <= n {
		return values
	}
	return append([]string(nil), values[len(values)-n:]...)
}

func (s *AppState) compactAgentMessages(ctx context.Context, model string, messages []OllamaMessage, cfg Config, originalTask string) ([]OllamaMessage, bool) {
	if !shouldCompactMessages(messages, cfg) || len(messages) < 4 {
		return messages, false
	}
	before := estimateMessageTokens(messages)
	keep := cfg.ContextCompactionKeepRecent
	if keep < 6 {
		keep = 12
	}
	if keep > len(messages)-2 {
		keep = len(messages) - 2
	}

	prompt := `Verdichte den folgenden Verlauf eines Software-Agenten in einen belastbaren Arbeitszustand.
Bewahre ausschließlich überprüfbare Fakten und ausdrücklich getroffene Entscheidungen. Erfinde nichts.
Unbedingt erhalten: ursprüngliche Nutzeraufgabe, Nutzerfreigaben/Absagen, Projektstruktur und Buildsystem, gelesene/geänderte Dateien, ausgeführte Befehle mit Ergebnis, genaue Fehlerdiagnosen, Git-Zustand, offene Blocker und die nächste sinnvolle Aktion.
Nicht aufnehmen: Gedankenkette, Smalltalk, redundante Statusmeldungen oder wiederholte Anweisungen.
Der Zustand muss reichen, damit ein neuer Agentenlauf ohne ältere Nachrichten korrekt fortsetzen kann.

ORIGINAL TASK:
` + originalTask + "\n\nTRANSCRIPT:\n" + renderMessagesForCompaction(messages)

	compactionMessages := []OllamaMessage{
		{Role: "system", Content: "Du erstellst eine faktische, strukturierte Arbeitszusammenfassung für einen Coding-Agenten. Keine Gedankenkette."},
		{Role: "user", Content: prompt},
	}
	compactionCtx, cancel := context.WithTimeout(ctx, minDuration(90*time.Second, timeDurationSeconds(cfg.ModelTimeout)))
	defer cancel()
	content, err := s.Ollama.Chat(compactionCtx, model, compactionMessages, compactionSchema)
	state := deterministicContextSummary(messages, originalTask)
	mode := "deterministisch"
	if err == nil {
		var parsed compactedContext
		if json.Unmarshal([]byte(content), &parsed) == nil && strings.TrimSpace(parsed.Summary) != "" {
			if strings.TrimSpace(parsed.OriginalTask) == "" {
				parsed.OriginalTask = originalTask
			}
			state = parsed
			mode = "modellgestützt"
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
	s.AddEvent(UIEvent{
		Type:    "context_compacted",
		Message: "Kontext komprimiert",
		Detail:  fmt.Sprintf("Modus: %s\nGeschätzte Tokens: %d → %d\nBeibehaltene aktuelle Nachrichten: %d", mode, before, after, keep),
	})
	return result, true
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
	for _, limit := range []int{16000, 10000, 6000, 3500, 2000, 1200} {
		candidate := append([]OllamaMessage(nil), messages...)
		for i := 2; i < len(candidate); i++ {
			if utf8.RuneCountInString(candidate[i].Content) > limit {
				candidate[i].Content = truncateText(candidate[i].Content, limit) + "\n\n[Kontextbudget: diese neuere Nachricht wurde gekürzt; vollständige Ausgabe steht im Ausgabenbereich.]"
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
