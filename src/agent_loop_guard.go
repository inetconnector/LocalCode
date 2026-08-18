// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const agentLoopHistoryLimit = 32

type agentLoopObservation struct {
	Action      string
	Fingerprint string
	Outcome     string
	Failed      bool
}

type agentLoopGuard struct {
	history []agentLoopObservation
}

func newAgentLoopGuard() *agentLoopGuard {
	return &agentLoopGuard{history: make([]agentLoopObservation, 0, agentLoopHistoryLimit)}
}

// agentActionFingerprint hashes the complete structured action input while
// deliberately ignoring the human-readable message. This makes semantically
// identical tool calls comparable across model turns without treating a changed
// explanation as progress. Unlike actionSignature, edit payloads and MCP
// arguments are part of this fingerprint.
func agentActionFingerprint(action AgentAction) string {
	action.Message = ""
	data, err := json.Marshal(action)
	if err != nil {
		data = []byte(actionSignature(action))
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}

func agentOutcomeFingerprint(result string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(result, "\r\n", "\n"))
	if normalized == "" {
		return "empty"
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:12])
}

func (g *agentLoopGuard) ShouldBlock(action AgentAction) string {
	if g == nil || action.Action == "finish" || action.Action == "ask_user" {
		return ""
	}
	fingerprint := agentActionFingerprint(action)
	if fingerprint == "" {
		return ""
	}

	matches := make([]agentLoopObservation, 0, 3)
	for i := len(g.history) - 1; i >= 0 && len(matches) < 3; i-- {
		if g.history[i].Fingerprint == fingerprint {
			matches = append(matches, g.history[i])
		}
	}
	if len(matches) >= 2 {
		if matches[0].Failed && matches[1].Failed {
			return "Die gleiche strukturierte Werkzeugaktion ist in dieser Agentensitzung bereits zweimal fehlgeschlagen. Eine dritte unveränderte Ausführung wird blockiert."
		}
		if matches[0].Outcome != "" && matches[0].Outcome == matches[1].Outcome {
			return "Die gleiche strukturierte Werkzeugaktion hat in dieser Agentensitzung bereits zweimal dasselbe Ergebnis geliefert. Eine dritte unveränderte Ausführung ohne neuen Zustand wird blockiert."
		}
	}

	for period := 2; period <= 4; period++ {
		if len(g.history) < period*2 {
			continue
		}
		start := len(g.history) - period*2
		cycleA := g.history[start : start+period]
		cycleB := g.history[start+period:]
		equal := true
		for i := 0; i < period; i++ {
			if cycleA[i].Fingerprint != cycleB[i].Fingerprint || cycleA[i].Outcome != cycleB[i].Outcome || cycleA[i].Failed != cycleB[i].Failed {
				equal = false
				break
			}
		}
		if equal && cycleA[0].Fingerprint == fingerprint {
			return "Eine wiederholte Werkzeugzyklus-Schleife wurde erkannt. Derselbe Aktions-/Ergebniszyklus wurde bereits zweimal ohne beobachtbaren Fortschritt durchlaufen."
		}
	}
	return ""
}

func (g *agentLoopGuard) Observe(action AgentAction, result string, failed bool, originalTask string) {
	if g == nil || action.Action == "finish" || action.Action == "ask_user" {
		return
	}
	fingerprint := agentActionFingerprint(action)
	outcome := agentOutcomeFingerprint(result)

	// A successful real project mutation is progress. Native no-op edits are
	// errors before reaching this point, so they cannot incorrectly reset the
	// guard. A successful verification also establishes a new trustworthy state.
	if !failed && (actionMutatesProject(action) || actionVerifiesProject(action, originalTask)) {
		g.history = g.history[:0]
		return
	}

	// A repeated read/tool action that now returns different information is also
	// progress, for example after an external process changed the project.
	for i := len(g.history) - 1; i >= 0; i-- {
		if g.history[i].Fingerprint != fingerprint {
			continue
		}
		if g.history[i].Outcome != outcome {
			g.history = g.history[:0]
		}
		break
	}

	g.history = append(g.history, agentLoopObservation{
		Action:      action.Action,
		Fingerprint: fingerprint,
		Outcome:     outcome,
		Failed:      failed,
	})
	if len(g.history) > agentLoopHistoryLimit {
		copy(g.history, g.history[len(g.history)-agentLoopHistoryLimit:])
		g.history = g.history[:agentLoopHistoryLimit]
	}
}
