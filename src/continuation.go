// SPDX-License-Identifier: Apache-2.0

package main

import "strings"

func likelyContinuationAnswer(question, answer string) bool {
	q := normalizedQuestion(question)
	a := normalizedQuestion(answer)
	if q == "" || a == "" {
		return false
	}
	words := strings.Fields(a)
	if len(words) <= 12 {
		for _, prefix := range []string{
			"ja", "j", "ok", "okay", "mach", "tu", "installiere", "erneut", "nochmal", "versuch", "weiter", "nein", "n", "abbrechen", "manuell",
			"das ist verbunden", "ist verbunden", "gerät ist verbunden", "geraet ist verbunden",
		} {
			if a == prefix || strings.HasPrefix(a, prefix+" ") {
				return true
			}
		}
	}
	stop := map[string]bool{"es": true, "ist": true, "dass": true, "der": true, "die": true, "das": true, "ein": true, "eine": true, "und": true, "oder": true, "zu": true, "im": true, "in": true, "mit": true, "von": true, "sie": true, "ich": true, "du": true, "bitte": true, "moechten": true, "möchten": true, "wollen": true, "soll": true}
	qset := map[string]bool{}
	for _, w := range strings.Fields(q) {
		if len(w) >= 3 && !stop[w] {
			qset[w] = true
		}
	}
	overlap := 0
	for _, w := range words {
		if qset[w] {
			overlap++
		}
	}
	if overlap >= 1 && len(words) <= 24 {
		return true
	}
	// Longer imperative requests with no semantic overlap are new tasks, not
	// answers to an old question. This prevents stale Git/ADB questions from
	// hijacking requests such as "suche die neuesten Nachrichten".
	for _, lead := range []string{"analysiere ", "kompiliere ", "suche ", "schau ", "erstelle ", "aendere ", "ändere ", "pruefe ", "prüfe ", "recherchiere ", "verteile ", "installiere "} {
		if strings.HasPrefix(a, lead) {
			return false
		}
	}
	return len(words) <= 6
}

func normalizeAgentAction(a AgentAction, fallbackTask string) AgentAction {
	if a.Action == "web_search" {
		candidate := strings.TrimSpace(a.Query)
		if candidate == "" {
			candidate = strings.TrimSpace(a.Message)
		}
		a.Query = deriveWebQuery(fallbackTask, candidate)
	}
	if a.Action == "git" && len(a.Args) == 0 {
		intent := normalizedQuestion(strings.TrimSpace(a.Message + " " + fallbackTask))
		switch {
		case strings.Contains(intent, "commit") || strings.Contains(intent, "committe") || strings.Contains(intent, "eincheck"):
			a.Action = "git_commit"
			a.StageAll = true
			a.CommitMessage = deriveCommitMessage(fallbackTask)
		case strings.Contains(intent, "status"):
			a.Args = []string{"status", "--short", "--branch"}
		case strings.Contains(intent, "diff"):
			a.Args = []string{"diff", "--stat"}
		case strings.Contains(intent, "initialis") || strings.Contains(intent, "git init") || strings.Contains(intent, "repository anlegen"):
			a.Args = []string{"init"}
		case strings.Contains(intent, "push"):
			a.Args = []string{"push"}
		case strings.Contains(intent, "pull"):
			a.Args = []string{"pull", "--ff-only"}
		case strings.Contains(intent, "add") || strings.Contains(intent, "stage") || strings.Contains(intent, "hinzuf"):
			if strings.Contains(intent, ".gitignore") {
				a.Args = []string{"add", "--", ".gitignore"}
			} else {
				a.Args = []string{"add", "-A", "--", ".", ":(exclude).vs/**"}
			}
		default:
			a.Args = []string{"status", "--short", "--branch"}
		}
	}
	if a.Action == "git_commit" {
		a.StageAll = true
		if strings.TrimSpace(a.CommitMessage) == "" {
			a.CommitMessage = deriveCommitMessage(fallbackTask)
		}
	}
	return a
}

func blockedAvoidanceQuestion(task, question string) (bool, string) {
	t := normalizedQuestion(task)
	q := normalizedQuestion(question)
	if q == "" {
		return false, ""
	}
	gitQuestion := strings.Contains(q, "git") && (strings.Contains(q, "initialis") || strings.Contains(q, "erstellen") || strings.Contains(q, "anlegen") || strings.Contains(q, "git init"))
	gitTask := false
	for _, marker := range []string{"git", "repository", "repo", "commit", "branch", "push", "pull request", "versionier"} {
		if strings.Contains(t, marker) {
			gitTask = true
			break
		}
	}
	if gitQuestion && !gitTask {
		return true, "Ein Git-Repository ist für diese Aufgabe nicht erforderlich. Fahre ohne Git fort. Initialisiere Git nur, wenn der Nutzer ausdrücklich Git, Versionierung, Commit, Branch, Push oder Pull Request verlangt."
	}
	for _, marker := range []string{
		"bitte bestätigen sie, dass sie den debug-apk bereits erfolgreich gebaut",
		"bitte bestaetigen sie, dass sie den debug-apk bereits erfolgreich gebaut",
		"stellen sie sicher, dass adb installiert",
		"adb korrekt installiert und in ihrem systempfad",
		"befehl manuell eingeben oder erneut versuchen",
		"möchten sie den befehl manuell eingeben",
		"moechten sie den befehl manuell eingeben",
	} {
		if strings.Contains(q, marker) {
			return true, "Diese Frage kann LocalCode selbst mit Werkzeugen klären. Nutze discover_tool oder tool_inventory, prüfe absolute Pfade, führe eine sichere Diagnose aus und werte Exitcode, STDOUT und STDERR aus. Fehlt ein unterstütztes Werkzeug, löst LocalCode automatisch eine Installationsgenehmigung aus. Frage den Nutzer erst nach einer tatsächlich notwendigen physischen Aktion wie RSA-Bestätigung am Gerät."
		}
	}
	return false, ""
}

func taskAutomationHint(task string) string {
	t := normalizedQuestion(task)
	for _, marker := range []string{"verteile", "auf das handy", "auf dem handy", "installiere die app", "deploy", "adb install"} {
		if strings.Contains(t, marker) {
			return "SYSTEMHINWEIS ZUR AUFGABE: Verwende für diesen Android-Deployment-Auftrag bevorzugt deploy_android. Frage nicht, ob bereits gebaut wurde oder ob ADB installiert ist; die Aktion prüft, baut, repariert fehlende unterstützte Werkzeuge nach Genehmigung und installiert die APK."
		}
	}
	for _, marker := range []string{"kompiliere", "kompilieren", "baue das projekt", "build das projekt", "build project", "erstelle den build"} {
		if strings.Contains(t, marker) {
			return "SYSTEMHINWEIS ZUR AUFGABE: Verwende bevorzugt build_project. Rate keinen Buildbefehl und frage nicht nach bereits vorhandenen Artefakten; die Aktion erkennt das Buildsystem und führt den Build aus."
		}
	}
	for _, marker := range []string{"neueste nachrichten", "aktuelle nachrichten", "schau im internet", "recherchiere im internet", "suche im internet"} {
		if strings.Contains(t, marker) {
			return "SYSTEMHINWEIS ZUR AUFGABE: Verwende web_search mit einer konkreten, nicht leeren Suchanfrage aus der Nutzeraufgabe. Eine alte Git-, Build- oder ADB-Rückfrage gehört nicht zu dieser neuen Aufgabe."
		}
	}
	for _, marker := range []string{"analysiere das projekt", "analysiere projekt", "prüfe das projekt", "pruefe das projekt"} {
		if strings.Contains(t, marker) {
			return "SYSTEMHINWEIS ZUR AUFGABE: Verwende project_info und die Lese-/Suchwerkzeuge. Ein fehlendes Git-Repository ist kein Hindernis und darf keine Rückfrage nach git init auslösen."
		}
	}
	return ""
}
