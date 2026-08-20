// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const agentLoopHistoryLimit = 32

type agentLoopBlockReason uint8

const (
	agentLoopBlockNone agentLoopBlockReason = iota
	agentLoopBlockCycle
	agentLoopBlockRepeatedFailure
	agentLoopBlockRepeatedOutcome
)

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

// agentActionFingerprint hashes the complete structured action while
// deliberately ignoring Message. A rewritten explanation is not progress, but
// edit payloads, command arguments and MCP/tool arguments remain significant.
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

func (g *agentLoopGuard) ShouldBlock(action AgentAction) agentLoopBlockReason {
	if g == nil || loopGuardControlAction(action) {
		return agentLoopBlockNone
	}
	fingerprint := agentActionFingerprint(action)

	// Detect A/B, A/B/C and short four-action cycles only after the complete
	// action/outcome sequence has repeated twice. Requiring outcomes and failure
	// state to match avoids treating a changing diagnostic cycle as stagnation.
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
			return agentLoopBlockCycle
		}
	}

	matches := make([]agentLoopObservation, 0, 2)
	for i := len(g.history) - 1; i >= 0 && len(matches) < 2; i-- {
		if g.history[i].Fingerprint == fingerprint {
			matches = append(matches, g.history[i])
		}
	}
	if len(matches) < 2 {
		return agentLoopBlockNone
	}
	if matches[0].Failed && matches[1].Failed {
		return agentLoopBlockRepeatedFailure
	}
	if matches[0].Outcome != "" && matches[0].Outcome == matches[1].Outcome {
		return agentLoopBlockRepeatedOutcome
	}
	return agentLoopBlockNone
}

func (g *agentLoopGuard) Observe(action AgentAction, result string, failed bool, originalTask string) {
	if g == nil || loopGuardControlAction(action) {
		return
	}
	fingerprint := agentActionFingerprint(action)
	outcome := agentOutcomeFingerprint(result)

	// A successful real project mutation or successful verification establishes
	// a new trustworthy state. Native no-op edits fail before this point, so they
	// cannot incorrectly reset stagnation history.
	if !failed && (actionMutatesProject(action) || actionVerifiesProject(action, originalTask)) {
		g.history = g.history[:0]
		return
	}

	// The same diagnostic returning different data is new evidence (for example
	// after an external process changed the project), so stale history is reset.
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
		g.history = append(g.history[:0], g.history[len(g.history)-agentLoopHistoryLimit:]...)
	}
}

func loopGuardControlAction(action AgentAction) bool {
	return action.Action == "finish" || action.Action == "ask_user"
}

func agentImmediateRepeatMessage(cfg Config) string {
	return localizeConfigText(cfg, "Identische Werkzeugaktion blockiert", "Identical tool action blocked")
}

func agentImmediateRepeatDetail(cfg Config, action AgentAction) string {
	return localizeConfigText(cfg,
		action.Action+" wurde unmittelbar zuvor bereits ohne neue Information angefordert.",
		action.Action+" was already requested immediately before without new information.")
}

func agentImmediateRepeatHint(cfg Config, repeatBlocks int) string {
	de := "SYSTEMHINWEIS: Die identische Aktion wurde blockiert. Wähle eine andere Diagnose, ändere Argumente oder Vorgehen und werte die bereits vorhandene Ausgabe aus."
	en := "SYSTEM NOTICE: The identical action was blocked. Choose a different diagnosis, change the arguments or approach, and use the output already available."
	if repeatBlocks >= 2 {
		de += " Stelle keine weitere gleichartige Rückfrage; schließe mit einer präzisen Fehlerdiagnose ab, falls keine sichere Alternative existiert."
		en += " Do not ask another equivalent question; finish with a precise failure diagnosis if no safe alternative exists."
	}
	return localizeConfigText(cfg, de, en)
}

func agentLoopBlockDetail(cfg Config, reason agentLoopBlockReason, action AgentAction) string {
	var de, en string
	switch reason {
	case agentLoopBlockCycle:
		de = "Derselbe Aktions-/Ergebniszyklus wurde bereits zweimal ohne beobachtbaren Fortschritt durchlaufen."
		en = "The same action/result cycle already ran twice without observable progress."
	case agentLoopBlockRepeatedFailure:
		de = "Dieselbe strukturierte Werkzeugaktion ist bereits zweimal fehlgeschlagen. Eine dritte unveränderte Ausführung wird blockiert."
		en = "The same structured tool action already failed twice. A third unchanged execution is blocked."
	case agentLoopBlockRepeatedOutcome:
		de = "Dieselbe strukturierte Werkzeugaktion hat bereits zweimal dasselbe Ergebnis geliefert. Eine dritte unveränderte Ausführung ohne neuen Zustand wird blockiert."
		en = "The same structured tool action already returned the same result twice. A third unchanged execution without new state is blocked."
	default:
		de = "Die Werkzeugaktion wurde wegen fehlenden Fortschritts blockiert."
		en = "The tool action was blocked because it made no progress."
	}
	return fmt.Sprintf("%s: %s", action.Action, localizeConfigText(cfg, de, en))
}

func agentLoopBlockHint(cfg Config, repeatBlocks int) string {
	de := "SYSTEMHINWEIS: LocalCode hat eine stagnierende Werkzeugschleife blockiert. Verwende die vorhandenen Ergebnisse als Evidenz und ändere Diagnose, Argumente oder Vorgehen. Wiederhole nicht dieselbe Aktion bzw. denselben kurzen Zyklus ohne neue Information."
	en := "SYSTEM NOTICE: LocalCode blocked a stagnant tool loop. Use the existing results as evidence and change the diagnosis, arguments, or approach. Do not repeat the same action or short cycle without new information."
	if repeatBlocks >= 2 {
		de += " Falls keine sichere neue Diagnose existiert, schließe mit der präzisen Fehlerursache ab statt weiter zu kreisen."
		en += " If no safe new diagnostic path exists, finish with the precise failure cause instead of continuing to loop."
	}
	return localizeConfigText(cfg, de, en)
}
