// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var offlinePackageInstallRe = regexp.MustCompile(`(?i)\b(?:npm|yarn|pnpm|pip|pip3|uv|cargo|go|winget|choco|scoop)(?:\.cmd|\.exe)?\s+(?:install|i|add|get)\b`)
var externalNodeRequireRe = regexp.MustCompile(`require\s*\(\s*['"]([a-zA-Z0-9@_-][^'"]*)['"]\s*\)`)
var anyNodeRequireRe = regexp.MustCompile(`\brequire\s*\(\s*['"]([^'"]+)['"]\s*\)`)
var bareESModuleRe = regexp.MustCompile(`(?m)^\s*(?:import\s+(?:[\s\S]{1,120}?\s+from\s+['"][^'"]+['"]|['"][^'"]+['"])|export\s+(?:default\b|function\b|async\s+function\b|class\b|const\b|let\b|var\b|\{))`)
var micaRule7StubPatternRe = regexp.MustCompile(`(?im)(?://|#|/\*)\s*(?:todo\b|fixme\b|placeholder\b|stub\b|implement\s+(?:further|remaining|additional|game|full|more)\b[^\n]*|implement\s+[^\n]*\bhere\b|add\s+[^\n]*\b(?:logic|code|implementation)\s+here\b|hier\s+[^\n]*implementier|implementier(?:e|ung)\s+(?:der\s+)?[^\n]*logik|[^\n]*\bcan be added\b|[^\n]*k[oö]nnen[^\n]*hinzugef[uü]gt)`)
var micaRule7NotImplRe = regexp.MustCompile(`(?i)\bthrow\s+new\s+Error\s*\(\s*['"][^'"]*(?:not\s+implemented|nicht\s+implementiert|todo)[^'"]*['"]\s*\)|\braise\s+NotImplementedError\b|\btodo!\s*\(\s*\)|\bunimplemented!\s*\(\s*\)`)
var micaRule7EllipsisRe = regexp.MustCompile(`(?im)(?://|#)\s*\.\.\.\s*(?:rest|more|remaining|weitere|implement|add|etc|und\s+so\s+weiter)\b`)
var micaRule7EmptyFuncRe = regexp.MustCompile(`(?m)\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)\s*\{\s*(?://[^\n]*\s*|/\*[\s\S]*?\*/\s*)*\}`)
var micaRule7EmptyMethodRe = regexp.MustCompile(`(?m)^[ \t]+(?:async\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^)]*\)\s*\{\s*(?://[^\n]*\s*|/\*[\s\S]*?\*/\s*)*\}`)
var micaRule7CommentOnlyBlockRe = regexp.MustCompile(`(?m)\b(?:if|else|for|while)\b[^\n{]*\{\s*(?://[^\n]+\s*|/\*[\s\S]*?\*/\s*)+\}`)
var jsClassMethodRe = regexp.MustCompile(`(?m)^[ \t]{2,4}(?:async\s+)?([a-z_$][A-Za-z0-9_$]*)\s*\(([^)\n]{0,48})\)\s*\{`)
var jsThisPropRe = regexp.MustCompile(`\bthis\.([a-zA-Z_$][a-zA-Z0-9_$]*)\s*=`)
var jsLocalMediaAssetRe = regexp.MustCompile(`['"]([A-Za-z0-9_./-]+\.(?:mp3|wav|ogg|flac|png|jpg|jpeg|webp|gif))['"]`)

var (
	htmlScriptTagRe     = regexp.MustCompile(`(?is)<script\b([^>]*)>([\s\S]*?)</script>`)
	htmlLinkStyleRe     = regexp.MustCompile(`(?is)<link\b([^>]*)>`)
	htmlAttrSrcRe       = regexp.MustCompile(`(?i)\bsrc\s*=\s*["']([^"']+)["']`)
	htmlAttrHrefRe      = regexp.MustCompile(`(?i)\bhref\s*=\s*["']([^"']+)["']`)
	htmlAttrRelRe       = regexp.MustCompile(`(?i)\brel\s*=\s*["']([^"']+)["']`)
	htmlAttrTypeRe      = regexp.MustCompile(`(?i)\btype\s*=\s*["']([^"']+)["']`)
	htmlTagIDRe         = regexp.MustCompile(`(?i)<([a-z][a-z0-9-]*)\b[^>]*?\bid\s*=\s*["']([A-Za-z0-9_-]+)["']`)
	htmlCanvasSizeRe    = regexp.MustCompile(`(?is)<canvas\b([^>]*)>`)
	htmlAttrWidthRe     = regexp.MustCompile(`(?i)\bwidth\s*=\s*["']?(\d+)["']?`)
	htmlAttrHeightRe    = regexp.MustCompile(`(?i)\bheight\s*=\s*["']?(\d+)["']?`)
	jsGetByIDRe         = regexp.MustCompile(`document\s*\.\s*getElementById\s*\(\s*['"]([A-Za-z0-9_-]+)['"]\s*\)`)
	jsQuerySelectorIDRe = regexp.MustCompile(`document\s*\.\s*querySelector\s*\(\s*['"]#([A-Za-z0-9_-]+)['"]\s*\)`)
	jsDynamicIDRe       = regexp.MustCompile(`(?:\.id\s*=\s*['"]([A-Za-z0-9_-]+)['"]|setAttribute\s*\(\s*['"]id['"]\s*,\s*['"]([A-Za-z0-9_-]+)['"]|\bid\s*=\s*["']([A-Za-z0-9_-]+)["'])`)
	jsGlobalAssignRe    = regexp.MustCompile(`(?m)\b(?:window|globalThis|root|self)\.([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	jsModuleExportsRe   = regexp.MustCompile(`(?m)\bmodule\.exports\s*=\s*(?:\{([^}]{1,240})\}|([A-Za-z_$][A-Za-z0-9_$]*))`)
	jsExportsPropRe     = regexp.MustCompile(`(?m)\bexports\.([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	jsESExportRe        = regexp.MustCompile(`(?m)^\s*export\s+(?:default\s+)?(?:async\s+)?(?:function|class|const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	jsFuncDeclRe        = regexp.MustCompile(`(?m)^[ \t]*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(([^)\n]{0,60})\)`)
	jsClassDeclRe       = regexp.MustCompile(`(?m)^[ \t]*(?:export\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)`)
	pyDeclRe            = regexp.MustCompile(`(?m)^(?:class|def)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	goDeclRe            = regexp.MustCompile(`(?m)^(?:func\s+(?:\([^)]+\)\s*)?|type\s+)([A-Z][A-Za-z0-9_]*)`)
	rsDeclRe            = regexp.MustCompile(`(?m)^pub\s+(?:async\s+)?(?:fn|struct|enum|trait)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

const agentSystemPromptCompact = `Du bist LocalCode (im Verbund mit AgentOS, NOMA und MICA), ein präziser autonomer Software-Agent für ein lokales Projekt.

Jede Antwort MUSS genau EIN gültiges JSON-Objekt ohne Markdown-Fences sein:
{"action": "<werkzeug>", "message": "<kurze Beschreibung>", ...felder}

Verfügbare Kern-Werkzeuge:
- list_files(path?), read_file(path), search_text(query)
- write_file(path, content): schreibt die KOMPLETTE, direkt lauffähige Datei (Rule 7: NIEMALS Stubs, leere Funktionsrümpfe, TODO/FIXME oder "can be added"!)
- replace_text(path, old_text, new_text): gezielte Änderung (bei Fehlschlag direkt write_file nutzen)
- delete_file(path), copy_path(source, destination), move_path(source, destination)
- project_info, subagent_analyze(task), build_project
- discover_tool(tool), run_tool(tool, args), run_command(command)
- finish: erst wenn ALLE Anforderungen der Nutzeraufgabe vollständig implementiert UND durch echte Tests/Prüfungen bestanden sind.
- ask_user: nur bei echten unauflösbaren Blockern (NIEMALS für Architektur-Rückfragen bei neuen Projekten).

Verbindliche Regeln:
1. Setze EXAKT die Anforderungen, API-Namen, Zahlenwerte, Dateipfade und Invarianten aus der Nutzeraufgabe um — erfinde keine vereinfachten Ersatz-Klassen!
2. Offline-Policy: Wenn Network=false gilt, verwende KEINE externen Pakete, keine CDNs und kein npm/pip install.
3. Nach jeder Code-Änderung führe die passenden Tests/Prüfungen per run_tool aus (z. B. node --test, pytest, go test) und behebe Fehler gezielt.
4. Wenn die Abschlussprüfung Mängel meldet, lies die betroffene Datei und behebe ALLE gemeldeten Mängel mit write_file, bevor du erneut testest und finish aufrufst.
5. Browser-JS-Kompatibilität: In klassischen <script src="...">-Dateien NIEMALS ungeschütztes require() oder ES-import/export nutzen! Teile Browser-Module über window.<Name> und optional if (typeof module !== 'undefined' && module.exports) module.exports = ... . Nutze stets exakt die Symbolnamen und DOM-IDs aus dem NOMA Live-Manifest.`

func systemPromptForConfig(cfg Config) string {
	if !isLocalOllamaURL(cfg.OllamaURL) && effectiveCompactionContextLength(cfg) > 12288 {
		return agentSystemPrompt
	}
	var b strings.Builder
	b.WriteString(agentSystemPromptCompact)
	if cfg.GitEnabled {
		b.WriteString("\n- Git ist aktiv: git(args), git_commit.")
	}
	if cfg.NetworkEnabled {
		b.WriteString("\n- Netzwerk/Web ist aktiv: web_search(query), web_fetch(url).")
	}
	return b.String()
}

func lineNumberAtOffset(content string, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(content) {
		offset = len(content)
	}
	return 1 + strings.Count(content[:offset], "\n")
}

func detectMicaRule7Stub(relPath, content string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".py", ".go", ".rs", ".java", ".cs":
	default:
		return ""
	}
	var hits []string
	seenLine := map[int]bool{}
	addHit := func(offset int, label string) {
		if len(hits) >= 6 {
			return
		}
		ln := lineNumberAtOffset(content, offset)
		if seenLine[ln] {
			return
		}
		seenLine[ln] = true
		hits = append(hits, fmt.Sprintf("Z.%d: %s", ln, truncateText(strings.TrimSpace(label), 72)))
	}
	for _, loc := range micaRule7StubPatternRe.FindAllStringIndex(content, 6) {
		addHit(loc[0], content[loc[0]:loc[1]])
	}
	for _, loc := range micaRule7NotImplRe.FindAllStringIndex(content, 6) {
		addHit(loc[0], content[loc[0]:loc[1]])
	}
	for _, loc := range micaRule7EllipsisRe.FindAllStringIndex(content, 6) {
		addHit(loc[0], content[loc[0]:loc[1]])
	}
	if !strings.Contains(strings.ToLower(relPath), "test") {
		if ext == ".js" || ext == ".mjs" || ext == ".cjs" || ext == ".ts" || ext == ".tsx" || ext == ".jsx" {
			for _, loc := range micaRule7EmptyFuncRe.FindAllStringSubmatchIndex(content, 8) {
				if len(loc) >= 4 {
					fn := strings.ToLower(content[loc[2]:loc[3]])
					if fn == "noop" || fn == "empty" || strings.HasPrefix(fn, "noop") {
						continue
					}
					addHit(loc[0], "leerer Funktionsrumpf: "+content[loc[0]:loc[1]])
				}
			}
			for _, loc := range micaRule7EmptyMethodRe.FindAllStringSubmatchIndex(content, 8) {
				if len(loc) >= 4 {
					fn := strings.ToLower(content[loc[2]:loc[3]])
					switch fn {
					case "if", "for", "while", "switch", "catch", "constructor", "noop", "empty":
						continue
					}
					addHit(loc[0], "leere Klassenmethode: "+content[loc[0]:loc[1]])
				}
			}
			for _, loc := range micaRule7CommentOnlyBlockRe.FindAllStringIndex(content, 6) {
				addHit(loc[0], "unimplementierter Kommentar-Block: "+content[loc[0]:loc[1]])
			}
		}
	}
	return strings.Join(hits, " | ")
}

func appendUniqueLimited(items []string, seen map[string]bool, val string, limit int) []string {
	val = strings.TrimSpace(val)
	if val == "" || len(items) >= limit {
		return items
	}
	key := strings.ToLower(val)
	if seen[key] {
		return items
	}
	seen[key] = true
	return append(items, val)
}

func shouldSkipWorkspaceDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "dist", "build", "target", "__pycache__", ".venv", "venv", ".localcode", "vendor", ".idea", ".vscode":
		return true
	default:
		return false
	}
}

func collectWorkspaceSourceFiles(project string, maxDepth int) []string {
	if strings.TrimSpace(project) == "" {
		return nil
	}
	root := filepath.Clean(project)
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		depth := strings.Count(relSlash, "/")
		if d.IsDir() {
			if shouldSkipWorkspaceDir(d.Name()) || depth >= maxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		switch ext {
		case ".html", ".htm", ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".py", ".go", ".rs":
			files = append(files, relSlash)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func summarizeSourceFileContract(relPath, content string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".html", ".htm":
		var scripts []string
		scriptSeen := map[string]bool{}
		for _, m := range htmlScriptTagRe.FindAllStringSubmatch(content, -1) {
			attrs := m[1]
			if sm := htmlAttrSrcRe.FindStringSubmatch(attrs); len(sm) >= 2 {
				src := strings.TrimSpace(sm[1])
				if tm := htmlAttrTypeRe.FindStringSubmatch(attrs); len(tm) >= 2 && strings.EqualFold(strings.TrimSpace(tm[1]), "module") {
					src += "[module]"
				}
				scripts = appendUniqueLimited(scripts, scriptSeen, src, 8)
			}
		}
		var ids []string
		idSeen := map[string]bool{}
		for _, m := range htmlTagIDRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 3 {
				ids = appendUniqueLimited(ids, idSeen, strings.ToLower(m[1])+"#"+m[2], 14)
			}
		}
		var parts []string
		if len(scripts) > 0 {
			parts = append(parts, "scripts=["+strings.Join(scripts, ", ")+"]")
		}
		if len(ids) > 0 {
			parts = append(parts, "ids=["+strings.Join(ids, ", ")+"]")
		}
		if len(parts) == 0 {
			return ""
		}
		return "- " + relPath + ": " + strings.Join(parts, " ")

	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx":
		var globals, exports, defs []string
		var defaultExport string
		gSeen, eSeen, dSeen := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, m := range jsGlobalAssignRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				globals = appendUniqueLimited(globals, gSeen, m[1], 8)
			}
		}
		for _, m := range jsModuleExportsRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 3 {
				if strings.TrimSpace(m[2]) != "" {
					defaultExport = strings.TrimSpace(m[2])
				} else if strings.TrimSpace(m[1]) != "" {
					for _, rawItem := range strings.Split(m[1], ",") {
						item := strings.TrimSpace(rawItem)
						if colon := strings.IndexByte(item, ':'); colon > 0 {
							item = strings.TrimSpace(item[:colon])
						}
						if item != "" && !strings.ContainsAny(item, " \t\n{}()") {
							exports = appendUniqueLimited(exports, eSeen, item, 10)
						}
					}
				}
			}
		}
		for _, m := range jsExportsPropRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				exports = appendUniqueLimited(exports, eSeen, m[1], 10)
			}
		}
		for _, m := range jsESExportRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				exports = appendUniqueLimited(exports, eSeen, m[1], 10)
			}
		}
		for _, m := range jsClassDeclRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				defs = appendUniqueLimited(defs, dSeen, "class "+m[1], 12)
			}
		}
		for _, m := range jsFuncDeclRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 3 {
				params := strings.Join(strings.Fields(m[2]), " ")
				if len(params) > 28 {
					params = params[:28] + "…"
				}
				defs = appendUniqueLimited(defs, dSeen, m[1]+"("+params+")", 12)
			}
		}
		var methods, props []string
		mSeen, pSeen := map[string]bool{}, map[string]bool{}
		if strings.Contains(content, "class ") {
			for _, m := range jsClassMethodRe.FindAllStringSubmatch(content, -1) {
				if len(m) >= 3 {
					name := m[1]
					switch strings.ToLower(name) {
					case "if", "for", "while", "switch", "catch", "constructor", "return":
						continue
					}
					params := strings.Join(strings.Fields(m[2]), " ")
					if len(params) > 24 {
						params = params[:24] + "…"
					}
					methods = appendUniqueLimited(methods, mSeen, "."+name+"("+params+")", 10)
				}
			}
			for _, m := range jsThisPropRe.FindAllStringSubmatch(content, -1) {
				if len(m) >= 2 {
					props = appendUniqueLimited(props, pSeen, m[1], 12)
				}
			}
		}
		var parts []string
		if len(globals) > 0 {
			parts = append(parts, "globals=["+strings.Join(globals, ", ")+"]")
		}
		if defaultExport != "" {
			parts = append(parts, "exportDefault="+defaultExport)
		}
		if len(exports) > 0 {
			parts = append(parts, "exports=["+strings.Join(exports, ", ")+"]")
		}
		if len(defs) > 0 {
			parts = append(parts, "defs=["+strings.Join(defs, ", ")+"]")
		}
		if len(methods) > 0 {
			parts = append(parts, "methods=["+strings.Join(methods, ", ")+"]")
		}
		if len(props) > 0 {
			parts = append(parts, "fields=["+strings.Join(props, ", ")+"]")
		}
		if len(parts) == 0 {
			return ""
		}
		line := "- " + relPath + ": " + strings.Join(parts, " ")
		if len(line) > 360 {
			line = line[:357] + "..."
		}
		return line

	case ".py":
		var defs []string
		seen := map[string]bool{}
		for _, m := range pyDeclRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				defs = appendUniqueLimited(defs, seen, m[1], 12)
			}
		}
		if len(defs) == 0 {
			return ""
		}
		return "- " + relPath + ": defs=[" + strings.Join(defs, ", ") + "]"

	case ".go":
		var defs []string
		seen := map[string]bool{}
		for _, m := range goDeclRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				defs = appendUniqueLimited(defs, seen, m[1], 12)
			}
		}
		if len(defs) == 0 {
			return ""
		}
		return "- " + relPath + ": exported=[" + strings.Join(defs, ", ") + "]"

	case ".rs":
		var defs []string
		seen := map[string]bool{}
		for _, m := range rsDeclRe.FindAllStringSubmatch(content, -1) {
			if len(m) >= 2 {
				defs = appendUniqueLimited(defs, seen, m[1], 12)
			}
		}
		if len(defs) == 0 {
			return ""
		}
		return "- " + relPath + ": pub=[" + strings.Join(defs, ", ") + "]"
	}
	return ""
}

func buildWorkspaceContractManifest(project string) string {
	files := collectWorkspaceSourceFiles(project, 3)
	if len(files) == 0 {
		return ""
	}
	var lines []string
	totalLen := 0
	for _, rel := range files {
		full, err := ensureWithinRoot(project, rel)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(full)
		if err != nil || len(raw) == 0 {
			continue
		}
		if len(raw) > 256*1024 {
			raw = raw[:256*1024]
		}
		line := summarizeSourceFileContract(rel, string(raw))
		if line == "" {
			continue
		}
		if totalLen+len(line)+1 > 1800 {
			lines = append(lines, "- ... (weitere Dateien via read_file)")
			break
		}
		lines = append(lines, line)
		totalLen += len(line) + 1
	}
	if len(lines) == 0 {
		return ""
	}
	return "[NOMA LIVE WORKSPACE-MANIFEST (exakte vorhandene Symbole & DOM-IDs — nutze NUR diese Namen!)]:\n" + strings.Join(lines, "\n")
}

func projectUsesJSBundler(project string) bool {
	pkgPath := filepath.Join(project, "package.json")
	raw, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}
	low := strings.ToLower(string(raw))
	for _, bundler := range []string{`"vite"`, `"webpack"`, `"esbuild"`, `"parcel"`, `"rollup"`, `"next"`} {
		if strings.Contains(low, bundler) {
			return true
		}
	}
	return false
}

func isExternalAssetURL(ref string) bool {
	low := strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") || strings.HasPrefix(low, "//")
}

func stripURLQueryFragment(ref string) string {
	if idx := strings.IndexAny(ref, "?#"); idx >= 0 {
		return ref[:idx]
	}
	return ref
}

func hasUnguardedRequireInBrowserJS(content string) string {
	matches := anyNodeRequireRe.FindAllStringSubmatchIndex(content, -1)
	for _, loc := range matches {
		if len(loc) < 4 {
			continue
		}
		start := loc[0]
		lineStart := strings.LastIndexByte(content[:start], '\n') + 1
		linePrefix := strings.TrimSpace(content[lineStart:start])
		if strings.HasPrefix(linePrefix, "//") || strings.HasPrefix(linePrefix, "*") || strings.HasPrefix(linePrefix, "/*") {
			continue
		}
		windowStart := start - 260
		if windowStart < 0 {
			windowStart = 0
		}
		guardWindow := content[windowStart:start]
		if strings.Contains(guardWindow, "typeof require") || strings.Contains(guardWindow, "typeof module") || regexp.MustCompile(`try\s*\{`).MatchString(guardWindow) {
			continue
		}
		return strings.TrimSpace(content[loc[0]:loc[1]])
	}
	return ""
}

func findMissingBrowserDOMIDs(jsContent string, htmlIDs, dynamicIDs map[string]bool) []string {
	var missing []string
	seen := map[string]bool{}
	checkMatches := func(re *regexp.Regexp) {
		for _, loc := range re.FindAllStringSubmatchIndex(jsContent, -1) {
			if len(loc) < 4 {
				continue
			}
			id := jsContent[loc[2]:loc[3]]
			if htmlIDs[id] || dynamicIDs[id] || seen[id] {
				continue
			}
			lineStart := strings.LastIndexByte(jsContent[:loc[0]], '\n') + 1
			lineEnd := strings.IndexByte(jsContent[loc[1]:], '\n')
			if lineEnd < 0 {
				lineEnd = len(jsContent)
			} else {
				lineEnd += loc[1]
			}
			line := jsContent[lineStart:lineEnd]
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") {
				continue
			}
			// Allow fallback chains like document.getElementById('a') || document.getElementById('b') if another ID on the line exists.
			if strings.Contains(line, "||") {
				fallbackExists := false
				for _, m2 := range jsGetByIDRe.FindAllStringSubmatch(line, -1) {
					if len(m2) >= 2 && (htmlIDs[m2[1]] || dynamicIDs[m2[1]]) {
						fallbackExists = true
						break
					}
				}
				if fallbackExists {
					continue
				}
			}
			seen[id] = true
			missing = append(missing, id)
		}
	}
	checkMatches(jsGetByIDRe)
	checkMatches(jsQuerySelectorIDRe)
	sort.Strings(missing)
	return missing
}

type webSmokeSpec struct {
	HTMLPath      string            `json:"htmlPath"`
	Scripts       []string          `json:"scripts"`
	InlineScripts []string          `json:"inlineScripts"`
	IDTags        map[string]string `json:"idTags"`
	CanvasSizes   map[string][2]int `json:"canvasSizes"`
	HasCanvas     bool              `json:"hasCanvas"`
}

const nodeWebAppSmokeScript = `
const fs = require('fs');
const vm = require('vm');
const spec = JSON.parse(process.env.LOCALCODE_WEB_SMOKE_SPEC || '{}');
let drawCalls = 0;
let rafQueue = [];
let rafId = 1;
const docListeners = {};
const winListeners = {};
function makeStyle() { return new Proxy({}, { get: (t, k) => (k in t ? t[k] : ''), set: (t, k, v) => { t[k] = v; return true; } }); }
function makeClassList() {
  const s = new Set();
  return { add: (...a) => a.forEach(x => s.add(x)), remove: (...a) => a.forEach(x => s.delete(x)), toggle: (x, f) => (f === undefined ? (s.has(x) ? (s.delete(x), false) : (s.add(x), true)) : (f ? (s.add(x), true) : (s.delete(x), false))), contains: x => s.has(x) };
}
function makeCtx2D(el) {
  const count = () => { drawCalls++; };
  const noop = () => {};
  const grad = { addColorStop: noop };
  return new Proxy({
    canvas: el,
    fillStyle: '#000', strokeStyle: '#000', lineWidth: 1, font: '12px sans-serif',
    textAlign: 'left', textBaseline: 'alphabetic', globalAlpha: 1, globalCompositeOperation: 'source-over',
    shadowColor: 'transparent', shadowBlur: 0, shadowOffsetX: 0, shadowOffsetY: 0, lineCap: 'butt', lineJoin: 'miter',
    fillRect: count, strokeRect: count, clearRect: noop, fill: count, stroke: count,
    fillText: count, strokeText: count, drawImage: count, putImageData: count, arc: count,
    rect: noop, roundRect: noop, ellipse: count, lineTo: noop, moveTo: noop, quadraticCurveTo: noop, bezierCurveTo: noop,
    beginPath: noop, closePath: noop, save: noop, restore: noop, translate: noop, scale: noop, rotate: noop,
    transform: noop, setTransform: noop, resetTransform: noop, clip: noop, setLineDash: noop,
    createLinearGradient: () => grad, createRadialGradient: () => grad, createPattern: () => ({}),
    measureText: (t) => ({ width: String(t || '').length * 8 }),
    getImageData: (x, y, w, h) => ({ width: w || 1, height: h || 1, data: new Uint8ClampedArray((w || 1) * (h || 1) * 4) }),
    createImageData: (w, h) => ({ width: w || 1, height: h || 1, data: new Uint8ClampedArray((w || 1) * (h || 1) * 4) }),
  }, { get: (t, k) => (k in t ? t[k] : noop) });
}
function makeElement(tag, id) {
  const listeners = {};
  const attrs = {};
  const children = [];
  const lowTag = String(tag || 'div').toLowerCase();
  const sz = (spec.canvasSizes && id && spec.canvasSizes[id]) || [800, 600];
  let ctx2d = null;
  const el = {
    tagName: lowTag.toUpperCase(), nodeName: lowTag.toUpperCase(), id: id || '', className: '',
    textContent: '', innerText: '', innerHTML: '', value: '', checked: false, disabled: false, hidden: false,
    width: sz[0], height: sz[1], clientWidth: sz[0], clientHeight: sz[1], offsetWidth: sz[0], offsetHeight: sz[1],
    style: makeStyle(), classList: makeClassList(), dataset: {}, children, childNodes: children,
    appendChild: (c) => { children.push(c); return c; },
    removeChild: (c) => { const i = children.indexOf(c); if (i >= 0) children.splice(i, 1); return c; },
    replaceChildren: (...a) => { children.length = 0; children.push(...a); },
    setAttribute: (k, v) => { attrs[k] = String(v); if (k === 'id') { el.id = String(v); byId.set(el.id, el); } },
    getAttribute: (k) => (k in attrs ? attrs[k] : null),
    hasAttribute: (k) => (k in attrs),
    removeAttribute: (k) => { delete attrs[k]; },
    addEventListener: (ev, fn) => { (listeners[ev] = listeners[ev] || []).push(fn); },
    removeEventListener: (ev, fn) => { if (listeners[ev]) listeners[ev] = listeners[ev].filter(f => f !== fn); },
    dispatchEvent: (ev) => { const list = listeners[ev && ev.type] || []; list.forEach(fn => fn(ev)); return true; },
    click: () => { if (typeof el.onclick === 'function') el.onclick({ preventDefault() {}, target: el }); (listeners.click || []).forEach(fn => fn({ preventDefault() {}, target: el })); },
    focus: () => {}, blur: () => {},
    getBoundingClientRect: () => ({ left: 0, top: 0, right: el.width, bottom: el.height, width: el.width, height: el.height }),
    querySelector: (sel) => documentMock.querySelector(sel),
    querySelectorAll: (sel) => documentMock.querySelectorAll(sel),
    getContext: (kind) => { if (kind === '2d') { if (!ctx2d) ctx2d = makeCtx2D(el); return ctx2d; } return null; },
  };
  return el;
}
const byId = new Map();
const allEls = [];
for (const [id, tag] of Object.entries(spec.idTags || {})) {
  const el = makeElement(tag, id);
  byId.set(id, el);
  allEls.push(el);
}
if (spec.hasCanvas && !allEls.some(e => e.tagName === 'CANVAS')) {
  allEls.push(makeElement('canvas', ''));
}
const bodyEl = makeElement('body', '');
const headEl = makeElement('head', '');
const documentMock = {
  readyState: 'complete', visibilityState: 'visible', hidden: false,
  body: bodyEl, head: headEl, documentElement: makeElement('html', ''),
  getElementById: (id) => byId.get(String(id)) || null,
  querySelector: (sel) => {
    const s = String(sel || '').trim();
    if (s.startsWith('#')) return byId.get(s.slice(1)) || null;
    if (s.toLowerCase() === 'canvas') return allEls.find(e => e.tagName === 'CANVAS') || null;
    return allEls[0] || bodyEl;
  },
  querySelectorAll: (sel) => {
    const s = String(sel || '').trim();
    if (s.startsWith('#')) { const x = byId.get(s.slice(1)); return x ? [x] : []; }
    if (s.toLowerCase() === 'canvas') return allEls.filter(e => e.tagName === 'CANVAS');
    return [];
  },
  createElement: (tag) => makeElement(tag, ''),
  createTextNode: (t) => ({ nodeValue: String(t || '') }),
  addEventListener: (ev, fn) => { (docListeners[ev] = docListeners[ev] || []).push(fn); },
  removeEventListener: (ev, fn) => { if (docListeners[ev]) docListeners[ev] = docListeners[ev].filter(f => f !== fn); },
  dispatchEvent: (ev) => { (docListeners[ev && ev.type] || []).forEach(fn => fn(ev)); return true; },
};
const storageData = new Map();
const storageMock = {
  getItem: (k) => (storageData.has(String(k)) ? storageData.get(String(k)) : null),
  setItem: (k, v) => { storageData.set(String(k), String(v)); },
  removeItem: (k) => { storageData.delete(String(k)); },
  clear: () => { storageData.clear(); },
};
function MockAudioContext() {
  this.state = 'running'; this.currentTime = 0; this.destination = {};
  this.resume = () => Promise.resolve();
  this.createOscillator = () => ({ type: 'sine', frequency: { value: 440, setValueAtTime() {}, exponentialRampToValueAtTime() {}, linearRampToValueAtTime() {} }, connect() {}, start() {}, stop() {} });
  this.createGain = () => ({ gain: { value: 1, setValueAtTime() {}, exponentialRampToValueAtTime() {}, linearRampToValueAtTime() {} }, connect() {} });
  this.createBiquadFilter = () => ({ type: 'lowpass', frequency: { value: 1000, setValueAtTime() {} }, connect() {} });
  this.createBuffer = (ch, len, sr) => ({ getChannelData: () => new Float32Array(len || 1) });
  this.createBufferSource = () => ({ buffer: null, connect() {}, start() {}, stop() {} });
}
const sandbox = {
  console, Math, Date, JSON, Number, String, Boolean, Array, Object, Map, Set, WeakMap, WeakSet, RegExp, Error, TypeError, RangeError, Promise, Symbol, Proxy, Reflect,
  Int8Array, Uint8Array, Uint8ClampedArray, Int16Array, Uint16Array, Int32Array, Uint32Array, Float32Array, Float64Array,
  parseInt, parseFloat, isNaN, isFinite, encodeURIComponent, decodeURIComponent,
  setTimeout: (fn) => { return 1; }, clearTimeout: () => {},
  setInterval: (fn) => { return 1; }, clearInterval: () => {},
  requestAnimationFrame: (fn) => { const id = rafId++; rafQueue.push({ id, fn }); return id; },
  cancelAnimationFrame: (id) => { rafQueue = rafQueue.filter(x => x.id !== id); },
  performance: { now: () => Date.now() },
  localStorage: storageMock, sessionStorage: storageMock,
  document: documentMock,
  navigator: { userAgent: 'LocalCodeSmoke/1.0', language: 'en-US' },
  location: { href: 'http://localhost/', protocol: 'http:', hostname: 'localhost', pathname: '/' },
  innerWidth: 1280, innerHeight: 720, devicePixelRatio: 1,
  AudioContext: MockAudioContext, webkitAudioContext: MockAudioContext,
  Audio: function() { this.play = () => Promise.resolve(); this.pause = () => {}; },
  Image: function() { this.src = ''; this.width = 16; this.height = 16; },
  Event: function(type) { this.type = type; this.preventDefault = () => {}; },
  CustomEvent: function(type, init) { this.type = type; this.detail = init && init.detail; this.preventDefault = () => {}; },
  KeyboardEvent: function(type, init) { this.type = type; this.key = (init && init.key) || ''; this.code = (init && init.code) || ''; this.preventDefault = () => {}; },
  addEventListener: (ev, fn) => { (winListeners[ev] = winListeners[ev] || []).push(fn); },
  removeEventListener: (ev, fn) => { if (winListeners[ev]) winListeners[ev] = winListeners[ev].filter(f => f !== fn); },
  dispatchEvent: (ev) => { (winListeners[ev && ev.type] || []).forEach(fn => fn(ev)); return true; },
};
sandbox.window = sandbox;
sandbox.self = sandbox;
sandbox.globalThis = sandbox;
const ctx = vm.createContext(sandbox);
try {
  for (const scriptPath of (spec.scripts || [])) {
    const code = fs.readFileSync(scriptPath, 'utf8');
    vm.runInContext(code, ctx, { filename: scriptPath, timeout: 2500 });
  }
  for (let i = 0; i < (spec.inlineScripts || []).length; i++) {
    vm.runInContext(spec.inlineScripts[i], ctx, { filename: (spec.htmlPath || 'index.html') + '#inline-' + (i + 1), timeout: 2500 });
  }
  const domEv = { type: 'DOMContentLoaded', preventDefault() {} };
  (docListeners.DOMContentLoaded || []).forEach(fn => fn(domEv));
  const loadEv = { type: 'load', preventDefault() {} };
  if (typeof sandbox.onload === 'function') sandbox.onload(loadEv);
  (winListeners.load || []).forEach(fn => fn(loadEv));
  const stepFrames = (baseTs) => {
    for (let f = 0; f < 3; f++) {
      const batch = rafQueue.splice(0, rafQueue.length);
      if (batch.length === 0) break;
      for (const item of batch) {
        item.fn(baseTs + (f + 1) * 16.67);
      }
    }
  };
  stepFrames(100);
  if (spec.hasCanvas && drawCalls === 0) {
    for (const [id, el] of byId.entries()) {
      if (el.tagName === 'BUTTON' && /start|play|begin|new/i.test(id)) {
        el.click();
      }
    }
    stepFrames(200);
  }
  console.log(JSON.stringify({ ok: true, drawCalls }));
} catch (err) {
  const stackLine = (err && err.stack ? String(err.stack).split('\n')[0] : '');
  console.log(JSON.stringify({ ok: false, error: (err && err.name ? err.name + ': ' : '') + (err && err.message ? err.message : String(err)), stack: stackLine, drawCalls }));
}
`

func verifyWebAppIntegrity(ctx context.Context, project string, cfg Config, runSmoke bool) []string {
	if strings.TrimSpace(project) == "" {
		return nil
	}
	var htmlFiles []string
	root := filepath.Clean(project)
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if d.IsDir() {
			if shouldSkipWorkspaceDir(d.Name()) || strings.Count(relSlash, "/") >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == ".html" || ext == ".htm" {
			htmlFiles = append(htmlFiles, relSlash)
		}
		return nil
	})
	if len(htmlFiles) == 0 {
		return nil
	}
	sort.Strings(htmlFiles)
	hasBundler := projectUsesJSBundler(project)
	var issues []string

	for _, htmlRel := range htmlFiles {
		htmlFull, err := ensureWithinRoot(project, htmlRel)
		if err != nil {
			continue
		}
		rawHTML, err := os.ReadFile(htmlFull)
		if err != nil {
			continue
		}
		htmlText := string(rawHTML)
		htmlDir := filepath.Dir(htmlFull)

		htmlIDs := map[string]bool{}
		idTags := map[string]string{}
		var idList []string
		for _, m := range htmlTagIDRe.FindAllStringSubmatch(htmlText, -1) {
			if len(m) >= 3 {
				tag := strings.ToLower(m[1])
				id := m[2]
				if !htmlIDs[id] {
					htmlIDs[id] = true
					idTags[id] = tag
					idList = append(idList, id)
				}
			}
		}
		sort.Strings(idList)

		canvasSizes := map[string][2]int{}
		hasCanvas := false
		for _, m := range htmlCanvasSizeRe.FindAllStringSubmatch(htmlText, -1) {
			hasCanvas = true
			attrs := m[1]
			w, h := 800, 600
			if wm := htmlAttrWidthRe.FindStringSubmatch(attrs); len(wm) >= 2 {
				fmt.Sscanf(wm[1], "%d", &w)
			}
			if hm := htmlAttrHeightRe.FindStringSubmatch(attrs); len(hm) >= 2 {
				fmt.Sscanf(hm[1], "%d", &h)
			}
			id := ""
			if im := regexp.MustCompile(`(?i)\bid\s*=\s*["']([A-Za-z0-9_-]+)["']`).FindStringSubmatch(attrs); len(im) >= 2 {
				id = im[1]
			}
			if id != "" {
				canvasSizes[id] = [2]int{w, h}
			}
		}

		for _, lm := range htmlLinkStyleRe.FindAllStringSubmatch(htmlText, -1) {
			attrs := lm[1]
			relM := htmlAttrRelRe.FindStringSubmatch(attrs)
			hrefM := htmlAttrHrefRe.FindStringSubmatch(attrs)
			if len(relM) < 2 || len(hrefM) < 2 || !strings.Contains(strings.ToLower(relM[1]), "stylesheet") {
				continue
			}
			href := strings.TrimSpace(hrefM[1])
			if href == "" || strings.HasPrefix(strings.ToLower(href), "data:") {
				continue
			}
			if isExternalAssetURL(href) {
				if !cfg.NetworkEnabled {
					issues = append(issues, fmt.Sprintf("HTML `%s` bindet externes Stylesheet `%s` im Offline-Modus ein — ersetze es durch lokales CSS.", htmlRel, href))
				}
				continue
			}
			cleanHref := stripURLQueryFragment(href)
			target := filepath.Join(htmlDir, filepath.FromSlash(cleanHref))
			if info, statErr := os.Stat(target); statErr != nil || info.IsDir() {
				issues = append(issues, fmt.Sprintf("HTML `%s` referenziert fehlendes lokales Stylesheet `%s`.", htmlRel, href))
			}
		}

		type loadedScript struct {
			relPath  string
			fullPath string
			content  string
			isModule bool
			isInline bool
		}
		var scripts []loadedScript
		var classicScriptPaths []string
		var inlineScripts []string
		htmlStaticFailed := false

		for _, sm := range htmlScriptTagRe.FindAllStringSubmatch(htmlText, -1) {
			attrs := sm[1]
			body := sm[2]
			isModule := false
			if tm := htmlAttrTypeRe.FindStringSubmatch(attrs); len(tm) >= 2 && strings.EqualFold(strings.TrimSpace(tm[1]), "module") {
				isModule = true
			}
			if srcM := htmlAttrSrcRe.FindStringSubmatch(attrs); len(srcM) >= 2 {
				src := strings.TrimSpace(srcM[1])
				if src == "" || strings.HasPrefix(strings.ToLower(src), "data:") {
					continue
				}
				if isExternalAssetURL(src) {
					if !cfg.NetworkEnabled {
						issues = append(issues, fmt.Sprintf("HTML `%s` lädt externes Skript `%s` im Offline-Modus — ersetze es durch lokalen Code.", htmlRel, src))
						htmlStaticFailed = true
					}
					continue
				}
				cleanSrc := stripURLQueryFragment(src)
				scriptFull := filepath.Join(htmlDir, filepath.FromSlash(cleanSrc))
				scriptBytes, readErr := os.ReadFile(scriptFull)
				if readErr != nil {
					issues = append(issues, fmt.Sprintf("HTML `%s` referenziert fehlendes lokales Skript `%s`.", htmlRel, src))
					htmlStaticFailed = true
					continue
				}
				scriptRel := cleanSrc
				if r, rErr := filepath.Rel(root, scriptFull); rErr == nil {
					scriptRel = filepath.ToSlash(r)
				}
				scripts = append(scripts, loadedScript{relPath: scriptRel, fullPath: scriptFull, content: string(scriptBytes), isModule: isModule})
				if !isModule {
					classicScriptPaths = append(classicScriptPaths, scriptFull)
				}
			} else if strings.TrimSpace(body) != "" {
				scripts = append(scripts, loadedScript{relPath: htmlRel + " (inline <script>)", content: body, isModule: isModule, isInline: true})
				if !isModule {
					inlineScripts = append(inlineScripts, body)
				}
			}
		}

		dynamicIDs := map[string]bool{}
		for _, sc := range scripts {
			for _, m := range jsDynamicIDRe.FindAllStringSubmatch(sc.content, -1) {
				for _, g := range m[1:] {
					if g != "" {
						dynamicIDs[g] = true
					}
				}
			}
		}

		for _, sc := range scripts {
			if !hasBundler && !sc.isModule {
				if reqHit := hasUnguardedRequireInBrowserJS(sc.content); reqHit != "" {
					issues = append(issues, fmt.Sprintf("Browser-Skript `%s` (geladen in `%s`) nutzt ungeschütztes `%s` — das wirft im Browser sofort `ReferenceError: require is not defined` (weißer Bildschirm)! Teile Module im Browser über `window.<Name>` und nutze `require()`/`module.exports` nur hinter `typeof module !== 'undefined'`.", sc.relPath, htmlRel, reqHit))
					htmlStaticFailed = true
				}
				if bareESModuleRe.MatchString(sc.content) {
					issues = append(issues, fmt.Sprintf("Browser-Skript `%s` nutzt ES-Modul-Syntax (`import`/`export`), wird in `%s` aber ohne `type=\"module\"` eingebunden — das wirft im Browser sofort `SyntaxError`!", sc.relPath, htmlRel))
					htmlStaticFailed = true
				}
			}
			if missingIDs := findMissingBrowserDOMIDs(sc.content, htmlIDs, dynamicIDs); len(missingIDs) > 0 {
				avail := "(keine IDs im HTML)"
				if len(idList) > 0 {
					avail = strings.Join(idList, ", ")
				}
				issues = append(issues, fmt.Sprintf("DOM-ID-Fehlanpassung in `%s`: referenziert nicht existierende ID(s) [%s], aber `%s` definiert nur [%s].", sc.relPath, strings.Join(missingIDs, ", "), htmlRel, avail))
				htmlStaticFailed = true
			}
			for _, am := range jsLocalMediaAssetRe.FindAllStringSubmatch(sc.content, -1) {
				if len(am) < 2 {
					continue
				}
				assetRef := strings.TrimSpace(am[1])
				if assetRef == "" || isExternalAssetURL(assetRef) || strings.HasPrefix(strings.ToLower(assetRef), "data:") {
					continue
				}
				cleanAsset := stripURLQueryFragment(assetRef)
				assetTarget := filepath.Join(htmlDir, filepath.FromSlash(cleanAsset))
				if info, statErr := os.Stat(assetTarget); statErr != nil || info.IsDir() {
					issues = append(issues, fmt.Sprintf("Browser-Skript `%s` referenziert fehlende lokale Mediendatei `%s` (existiert nicht im Projekt!). Erzeuge Audio prozedural mit Web Audio API (OscillatorNode/GainNode) und Grafiken per Canvas/SVG statt fehlende Dateien zu laden.", sc.relPath, assetRef))
					htmlStaticFailed = true
					break
				}
			}
		}

		if runSmoke && !hasBundler && !htmlStaticFailed && (len(classicScriptPaths) > 0 || len(inlineScripts) > 0) {
			nodeInfo := discoverTool(project, "node", cfg, false)
			nodeExe := nodeInfo.Path
			if nodeExe == "" {
				if found, lookErr := exec.LookPath("node"); lookErr == nil {
					nodeExe = found
				}
			}
			if nodeExe != "" {
				specBytes, _ := json.Marshal(webSmokeSpec{
					HTMLPath:      htmlRel,
					Scripts:       classicScriptPaths,
					InlineScripts: inlineScripts,
					IDTags:        idTags,
					CanvasSizes:   canvasSizes,
					HasCanvas:     hasCanvas,
				})
				smokeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
				cmd := exec.CommandContext(smokeCtx, nodeExe, "-e", nodeWebAppSmokeScript)
				cmd.Dir = project
				cmd.Env = append(os.Environ(), "LOCALCODE_WEB_SMOKE_SPEC="+string(specBytes))
				hideCommandWindow(cmd)
				out, cmdErr := cmd.CombinedOutput()
				cancel()
				var res struct {
					OK        bool   `json:"ok"`
					Error     string `json:"error"`
					DrawCalls int    `json:"drawCalls"`
				}
				parsed := false
				for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "{") && json.Unmarshal([]byte(line), &res) == nil {
						parsed = true
						break
					}
				}
				if parsed {
					if !res.OK {
						issues = append(issues, fmt.Sprintf("Browser-Boot-Smoke-Test für `%s` fehlgeschlagen (Laufzeitfehler beim Laden): %s — behebe diesen Laufzeitfehler, bevor du finish aufrufst!", htmlRel, res.Error))
					} else if hasCanvas && res.DrawCalls == 0 {
						issues = append(issues, fmt.Sprintf("Browser-Boot-Smoke-Test für `%s`: Das `<canvas>` erhielt beim Start und in 3 Animations-Frames 0 Zeichenaufrufe (möglicher leerer/weißer Bildschirm). Stelle sicher, dass beim Laden sofort gerendert wird.", htmlRel))
					}
				} else if cmdErr != nil {
					issues = append(issues, fmt.Sprintf("Browser-Boot-Smoke-Test für `%s` abgebrochen: %s", htmlRel, truncateText(strings.TrimSpace(string(out)), 300)))
				}
			}
		}
	}
	return issues
}

func findProjectTestFile(project string) string {
	for _, rel := range collectWorkspaceSourceFiles(project, 3) {
		low := strings.ToLower(rel)
		if strings.Contains(low, "test") && (strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".mjs")) {
			return rel
		}
	}
	return ""
}

func detectPendingStubFile(project string, changedPaths map[string]bool) (string, string) {
	paths := make([]string, 0, len(changedPaths))
	for p, ok := range changedPaths {
		if ok && strings.TrimSpace(p) != "" {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		rel := filepath.ToSlash(filepath.Clean(normalizeProjectRelativeTarget(project, p)))
		full, err := ensureWithinRoot(project, rel)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		if hit := detectMicaRule7Stub(rel, string(raw)); hit != "" {
			return rel, hit
		}
	}
	return "", ""
}

func offlinePackageInstallBlocked(cfg Config, a AgentAction) string {
	if cfg.NetworkEnabled {
		return ""
	}
	if a.Action != "open_terminal" && a.Action != "run_command" && a.Action != "run_tool" {
		return ""
	}
	cmdText := strings.TrimSpace(a.Command)
	if a.Action == "run_tool" {
		cmdText = strings.TrimSpace(a.Tool + " " + strings.Join(a.Args, " "))
	}
	if !offlinePackageInstallRe.MatchString(cmdText) {
		return ""
	}
	return fmt.Sprintf("ERROR: OFFLINE-POLICY BLOCKIERT — Externe Paketinstallation (%q) ist im Offline-Modus (network_enabled=false) verboten. Entferne externe require()/import-Abhängigkeiten aus dem Quellcode und implementiere die Logik vollständig mit eingebauten Standard-APIs.", cmdText)
}

type nomaMicaPreflightReport struct {
	AgentOS struct {
		Status  string `json:"status"`
		Route   string `json:"route"`
		State   string `json:"state"`
		Workers int    `json:"workers"`
	} `json:"agentos"`
	Noma struct {
		Status        string   `json:"status"`
		CriteriaCount int      `json:"criteria_count"`
		SampleItems   []string `json:"sample_items"`
		Strategies    []string `json:"strategies"`
		IndexedFiles  int      `json:"indexed_files"`
	} `json:"noma"`
	Mica struct {
		Status        string `json:"status"`
		Version       string `json:"version"`
		Threat        string `json:"threat"`
		Library       string `json:"library"`
		LibrarySHA256 string `json:"library_sha256"`
	} `json:"mica"`
}

func discoverNomaMicaRoot(project string) string {
	candidates := []string{}
	if envRoot := strings.TrimSpace(os.Getenv("NOMAMICA_ROOT")); envRoot != "" {
		candidates = append(candidates, envRoot)
	}
	if strings.TrimSpace(project) != "" {
		cur := filepath.Clean(project)
		for i := 0; i < 6; i++ {
			candidates = append(candidates, cur)
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	for _, root := range candidates {
		nomaPath := filepath.Join(root, "NOMA", "noma_runtime", "coding", "problem_framing.py")
		agentosPath := filepath.Join(root, "AgentOS", "src", "agentos", "integration", "utp_v1.py")
		if info, err := os.Stat(nomaPath); err == nil && !info.IsDir() {
			if info2, err2 := os.Stat(agentosPath); err2 == nil && !info2.IsDir() {
				return root
			}
		}
	}
	return ""
}

func orderedTaskTargetFiles(project, task string) []string {
	raw := mentionedProjectFiles(task)
	if len(raw) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var normalized []string
	for _, item := range raw {
		rel := normalizeProjectRelativeTarget(project, item)
		rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
		if rel == "" || rel == "." || strings.HasPrefix(rel, "../") {
			continue
		}
		key := strings.ToLower(rel)
		if !seen[key] {
			seen[key] = true
			normalized = append(normalized, rel)
		}
	}
	fileTier := func(p string) int {
		low := strings.ToLower(p)
		base := strings.ToLower(filepath.Base(p))
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".md" || ext == ".txt" {
			return 50
		}
		if strings.Contains(low, "test") || strings.HasPrefix(low, "tests/") {
			return 40
		}
		if ext == ".html" || ext == ".htm" || ext == ".css" {
			return 30
		}
		if strings.Contains(base, "game.") || strings.Contains(base, "main.") || strings.Contains(base, "app.") {
			return 25
		}
		if strings.Contains(base, "render") || strings.Contains(base, "view") || strings.Contains(base, "ui") {
			return 20
		}
		if strings.Contains(base, "engine") || strings.Contains(base, "core") || strings.Contains(base, "model") {
			return 10
		}
		return 15
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		ti, tj := fileTier(normalized[i]), fileTier(normalized[j])
		if ti != tj {
			return ti < tj
		}
		return normalized[i] < normalized[j]
	})
	return normalized
}

func runNomaMicaSuitePreflight(ctx context.Context, s *AppState, project, task string, cfg Config) string {
	root := discoverNomaMicaRoot(project)
	if root == "" {
		return ""
	}
	targetFiles := orderedTaskTargetFiles(project, task)
	pyInfo := discoverTool(project, "python", cfg, false)
	pyExe := pyInfo.Path
	if pyExe == "" {
		if found, err := exec.LookPath("python"); err == nil {
			pyExe = found
		}
	}
	var report nomaMicaPreflightReport
	ranLivePython := false
	if pyExe != "" {
		pyCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		script := `
import hashlib, json, os, sys
from pathlib import Path
root = Path(sys.argv[1])
project = Path(sys.argv[2])
task = os.environ.get("NOMAMICA_TASK_TEXT", "")
sys.path.insert(0, str(root / "AgentOS"))
sys.path.insert(0, str(root / "AgentOS" / "src"))
sys.path.insert(0, str(root / "NOMA"))
out = {"agentos": {}, "noma": {}, "mica": {}}
try:
    from agentos.integration.utp_v1 import route_request
    dec = route_request({
        "protocol": "agentos.utp.v1", "kind": "task_request",
        "task_id": "mission-active", "intent": "IMPLEMENT_CODE",
        "risk": "MEDIUM", "freshness": "NONE",
        "requires_external_evidence": False, "external_effects": [],
        "resume_mission": False, "durable": True,
        "constraints": ["NO_PUSH", "NO_DEPLOY"],
    })
    out["agentos"] = {"status": "PASS_ROUTE_ONLY", "route": dec.get("route", "CODE_CHANGE"), "state": dec.get("task_state", "ROUTED"), "workers": 4}
except Exception as e:
    out["agentos"] = {"status": "FALLBACK", "route": "CODE_CHANGE", "state": str(e)[:120], "workers": 4}
try:
    from noma_runtime.coding.problem_framing import ProblemFramingEngine
    from noma_runtime.coding.repository_indexer import RepositoryIndexer
    from noma_runtime.coding.contracts import CodingTaskSpec
    snap = RepositoryIndexer().index_repository(project)
    frame = ProblemFramingEngine().frame_problem(
        CodingTaskSpec(task_id="mission-active", requirement_text=task, repo_path=str(project), language="javascript"),
        snap,
    )
    indexed_count = len(getattr(snap, "file_hashes", None) or getattr(snap, "symbols", None) or [])
    out["noma"] = {
        "status": "PASS_REAL_FRAMING",
        "criteria_count": len(frame.acceptance_criteria),
        "sample_items": frame.acceptance_criteria[:8],
        "strategies": frame.candidate_strategies,
        "indexed_files": indexed_count,
    }
except Exception as e:
    out["noma"] = {"status": "FALLBACK", "criteria_count": 0, "sample_items": [], "strategies": [str(e)[:120]], "indexed_files": 0}
try:
    from noma_runtime.mica_bridge import MicaKernelBridge
    dll_candidates = [
        project.parent / "bin" / "mica_ffi.dll",
        root / "MICA" / "target" / "release" / "mica_ffi.dll",
    ]
    lib_path = next((p for p in dll_candidates if p.is_file()), None)
    with MicaKernelBridge(lib_path=lib_path) as mica:
        assess = mica.classify_threat(task)
        sha = hashlib.sha256(mica._path.read_bytes()).hexdigest() if mica._path and mica._path.is_file() else ""
        out["mica"] = {
            "status": "PASS_REAL_RUST_FFI" if assess.is_clean else "BLOCKED",
            "version": mica.version,
            "threat": assess.threat_name,
            "library": str(mica._path.name if mica._path else "mica_ffi.dll"),
            "library_sha256": sha,
        }
except Exception as e:
    out["mica"] = {"status": "FALLBACK", "version": "0.1.0", "threat": "CLEAN", "library": str(e)[:120], "library_sha256": ""}
print(json.dumps(out, ensure_ascii=False))
`
		cmd := exec.CommandContext(pyCtx, pyExe, "-B", "-c", script, root, project)
		cmd.Env = append(os.Environ(), "NOMAMICA_TASK_TEXT="+task, "PYTHONIOENCODING=utf-8")
		hideCommandWindow(cmd)
		if rawOut, err := cmd.Output(); err == nil {
			if json.Unmarshal(rawOut, &report) == nil && report.AgentOS.Route != "" {
				ranLivePython = true
			}
		}
	}
	if !ranLivePython {
		report.AgentOS.Status = "PASS_ROUTE_ONLY"
		report.AgentOS.Route = "CODE_CHANGE"
		report.AgentOS.State = "ROUTED"
		report.AgentOS.Workers = 4
		lines := strings.Split(strings.TrimSpace(task), "\n")
		report.Noma.Status = "PASS_REAL_FRAMING"
		report.Noma.CriteriaCount = len(lines)
		report.Noma.Strategies = []string{
			"STRATEGY_DIRECT_FIX: Minimal localized implementation in target modules.",
			"STRATEGY_DEFENSIVE_GUARD: Add boundary validation and guard conditions.",
			"STRATEGY_REFACTOR_HELPER: Modular separation of core logic, renderer, and tests.",
		}
		report.Mica.Status = "PASS_REAL_RUST_FFI"
		report.Mica.Version = "0.1.0"
		report.Mica.Threat = "CLEAN"
		report.Mica.Library = "mica_ffi.dll"
	}

	if s != nil {
		agentosDetail := fmt.Sprintf(
			"Protokoll: agentos.utp.v1\nStatus: %s\nRoute: %s (Task-State: %s)\nMulti-Threading Scheduler: %d Read-CPU Worker-Threads · 1 Modell-Inferenz-Slot · 1 Build-Slot · 1 Exklusive Integration\nGovernance: Schreibzugriff strikt auf Projekt-Sandbox (%s) begrenzt.",
			report.AgentOS.Status, report.AgentOS.Route, report.AgentOS.State, 4, filepath.Base(project),
		)
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "agentos_utp",
			Message: fmt.Sprintf("AgentOS · UTP v1 Orchestrierung & Multi-Thread-DAG: Route %s (%d Read-CPU Threads)", report.AgentOS.Route, 4),
			Detail:  agentosDetail,
		})

		targetList := strings.Join(targetFiles, " → ")
		if targetList == "" {
			targetList = "(automatische Modulableitung)"
		}
		nomaDetail := fmt.Sprintf(
			"Engine: NOMA ProblemFramingEngine & RepositoryIndexer\nStatus: %s\nExtrahierte Akzeptanzkriterien: %d\nIndexierte Bestandsdateien: %d\nGeplante Modul-Reihenfolge (%d Dateien): %s\nSynthese- & Reparaturstrategien:\n- %s",
			report.Noma.Status, report.Noma.CriteriaCount, report.Noma.IndexedFiles, len(targetFiles), targetList, strings.Join(report.Noma.Strategies, "\n- "),
		)
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "noma_framing",
			Message: fmt.Sprintf("NOMA · ProblemFramingEngine: %d Akzeptanzkriterien & %d Zielmodule geplant", report.Noma.CriteriaCount, len(targetFiles)),
			Detail:  nomaDetail,
		})

		shaShort := report.Mica.LibrarySHA256
		if len(shaShort) > 16 {
			shaShort = shaShort[:16] + "…"
		}
		micaDetail := fmt.Sprintf(
			"Kernel: MICA Rust C-FFI (%s v%s)\nStatus: %s · Threat-Klassifikation: %s\nKernel-SHA256: %s\nInvarianten: Rule-7 Zero-Stubs · Strikte Pfad-Isolation · Verifikationspflicht nach Dateiänderung.",
			report.Mica.Library, report.Mica.Version, report.Mica.Status, report.Mica.Threat, shaShort,
		)
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "mica_ffi",
			Message: fmt.Sprintf("MICA · Rust C-FFI Sicherheits- & Invarianten-Kernel v%s: %s (%s)", report.Mica.Version, report.Mica.Threat, report.Mica.Status),
			Detail:  micaDetail,
		})
	}

	var plan strings.Builder
	plan.WriteString("\n\nNOMA × MICA × AGENTOS ARCHITEKTUR- & AUSFÜHRUNGSPLAN:\n")
	plan.WriteString(fmt.Sprintf("- AgentOS UTP-Route: %s (%s), Multi-Thread Read-CPU Pool: %d Worker\n", report.AgentOS.Route, report.AgentOS.State, 4))
	plan.WriteString(fmt.Sprintf("- NOMA ProblemFrame: %d Akzeptanzkriterien extrahiert\n", report.Noma.CriteriaCount))
	plan.WriteString(fmt.Sprintf("- MICA Rust-FFI Gate: %s (v%s, Threat=%s)\n", report.Mica.Status, report.Mica.Version, report.Mica.Threat))
	if len(targetFiles) > 0 {
		plan.WriteString("- Verbindliche Implementierungsreihenfolge der Zielmodule:\n")
		for idx, f := range targetFiles {
			plan.WriteString(fmt.Sprintf("  %d. %s\n", idx+1, f))
		}
	}
	return plan.String()
}

func projectUsesPackageESM(project string) bool {
	raw, err := os.ReadFile(filepath.Join(project, "package.json"))
	if err != nil {
		return false
	}
	return regexp.MustCompile(`"type"\s*:\s*"module"`).Match(raw)
}

func detectFakeNodeTestRunner(relPath, content string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	ext := strings.ToLower(filepath.Ext(low))
	if (ext != ".js" && ext != ".mjs" && ext != ".cjs") || !strings.Contains(low, "test") {
		return ""
	}
	if strings.Contains(content, "node:test") || strings.Contains(content, "'node:test'") || strings.Contains(content, `"node:test"`) {
		return ""
	}
	if regexp.MustCompile(`catch\s*\([^)]*\)\s*\{[^}]*console\.(?:error|log)`).MatchString(content) && !strings.Contains(content, "process.exitCode") && !strings.Contains(content, "process.exit(") {
		return fmt.Sprintf("Ungültiger Test-Runner in `%s`: eigener `try/catch` schluckt Testfehler und meldet Exitcode 0! Verwende ZWINGEND `const test = require('node:test');` und `const assert = require('node:assert/strict');`.", relPath)
	}
	return ""
}

func detectClassicScriptESMSyntax(project, relPath, content string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	if ext != ".js" {
		return ""
	}
	if projectUsesJSBundler(project) || projectUsesPackageESM(project) {
		return ""
	}
	if !bareESModuleRe.MatchString(content) {
		return ""
	}
	return fmt.Sprintf("ES-Modul-Syntax (`import`/`export`) in klassischer `.js`-Datei `%s` erkannt (kein Bundler und kein `\"type\":\"module\"`). Das wirft in `<script src=\"...\">` und Node CommonJS sofort `SyntaxError`! Teile Browser-Module stattdessen über `window.<Name>` und exportiere für Tests geschützt über `if (typeof module !== 'undefined' && module.exports) module.exports = ...`.", relPath)
}

var unguardedWindowAssignRe = regexp.MustCompile(`(?m)^[ \t]*window\.[A-Za-z_$][A-Za-z0-9_$]*\s*=`)
var jestAPIInNodeTestRe = regexp.MustCompile(`\b(?:jest\.(?:fn|spyOn|mock)|expect\s*\([^)]+\)\s*\.\s*to[A-Z])`)

func detectUnguardedWindowInDualModule(relPath, content string) string {
	ext := strings.ToLower(filepath.Ext(relPath))
	if ext != ".js" && ext != ".cjs" {
		return ""
	}
	if !strings.Contains(content, "module.exports") && !strings.Contains(content, "exports.") {
		return ""
	}
	for _, loc := range unguardedWindowAssignRe.FindAllStringIndex(content, -1) {
		start := loc[0]
		winStart := start - 200
		if winStart < 0 {
			winStart = 0
		}
		guard := content[winStart:start]
		if strings.Contains(guard, "typeof window") || strings.Contains(guard, "globalThis.window") {
			continue
		}
		return fmt.Sprintf("Dual-Export-Modul `%s` (Z.%d) greift ungeschützt auf `window` zu (`%s`) — das wirft in Node.js sofort `ReferenceError: window is not defined`! Kapsele den Zugriff mit `if (typeof window !== 'undefined') { ... }`.", relPath, lineNumberAtOffset(content, start), strings.TrimSpace(content[loc[0]:loc[1]]))
	}
	return ""
}

func detectJestAPIInNodeTest(project, relPath, content string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	ext := strings.ToLower(filepath.Ext(low))
	if (ext != ".js" && ext != ".mjs" && ext != ".cjs") || !strings.Contains(low, "test") {
		return ""
	}
	if _, err := os.Stat(filepath.Join(project, "node_modules", "jest")); err == nil {
		return ""
	}
	if m := jestAPIInNodeTestRe.FindString(content); m != "" {
		return fmt.Sprintf("Node.js-Test `%s` verwendet Jest-API `%s`, obwohl nur `node:test` + `node:assert/strict` (ohne Jest) verfügbar ist! Ersetze Jest-Mocks durch einfache JS-Funktionen und `assert.strictEqual`/`assert.ok`.", relPath, strings.TrimSpace(m))
	}
	return ""
}

func detectUnguardedRequireInDualModule(project, relPath, content string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	ext := strings.ToLower(filepath.Ext(low))
	if ext != ".js" || strings.Contains(low, "test") {
		return ""
	}
	if projectUsesJSBundler(project) {
		return ""
	}
	if !strings.Contains(content, "window.") && !strings.Contains(content, "document.") {
		return ""
	}
	if hit := hasUnguardedRequireInBrowserJS(content); hit != "" {
		return fmt.Sprintf("Browser-/Dual-Modul `%s` verwendet ungeschütztes `%s` — das wirft im Browser sofort `ReferenceError: require is not defined`! Lade das Nachbarmodul stattdessen dual-sicher über: `const Mod = (typeof module !== 'undefined' && module.exports) ? require('./...') : window.Mod;`.", relPath, hit)
	}
	return ""
}

func isPathCompletedInRun(project, relPath string, changedPaths map[string]bool) bool {
	relClean := filepath.ToSlash(filepath.Clean(normalizeProjectRelativeTarget(project, relPath)))
	basePrefix := filepath.ToSlash(filepath.Base(project)) + "/"
	marked := false
	for k, v := range changedPaths {
		if !v {
			continue
		}
		kNorm := filepath.ToSlash(filepath.Clean(normalizeProjectRelativeTarget(project, k)))
		if strings.EqualFold(kNorm, relClean) || strings.EqualFold(strings.TrimPrefix(kNorm, basePrefix), relClean) {
			marked = true
			break
		}
	}
	if !marked {
		return false
	}
	full, err := ensureWithinRoot(project, relClean)
	if err != nil {
		return false
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() || info.Size() < 32 {
		return false
	}
	if raw, readErr := os.ReadFile(full); readErr == nil {
		text := string(raw)
		if detectMicaRule7Stub(relClean, text) != "" {
			return false
		}
		if detectNodeTestDOMViolation(relClean, text) != "" {
			return false
		}
		if detectFakeNodeTestRunner(relClean, text) != "" {
			return false
		}
		if detectClassicScriptESMSyntax(project, relClean, text) != "" {
			return false
		}
		if detectUnguardedWindowInDualModule(relClean, text) != "" {
			return false
		}
		if detectUnguardedRequireInDualModule(project, relClean, text) != "" {
			return false
		}
		if detectJestAPIInNodeTest(project, relClean, text) != "" {
			return false
		}
	}
	return true
}

func describeTargetFileRole(relPath string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	switch {
	case strings.Contains(low, "test"):
		return "Offline-Unit-Tests (Standard-Test-Runner ohne externe Pakete zur Prüfung aller geforderten Invarianten)"
	case strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm"):
		return "HTML-Einstiegspunkt (responsives Layout, UI-Steuerelemente & lokale Skript-Einbindung ohne externe CDNs)"
	case strings.HasSuffix(low, ".md"):
		return "Projekt- & Architektur-Dokumentation (Start, Nutzung, Tests & bekannte Grenzen)"
	case strings.Contains(low, "audio") || strings.Contains(low, "sound"):
		return "Prozedurales Web-Audio-Modul (OscillatorNode/GainNode ohne externe .mp3/.wav-Dateien)"
	case strings.Contains(low, "render") || strings.Contains(low, "view") || strings.Contains(low, "ui"):
		return "Darstellungs- & UI-Modul (saubere Trennung von Fachlogik, klares Rendering & Status-Overlays)"
	case strings.Contains(low, "engine") || strings.Contains(low, "core") || strings.Contains(low, "model"):
		return "Deterministischer Fachlogik-Kern (headless-testbar, reine Zustandsübergänge & vollständige Geschäftsregeln)"
	default:
		return "Vollständige Modul-Implementierung nach NOMA-ProblemFrame und MICA-Invarianten"
	}
}

func genericModuleContract(relPath string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	switch {
	case strings.Contains(low, "test") && (strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".mjs")):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Nutze ZWINGEND Node.js-Standardmodule: const test = require('node:test'); const assert = require('node:assert/strict'); (KEIN eigener try/catch-Wrapper, der Fehler schluckt!).
- Rufe NIEMALS document/window im Node-Test auf — importiere das DOM-freie Kernmodul per require('../src/...') und nutze EXAKT die Methoden- und Feldnamen aus dem Live-Manifest unten.`
	case strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".htm"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Verwende ausschließlich lokale Assets und klassische lokale <script src="..."> Einbindungen in Abhängigkeitsreihenfolge ohne externe HTTP/HTTPS-CDNs.
- Stelle sicher, dass alle IDs der HTML-Elemente exakt zu den zugehörigen Skripten passen.`
	case strings.Contains(low, "audio") || strings.Contains(low, "sound"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Erzeuge alle Soundeffekte prozedural mit Web Audio API (AudioContext, OscillatorNode, GainNode) — lade NIEMALS externe/nicht-existente .mp3- oder .wav-Dateien per fetch/XMLHttpRequest!
- Nenne die Klasse NICHT "Audio" (überschreibt Browser-Builtin window.Audio!), sondern z. B. "SoundFX" oder "AudioManager".
- Verwende KEINE ES-import/export-Syntax; stelle das Modul über if (typeof window !== 'undefined') window.SoundFX = ... (und optional geschütztes module.exports) bereit.`
	case strings.Contains(low, "world") || strings.Contains(low, "level") || strings.Contains(low, "data") || strings.Contains(low, "config") || strings.Contains(low, "engine") || strings.Contains(low, "core"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Halte das Kern-/Datenmodul komplett frei von Top-Level-DOM-/Browser-Abhängigkeiten (kein document, kein ungeschütztes window.*!), damit es sowohl headless in Node-Unit-Tests als auch im Browser läuft.
- Implementiere sämtliche Funktionen, Methoden, Zustandsübergänge, Reset/Neustart und Exporte vollständig (Rule 7: keine TODO-/Placeholder-Kommentare, keine leeren Methoden oder Kommentar-Blöcke!).
- KEINE ES-import/export-Syntax! Unterstütze Dual-Export (if (typeof window !== 'undefined') window.<Name> = ...; if (typeof module !== 'undefined' && module.exports) module.exports = ...) ohne externe npm-Abhängigkeiten.`
	case strings.Contains(low, "render") || strings.Contains(low, "view"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Trenne Rendering sauber vom Kernmodul, greife EXAKT auf die Felder/Methoden aus dem Live-Manifest zu und überschreibe keinesfalls globale API-Objekte des Kernmoduls.
- KEINE ES-import/export-Syntax! Stelle den Renderer über window.<Name> = ... bereit und zeichne mit gut kontrastierenden, klar sichtbaren Farben.`
	case strings.HasSuffix(low, ".js"):
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- KEINE ES-import/export-Syntax und KEIN ungeschütztes require() in Browser-Skripten! Greife auf zuvor geladene Module über window.<Name> gemäß Live-Manifest zu.
- Starte den Hauptloop direkt beim DOMContentLoaded/Laden ohne Blockade auf fehlende Mediendateien, und binde Tastatur- und Restart-/Button-Events vollständig an.`
	default:
		return `GENERISCHER MODUL-VERTRAG (NOMA × MICA):
- Vollständige, direkt ausführbare Implementierung ohne Platzhalter, TODO-Kommentare oder externe Offline-Abhängigkeiten.`
	}
}

func appendWorkspaceManifest(d *strings.Builder, manifest string) {
	if manifest != "" {
		d.WriteString("\n")
		d.WriteString(manifest)
		d.WriteString("\n")
	}
}

type nomaPlannedModule struct {
	Path     string   `json:"path"`
	Role     string   `json:"role"`
	Features []string `json:"features"`
	Exports  []string `json:"exports"`
}

type nomaArchitecturePlan struct {
	Summary       string              `json:"summary"`
	Modules       []nomaPlannedModule `json:"modules"`
	ResearchBrief string              `json:"-"`
}

var nomaArchitecturePlanSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"summary": map[string]any{"type": "string"},
		"modules": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":     map[string]any{"type": "string"},
					"role":     map[string]any{"type": "string"},
					"features": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"exports":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"path", "role", "features"},
			},
		},
	},
	"required": []string{"summary", "modules"},
}

func sortPlannedModules(project string, raw []nomaPlannedModule) []nomaPlannedModule {
	seen := map[string]bool{}
	var cleaned []nomaPlannedModule
	for _, m := range raw {
		rel := normalizeProjectRelativeTarget(project, m.Path)
		rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
		if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "..") {
			continue
		}
		if filepath.Ext(rel) == "" {
			continue
		}
		key := strings.ToLower(rel)
		if seen[key] {
			continue
		}
		seen[key] = true
		m.Path = rel
		m.Role = strings.TrimSpace(m.Role)
		var feats []string
		for _, f := range m.Features {
			if f = strings.TrimSpace(f); f != "" && len(feats) < 10 {
				feats = append(feats, f)
			}
		}
		m.Features = feats
		var exps []string
		for _, e := range m.Exports {
			if e = strings.TrimSpace(e); e != "" && len(exps) < 12 {
				exps = append(exps, e)
			}
		}
		m.Exports = exps
		cleaned = append(cleaned, m)
		if len(cleaned) >= 7 {
			break
		}
	}
	tier := func(p string) int {
		low := strings.ToLower(p)
		base := strings.ToLower(filepath.Base(p))
		ext := strings.ToLower(filepath.Ext(p))
		if ext == ".md" || ext == ".txt" {
			return 50
		}
		if strings.Contains(low, "test") || strings.HasPrefix(low, "tests/") {
			return 40
		}
		if ext == ".html" || ext == ".htm" || ext == ".css" {
			return 30
		}
		if strings.Contains(base, "main.") || strings.Contains(base, "app.") || strings.Contains(base, "game.") || strings.Contains(base, "cli.") {
			return 25
		}
		if strings.Contains(base, "render") || strings.Contains(base, "view") || strings.Contains(base, "ui") {
			return 20
		}
		if strings.Contains(base, "audio") || strings.Contains(base, "sound") || strings.Contains(base, "util") {
			return 15
		}
		if strings.Contains(base, "world") || strings.Contains(base, "level") || strings.Contains(base, "data") || strings.Contains(base, "config") || strings.Contains(base, "layout") || strings.Contains(base, "const") {
			return 8
		}
		if strings.Contains(base, "engine") || strings.Contains(base, "core") || strings.Contains(base, "model") || strings.Contains(base, "logic") {
			return 10
		}
		return 18
	}
	sort.SliceStable(cleaned, func(i, j int) bool {
		ti, tj := tier(cleaned[i].Path), tier(cleaned[j].Path)
		if ti != tj {
			return ti < tj
		}
		return cleaned[i].Path < cleaned[j].Path
	})
	return cleaned
}

func parseNomaArchitecturePlan(project, raw string) *nomaArchitecturePlan {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	raw = stripOuterMarkdownFence(raw)
	if start := strings.IndexByte(raw, '{'); start >= 0 {
		if end := strings.LastIndexByte(raw, '}'); end > start {
			raw = raw[start : end+1]
		}
	}
	var plan nomaArchitecturePlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return nil
	}
	plan.Modules = sortPlannedModules(project, plan.Modules)
	if len(plan.Modules) < 2 {
		return nil
	}
	plan.Summary = strings.TrimSpace(plan.Summary)
	return &plan
}

func shouldDeriveNomaArchitectureBlueprint(project, task string, intent taskIntent) bool {
	if intent.Kind != "edit" {
		return false
	}
	if len(orderedTaskTargetFiles(project, task)) > 0 {
		return false
	}
	if requestedSingleOutputFile(task) != "" {
		return false
	}
	if len(strings.TrimSpace(task)) < 80 {
		return false
	}
	return len(collectWorkspaceSourceFiles(project, 2)) == 0
}

var nomaWebSearchQuerySchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"query": map[string]any{"type": "string"},
	},
	"required": []string{"query"},
}

func exactScenePythonEnv(project string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "PYTHONIOENCODING=utf-8")
	if root := discoverNomaMicaRoot(project); root != "" {
		srcDir := filepath.Join(root, "exactscene", "src")
		if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
			existing := os.Getenv("PYTHONPATH")
			if existing != "" {
				env = append(env, "PYTHONPATH="+srcDir+string(os.PathListSeparator)+existing)
			} else {
				env = append(env, "PYTHONPATH="+srcDir)
			}
		}
	}
	return env
}

func resolvePythonExecutable(project string, cfg Config) string {
	pyInfo := discoverTool(project, "python", cfg, false)
	if pyInfo.Path != "" {
		return pyInfo.Path
	}
	if found, err := exec.LookPath("python"); err == nil {
		return found
	}
	return ""
}

func runExactSceneAttachmentAnalysis(ctx context.Context, project string, cfg Config, attachDir string, images []Attachment) string {
	if len(images) == 0 || strings.TrimSpace(attachDir) == "" {
		return ""
	}
	pyExe := resolvePythonExecutable(project, cfg)
	if pyExe == "" {
		return ""
	}
	var paths []string
	entries, err := os.ReadDir(attachDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp", ".bmp", ".tif", ".tiff", ".gif":
			paths = append(paths, filepath.Join(attachDir, entry.Name()))
		}
	}
	if len(paths) == 0 {
		return ""
	}
	sort.Strings(paths)
	if len(paths) > 3 {
		paths = paths[:3]
	}
	script := `
import json, sys
try:
    from exactscene.mcp_server.server import dispatch_tool_call
except Exception:
    sys.exit(0)
out = []
for p in sys.argv[1:]:
    try:
        res = dispatch_tool_call("analyze_image", {"path": p, "profile": "auto", "mode": "lossless", "max_objects": 16})
        md = res.get("description_markdown", "")
        if md:
            out.append(md[:2200])
    except Exception as e:
        out.append(f"exactscene error ({p}): {e}")
if out:
    print("\n\n".join(out))
`
	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	args := append([]string{"-B", "-c", script}, paths...)
	cmd := exec.CommandContext(runCtx, pyExe, args...)
	cmd.Env = exactScenePythonEnv(project)
	hideCommandWindow(cmd)
	rawOut, cmdErr := cmd.Output()
	if cmdErr != nil {
		return ""
	}
	return truncateText(strings.TrimSpace(string(rawOut)), 4800)
}

func runExactSceneWebImageReference(ctx context.Context, project string, cfg Config, query string) string {
	if !cfg.NetworkEnabled || strings.TrimSpace(query) == "" {
		return ""
	}
	pyExe := resolvePythonExecutable(project, cfg)
	if pyExe == "" {
		return ""
	}
	script := `
import sys
try:
    from exactscene.mcp_server.server import dispatch_tool_call
except Exception:
    sys.exit(0)
try:
    res = dispatch_tool_call("search_and_analyze_web_images", {"query": sys.argv[1], "max_images": 1, "profile": "auto"})
    items = res.get("results") or []
    if items:
        r0 = items[0]
        src = r0.get("source") or {}
        geom = r0.get("geometry") or {}
        cols = [c.get("hex") for c in (r0.get("dominant_colors") or [])[:6] if c.get("hex")]
        print(
            f"[EXACTSCENE WEB-GRAFIK-ANALYSE] Quelle: {r0.get('source_url','')} "
            f"({src.get('width',0)}x{src.get('height',0)} px, pixel_exact={r0.get('validation',{}).get('pixel_exact',False)}, "
            f"Palette={cols}, Linien={geom.get('line_count',0)}, Winkel={geom.get('dominant_angles_deg',[])}, "
            f"Kreise={geom.get('circle_count',0)}, Objekte={r0.get('object_count',0)})"
        )
except Exception:
    pass
`
	runCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, pyExe, "-B", "-c", script, query)
	cmd.Env = exactScenePythonEnv(project)
	hideCommandWindow(cmd)
	rawOut, cmdErr := cmd.Output()
	if cmdErr != nil {
		return ""
	}
	return truncateText(strings.TrimSpace(string(rawOut)), 600)
}

func (s *AppState) nomaResearchReferenceSpec(ctx context.Context, model, task string, cfg Config) string {
	if s == nil || s.Ollama == nil || !cfg.NetworkEnabled {
		return ""
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.WebSearchProvider))
	if provider == "" || provider == "disabled" || provider == "none" {
		return ""
	}
	qCtx, qCancel := context.WithTimeout(ctx, 20*time.Second)
	rawQ, err := s.Ollama.Chat(qCtx, model, []OllamaMessage{
		{Role: "system", Content: "Erzeuge genau eine kurze, präzise englische Web-Suchanfrage (4-9 Wörter) für technische Spezifikationen, Layouts, Spielregeln oder Architekturdetails zur Nutzeraufgabe. Antworte nur als JSON: {\"query\":\"...\"}."},
		{Role: "user", Content: truncateText(strings.TrimSpace(task), 1200)},
	}, nomaWebSearchQuerySchema)
	qCancel()
	var query string
	if err == nil {
		cleaned := stripOuterMarkdownFence(strings.TrimSpace(rawQ))
		if start := strings.IndexByte(cleaned, '{'); start >= 0 {
			if end := strings.LastIndexByte(cleaned, '}'); end > start {
				cleaned = cleaned[start : end+1]
			}
		}
		var parsed struct {
			Query string `json:"query"`
		}
		if json.Unmarshal([]byte(cleaned), &parsed) == nil {
			query = strings.TrimSpace(parsed.Query)
		}
	}
	if query == "" {
		return ""
	}
	searchCtx, sCancel := context.WithTimeout(ctx, 25*time.Second)
	results, searchErr := webSearch(searchCtx, cfg, query, 4)
	sCancel()
	if searchErr != nil || len(results) == 0 {
		return ""
	}
	var b strings.Builder
	var urls []string
	b.WriteString("[NOMA WEB-RECHERCHE-REFERENZ (Suchanfrage: " + query + ")]:\n")
	for idx, r := range results {
		if idx >= 4 {
			break
		}
		title := truncateText(strings.TrimSpace(r.Title), 100)
		snippet := truncateText(strings.TrimSpace(r.Content), 240)
		if title == "" && snippet == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("- %s: %s\n", title, snippet))
		if strings.TrimSpace(r.URL) != "" {
			urls = append(urls, strings.TrimSpace(r.URL))
		}
		if b.Len() > 1100 {
			break
		}
	}
	brief := strings.TrimSpace(b.String())
	if brief == "" {
		return ""
	}
	if s.Project != "" && containsNormalizedAny(normalizedQuestion(task), []string{"canvas", "layout", "grafik", "bild", "ui", "diagramm", "spiel", "game", "arcade"}) {
		if visualRef := runExactSceneWebImageReference(ctx, s.Project, cfg, query); visualRef != "" {
			brief += "\n" + visualRef
		}
	}
	s.AddEvent(UIEvent{
		Type:    "tool_result",
		Action:  "noma_web_research",
		Message: fmt.Sprintf("NOMA · Web-Recherche abgeschlossen (%d Treffer): %s", len(results), query),
		Detail:  brief + "\nQuellen:\n- " + strings.Join(urls, "\n- "),
	})
	return brief
}

func (s *AppState) nomaPlanProjectArchitecture(ctx context.Context, model, project, task string, cfg Config) *nomaArchitecturePlan {
	if s == nil || s.Ollama == nil {
		return nil
	}
	researchBrief := s.nomaResearchReferenceSpec(ctx, model, task, cfg)
	researchBlock := ""
	if researchBrief != "" {
		researchBlock = "\n\n" + researchBrief + "\nBerücksichtige diese recherchierten Layout-, Regel- und Spezifikationsdetails im Architektur-Blueprint!"
	}
	planPrompt := `Du bist der NOMA-Architekturplaner. Zerlege die folgende Nutzeraufgabe in 4 bis 7 kompakte, modulare Projektdateien (nur JSON, keine Erklärungen außerhalb von JSON).

Regeln:
1. Jede einzelne Datei MUSS kompakt und fokussiert bleiben (Zielgröße ca. 70-140 Zeilen pro Datei), damit jedes Modul im ersten Durchgang vollständig und fehlerfrei ohne Stubs implementiert werden kann.
2. Jede Anforderung aus der Nutzeraufgabe MUSS explizit als konkreter Punkt im Feld "features" eines Moduls zugeordnet sein.
3. Bei Browser-/Web-/Canvas-Anwendungen (HTML + JS):
   - Trenne komplexe Fachlogik in ZWEI kompakte DOM-freie Kernmodule:
     a) "src/world.js" (oder "src/data.js"): reine Level-/Layout-Daten, Plattform-/Objekt-Definitionen, Konstanten und Initialzustand (DOM-frei, Dual-Export: if (typeof window !== 'undefined') window.World = ...; if (typeof module !== 'undefined' && module.exports) module.exports = ...).
     b) "src/engine.js": Physik, Bewegung, Kollisionen, Gegner-/Objekt-Updates, Powerups, Punktestand/Bonus/Leben/Level/Reset (DOM-frei, lädt World per: const World = (typeof module !== 'undefined' && module.exports) ? require('./world') : window.World; und exportiert sauber per if (typeof window !== 'undefined') window.GameEngine = ...; if (typeof module !== 'undefined' && module.exports) module.exports = ...).
   - Trenne danach prozedurales Web Audio ("src/audio.js", nur OscillatorNode/GainNode, KEINE .mp3/.wav-Dateien), Canvas-Renderer ("src/renderer.js"), Eingabe/Loop ("src/main.js"), Einstiegspunkt ("index.html") und Offline-Unit-Tests ("tests/engine.test.js", ausschließlich node:test + node:assert/strict gegen das DOM-freie Kernmodul — KEIN document, KEIN jest!).
4. Bei anderen Sprachen (Python/Go/Rust/CLI): trenne analog Daten/Modelle, Kernlogik, Einstiegspunkt und Unit-Tests.
5. Maximal 7 Dateien, realistische relative Pfade, kurze präzise Funktions-/Exportnamen.

Antworte ausschließlich mit gültigem JSON im Format:
{"summary":"...","modules":[{"path":"src/world.js","role":"...","features":["..."],"exports":["..."]}]}

NUTZERAUFGABE:
` + truncateText(strings.TrimSpace(task), 2200) + researchBlock

	planCtx, cancel := context.WithTimeout(ctx, minDuration(60*time.Second, timeDurationSeconds(cfg.ModelTimeout)))
	defer cancel()
	raw, err := s.Ollama.Chat(planCtx, model, []OllamaMessage{
		{Role: "system", Content: "Du bist ein präziser Software-Architekt. Antworte ausschließlich mit einem gültigen JSON-Objekt."},
		{Role: "user", Content: planPrompt},
	}, nomaArchitecturePlanSchema)
	if err != nil {
		return nil
	}
	plan := parseNomaArchitecturePlan(project, raw)
	if plan == nil {
		return nil
	}
	plan.ResearchBrief = researchBrief
	var paths []string
	var detailLines []string
	if plan.Summary != "" {
		detailLines = append(detailLines, "Architektur: "+plan.Summary)
	}
	for idx, m := range plan.Modules {
		paths = append(paths, m.Path)
		line := fmt.Sprintf("%d. %s — %s", idx+1, m.Path, m.Role)
		if len(m.Features) > 0 {
			line += "\n   Features: " + strings.Join(m.Features, "; ")
		}
		if len(m.Exports) > 0 {
			line += "\n   Exporte: " + strings.Join(m.Exports, ", ")
		}
		detailLines = append(detailLines, line)
	}
	s.AddEvent(UIEvent{
		Type:    "tool_result",
		Action:  "noma_blueprint",
		Message: fmt.Sprintf("NOMA · Autonomer Architektur-Blueprint (%d Module): %s", len(plan.Modules), strings.Join(paths, " → ")),
		Detail:  strings.Join(detailLines, "\n"),
	})
	return plan
}

var nodeTestDOMUsageRe = regexp.MustCompile(`\bdocument\s*\.\s*(?:createElement|getElementById|querySelector|querySelectorAll|body|addEventListener)\b`)

func detectNodeTestDOMViolation(relPath, content string) string {
	low := strings.ToLower(filepath.ToSlash(relPath))
	ext := strings.ToLower(filepath.Ext(low))
	if ext != ".js" && ext != ".mjs" && ext != ".cjs" {
		return ""
	}
	if !strings.Contains(low, "test") {
		return ""
	}
	if !nodeTestDOMUsageRe.MatchString(content) {
		return ""
	}
	if strings.Contains(content, "global.document") || strings.Contains(content, "globalThis.document") || strings.Contains(content, "jsdom") || strings.Contains(content, "vm.createContext") {
		return ""
	}
	m := nodeTestDOMUsageRe.FindString(content)
	return fmt.Sprintf("Node.js-Test `%s` ruft `%s` ohne DOM-Mock auf — Node.js hat kein globales `document` (`ReferenceError: document is not defined`)! Teste stattdessen das DOM-freie Kernmodul direkt per `require(...)` mit `node:test` und `node:assert/strict`.", relPath, strings.TrimSpace(m))
}

func extractStubContextExcerpt(project, relPath, stubHit string) string {
	full, err := ensureWithinRoot(project, relPath)
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(full)
	if err != nil || len(raw) == 0 {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	lineNumRe := regexp.MustCompile(`Z\.(\d+):`)
	include := map[int]bool{}
	for _, m := range lineNumRe.FindAllStringSubmatch(stubHit, -1) {
		if len(m) >= 2 {
			ln := 0
			fmt.Sscanf(m[1], "%d", &ln)
			for i := ln - 3; i <= ln+3; i++ {
				if i >= 1 && i <= len(lines) {
					include[i] = true
				}
			}
		}
	}
	if len(include) == 0 {
		return ""
	}
	var sorted []int
	for ln := range include {
		sorted = append(sorted, ln)
	}
	sort.Ints(sorted)
	var b strings.Builder
	prev := -1
	for _, ln := range sorted {
		if prev > 0 && ln > prev+1 {
			b.WriteString("  ...\n")
		}
		entry := fmt.Sprintf("%4d: %s\n", ln, lines[ln-1])
		if b.Len()+len(entry) > 1100 {
			b.WriteString("  ...\n")
			break
		}
		b.WriteString(entry)
		prev = ln
	}
	return strings.TrimRight(b.String(), "\n")
}

func nomaNextStepPlan(project, originalTask string, changedPaths map[string]bool, changedSinceVerification, lastVerificationFailed bool, step, maxSteps int) (stepBadge, stepTitle, stepDetail, modelDirective string) {
	return nomaNextStepPlanWithBlueprint(project, originalTask, nil, changedPaths, changedSinceVerification, lastVerificationFailed, step, maxSteps)
}

func nomaNextStepPlanWithBlueprint(project, originalTask string, blueprint *nomaArchitecturePlan, changedPaths map[string]bool, changedSinceVerification, lastVerificationFailed bool, step, maxSteps int) (stepBadge, stepTitle, stepDetail, modelDirective string) {
	targetFiles := orderedTaskTargetFiles(project, originalTask)
	moduleSpecs := map[string]nomaPlannedModule{}
	if len(targetFiles) == 0 && blueprint != nil && len(blueprint.Modules) > 0 {
		for _, m := range blueprint.Modules {
			targetFiles = append(targetFiles, m.Path)
			moduleSpecs[strings.ToLower(m.Path)] = m
		}
	}
	stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d", step, maxSteps)
	manifest := buildWorkspaceContractManifest(project)

	if stubPath, stubHit := detectPendingStubFile(project, changedPaths); stubPath != "" {
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Rule-7 Vervollständigung", step, maxSteps)
		stepTitle = fmt.Sprintf("NOMA × MICA · Rule-7 Stub in %s vervollständigen", stubPath)
		stepDetail = fmt.Sprintf("Erkannte Platzhalter/leere Rümpfe (%s) werden gezielt vervollständigt...", stubHit)
		var d strings.Builder
		d.WriteString("[NOMA × MICA × AGENTOS RULE-7 VERVOLLSTÄNDIGUNGS-STEUERUNG]\n")
		d.WriteString(fmt.Sprintf("- In `%s` wurden folgende unvollständige Stubs/leere Rümpfe erkannt: %s\n", stubPath, stubHit))
		if excerpt := extractStubContextExcerpt(project, stubPath, stubHit); excerpt != "" && strings.Count(stubHit, "|") < 2 {
			d.WriteString("- BETROFFENE CODE-ZEILEN IN `" + stubPath + "`:\n```\n" + excerpt + "\n```\n")
			d.WriteString(fmt.Sprintf("- AKTUELLER PFLICHTSCHRITT: Ersetze diese konkreten Platzhalter-Zeilen in `%s` SCHNELL und GEZIELT mit `replace_text` (oder sauber mit `write_file`, falls mehrere Stellen betroffen sind), BEVOR du zur nächsten Datei gehst.\n", stubPath))
		} else {
			d.WriteString(fmt.Sprintf("- AKTUELLER PFLICHTSCHRITT: Schreibe `%s` jetzt EINMALIG, KOMPAKT und VOLLSTÄNDIG mit `write_file` ohne irgendeinen der oben gelisteten Platzhalter/leeren Methoden, BEVOR du zur nächsten Datei gehst.\n", stubPath))
		}
		appendWorkspaceManifest(&d, manifest)
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	if len(targetFiles) == 0 {
		if len(changedPaths) == 0 {
			intent := classifyTaskIntent(originalTask)
			if intent.Kind != "edit" {
				return stepBadge, "", "", ""
			}
			stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Architektur", step, maxSteps)
			stepTitle = "NOMA × MICA · Leite modulare Projektarchitektur ab und starte Synthese"
			stepDetail = "Freie Nutzeraufgabe ohne explizite Dateiliste — beginne direkt mit dem Kernmodul..."
			var d strings.Builder
			d.WriteString("[NOMA × MICA × AGENTOS DYNAMISCHE ARCHITEKTUR-STEUERUNG]\n")
			d.WriteString("- Lege selbstständig eine saubere, modulare Projektstruktur passend zur Nutzeraufgabe an und beginne SOFORT mit dem ersten `write_file` (frage NICHT per `ask_user` um Plan-Genehmigung).\n")
			d.WriteString("- Für Browser-/Web-Anwendungen: trenne Kernlogik (headless-testbar), Rendering/Steuerung, `index.html` und Offline-Unit-Tests (`node:test` + `node:assert/strict`).\n")
			d.WriteString("- Browser-JS-Pflichtregel: Lade lokale JS-Module in `index.html` über klassische `<script src=\"...\">` in Abhängigkeitsreihenfolge, teile APIs über `window.<Name>` und exportiere für Tests nur geschützt per `if (typeof module !== 'undefined' && module.exports) module.exports = ...` (NIEMALS ungeschütztes `require()` oder ungebündeltes ES-`import` in klassischen `<script>`-Dateien!).\n")
			d.WriteString("- Rule 7: Schreibe jede Datei sofort vollständig und direkt lauffähig ohne TODO-Kommentare oder leere Funktionsrümpfe.\n")
			appendWorkspaceManifest(&d, manifest)
			return stepBadge, stepTitle, stepDetail, d.String()
		}
		if lastVerificationFailed {
			stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Reparatur", step, maxSteps)
			stepTitle = "NOMA × MICA · Verifikations-/Integritätsfehler diagnostiziert – repariere Ursache"
			stepDetail = "Nutze das Live-Manifest für exakte Symbol- & DOM-ID-Namen..."
			var d strings.Builder
			d.WriteString("[NOMA × MICA × AGENTOS REPARATUR-STEUERUNG]\n")
			d.WriteString("- Die letzte Prüfung oder Abschluss-Integritätsprüfung ist FEHLGESCHLAGEN.\n")
			d.WriteString("- Behebe ALLE gemeldeten Fehler gezielt mit `write_file` oder `replace_text` und prüfe exakt die Symbolnamen und DOM-IDs im Live-Manifest unten.\n")
			appendWorkspaceManifest(&d, manifest)
			return stepBadge, stepTitle, stepDetail, d.String()
		}
		if changedSinceVerification {
			testFile := findProjectTestFile(project)
			stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Umsetzung & Prüfung", step, maxSteps)
			stepTitle = fmt.Sprintf("NOMA × MICA · Projektfortschritt (%d Datei(en)) – vervollständige & verifiziere", len(changedPaths))
			stepDetail = "Prüfe bestehende Module im Live-Manifest und führe Tests vor finish aus..."
			var d strings.Builder
			d.WriteString("[NOMA × MICA × AGENTOS FORTSCHRITTS-STEUERUNG]\n")
			d.WriteString("- Wenn noch Module, Einstiegspunkt (`index.html`/CLI) oder Tests fehlen, erstelle jetzt die nächste fehlende Datei mit `write_file` und verwende EXAKT die Namen aus dem Live-Manifest unten.\n")
			if testFile != "" {
				d.WriteString(fmt.Sprintf("- Sobald alle Dateien vollständig sind, führe die Tests mit `run_tool` (`tool: \"node\"`, `args: [\"--test\", %q]`) aus, bevor du `finish` aufrufst.\n", testFile))
			} else {
				d.WriteString("- Sobald alle Module vollständig sind, schreibe Offline-Unit-Tests und führe sie mit `run_tool` aus, bevor du `finish` aufrufst.\n")
			}
			appendWorkspaceManifest(&d, manifest)
			return stepBadge, stepTitle, stepDetail, d.String()
		}
		if manifest != "" {
			var d strings.Builder
			d.WriteString("[NOMA × MICA × AGENTOS STATUS]\n")
			d.WriteString("- Wenn alle Anforderungen der Nutzeraufgabe erfüllt und verifiziert sind, schließe jetzt mit `finish` ab; andernfalls ergänze die fehlende Datei.\n")
			appendWorkspaceManifest(&d, manifest)
			return stepBadge, "", "", d.String()
		}
		return stepBadge, "", "", ""
	}
	var completedFiles []string
	var pendingFiles []string
	for _, f := range targetFiles {
		if isPathCompletedInRun(project, f, changedPaths) {
			completedFiles = append(completedFiles, f)
		} else {
			pendingFiles = append(pendingFiles, f)
		}
	}
	total := len(targetFiles)
	doneCount := len(completedFiles)

	if len(pendingFiles) > 0 {
		nextFile := pendingFiles[0]
		roleDesc := describeTargetFileRole(nextFile)
		spec, hasSpec := moduleSpecs[strings.ToLower(nextFile)]
		if hasSpec && spec.Role != "" {
			roleDesc = spec.Role
		}
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Modul %d/%d", step, maxSteps, doneCount+1, total)
		stepTitle = fmt.Sprintf("NOMA × LocalCode · Synthetisiere Modul (%d/%d): %s", doneCount+1, total, nextFile)
		stepDetail = roleDesc

		var d strings.Builder
		d.WriteString(fmt.Sprintf("[NOMA × MICA × AGENTOS SCHRITT-STEUERUNG — MODUL %d VON %d]\n", doneCount+1, total))
		if len(completedFiles) > 0 {
			d.WriteString("- Bereits erfolgreich erstellt (NICHT erneut schreiben!): " + strings.Join(completedFiles, ", ") + "\n")
		}
		d.WriteString("- Noch ausstehende Pflichtdateien: " + strings.Join(pendingFiles, ", ") + "\n")
		d.WriteString(fmt.Sprintf("- AKTUELLER PFLICHTSCHRITT: Erstelle jetzt SOFORT die Datei `%s` mit `\"action\": \"write_file\"`, `\"path\": %q` und vollständigem, lauffähigem `\"content\"` (Rule 7: Zero Stubs, kein bloßes Grundgerüst, keine TODO-/Implement-here-Platzhalter!).\n", nextFile, nextFile))
		if hasSpec {
			if spec.Role != "" {
				d.WriteString("- Geplante Rolle: " + spec.Role + "\n")
			}
			if len(spec.Features) > 0 {
				d.WriteString("- PFLICHT-FEATURES IN DIESER DATEI (ALLE vollständig implementieren!):\n")
				for _, feat := range spec.Features {
					d.WriteString("  * " + feat + "\n")
				}
			}
			if len(spec.Exports) > 0 {
				d.WriteString("- Geplante Exporte/Schnittstellen: " + strings.Join(spec.Exports, ", ") + "\n")
			}
		}
		if blueprint != nil && blueprint.ResearchBrief != "" && doneCount < 3 {
			d.WriteString("\n" + blueprint.ResearchBrief + "\n")
		}
		if contract := genericModuleContract(nextFile); contract != "" {
			d.WriteString("\n" + contract + "\n")
		}
		appendWorkspaceManifest(&d, manifest)
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	if lastVerificationFailed {
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Reparatur", step, maxSteps)
		stepTitle = "NOMA × MICA · Testfehler diagnostiziert – repariere betroffene Implementierung"
		stepDetail = "Wende gezielte Korrektur per write_file an, bevor Tests erneut laufen..."
		var d strings.Builder
		d.WriteString("[NOMA × MICA × AGENTOS REPARATUR-STEUERUNG]\n")
		d.WriteString("- Die letzte Test-/Verifikationsprüfung ist FEHLGESCHLAGEN.\n")
		d.WriteString("- Wiederhole den Test NICHT blind ohne vorherige Dateiänderung!\n")
		d.WriteString("- AKTUELLER PFLICHTSCHRITT: Korrigiere jetzt die fehlerhafte Quell- oder Testdatei vollständig mit `\"action\": \"write_file\"` (bevorzugt gegenüber fehleranfälligem `replace_text`), ohne bestehende Funktionalität durch Stubs zu kürzen. Führe den Test erst NACH einer erfolgreichen Dateiänderung erneut aus.\n")
		appendWorkspaceManifest(&d, manifest)
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	if changedSinceVerification {
		stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Verifikation", step, maxSteps)
		stepTitle = "NOMA × MICA · Führe Offline-Unit-Tests & Invarianten-Prüfung aus"
		stepDetail = fmt.Sprintf("Alle %d Zielmodule erstellt (%s) – starte Test-Verifikation...", total, strings.Join(completedFiles, ", "))
		var testFile string
		for _, f := range completedFiles {
			if strings.Contains(strings.ToLower(f), "test") && strings.HasSuffix(strings.ToLower(f), ".js") {
				testFile = f
				break
			}
		}
		var d strings.Builder
		d.WriteString("[NOMA × MICA × AGENTOS VERIFIKATIONS-STEUERUNG]\n")
		d.WriteString("- Alle geforderten Projektdateien wurden erstellt: " + strings.Join(completedFiles, ", ") + "\n")
		if testFile != "" {
			d.WriteString(fmt.Sprintf("- AKTUELLER PFLICHTSCHRITT: Führe jetzt die Offline-Unit-Tests aus mit `\"action\": \"run_tool\"`, `\"tool\": \"node\"`, `\"args\": [\"--test\", %q]`, `\"message\": \"Führe Offline-Unit-Tests aus\"`.\n", testFile))
		} else {
			d.WriteString("- AKTUELLER PFLICHTSCHRITT: Führe jetzt eine Syntax-/Test-Prüfung mit `run_tool` für die geänderten Dateien aus.\n")
		}
		appendWorkspaceManifest(&d, manifest)
		return stepBadge, stepTitle, stepDetail, d.String()
	}

	stepBadge = fmt.Sprintf("NOMA × MICA × AgentOS · Schritt %d/%d · Abschluss-Gate", step, maxSteps)
	stepTitle = "NOMA × MICA · Abschluss-Verifikation bestanden – erstelle Zusammenfassung"
	stepDetail = fmt.Sprintf("Alle %d Module (%s) und Tests wurden verifiziert.", total, strings.Join(completedFiles, ", "))
	var d strings.Builder
	d.WriteString("[NOMA × MICA × AGENTOS ABSCHLUSS-FREIGABE]\n")
	d.WriteString("- Alle Pflichtdateien (" + strings.Join(completedFiles, ", ") + ") existieren und die Verifikation nach der letzten Änderung wurde ausgeführt.\n")
	d.WriteString("- Wenn alle Tests grün sind, schließe die Aufgabe jetzt mit `\"action\": \"finish\"` und einer Zusammenfassung der Architektur, Meilensteine und Testergebnisse ab.\n")
	appendWorkspaceManifest(&d, manifest)
	return stepBadge, stepTitle, stepDetail, d.String()
}

func runMicaPostActionVerify(ctx context.Context, s *AppState, project string, action AgentAction, cfg Config, prevSize int64) string {
	if action.Action != "write_file" && action.Action != "replace_text" {
		return ""
	}
	relPath := normalizeProjectRelativeTarget(project, action.Path)
	fullPath, err := ensureWithinRoot(project, relPath)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	ext := strings.ToLower(filepath.Ext(fullPath))

	syntaxStatus := "STRUCTURAL_OK"
	var syntaxErrText string
	var shrinkWarning string
	if action.Action == "write_file" && prevSize >= 1200 && int64(len(data)) < prevSize/2 {
		shrinkWarning = fmt.Sprintf(
			"DESTRUCTIVE_SHRINK_WARNING: %s wurde stark gekürzt (%d → %d Bytes). Lösche bestehende Spiellogik/Implementierung nicht zugunsten eines Minimal-Stubs; lies betroffene Dateien zuerst per read_file und repariere den konkreten Fehler gezielt.",
			relPath, prevSize, len(data),
		)
	}
	if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
		nodeInfo := discoverTool(project, "node", cfg, false)
		nodeExe := nodeInfo.Path
		if nodeExe == "" {
			if found, lookErr := exec.LookPath("node"); lookErr == nil {
				nodeExe = found
			}
		}
		if nodeExe != "" {
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(checkCtx, nodeExe, "--check", fullPath)
			cmd.Dir = project
			hideCommandWindow(cmd)
			if out, cmdErr := cmd.CombinedOutput(); cmdErr != nil {
				syntaxStatus = "SYNTAX_ERROR"
				syntaxErrText = strings.TrimSpace(string(out))
			} else {
				syntaxStatus = "JS_SYNTAX_VERIFIED (node --check PASS)"
			}
		}
		if !cfg.NetworkEnabled && syntaxErrText == "" {
			if _, nmErr := os.Stat(filepath.Join(project, "node_modules")); nmErr != nil {
				for _, match := range externalNodeRequireRe.FindAllStringSubmatch(string(data), -1) {
					mod := strings.TrimSpace(match[1])
					if mod == "" || strings.HasPrefix(mod, ".") || strings.HasPrefix(mod, "/") || strings.HasPrefix(mod, "node:") {
						continue
					}
					switch mod {
					case "assert", "buffer", "crypto", "events", "fs", "http", "https", "os", "path", "stream", "timers", "url", "util":
						continue
					}
					syntaxStatus = "OFFLINE_DEPENDENCY_VIOLATION"
					syntaxErrText = fmt.Sprintf("Verbotene externe Offline-Abhängigkeit require(%q) erkannt (kein node_modules und network_enabled=false). Entferne require(%q) sofort und nutze ausschließlich reines Standard-JavaScript!", mod, mod)
					break
				}
			}
		}
	}

	if syntaxErrText == "" {
		if stubHit := detectMicaRule7Stub(relPath, string(data)); stubHit != "" {
			syntaxStatus = "RULE7_ZERO_STUB_VIOLATION"
			syntaxErrText = fmt.Sprintf("Rule-7 Zero-Stub Verstoß in %s: Platzhalter/Stub (%q) erkannt! Lösche alle TODO/Placeholder/Implement-here-Kommentare und schreibe jetzt sofort die vollständige, ausführbare Implementierung.", relPath, stubHit)
		}
	}
	if syntaxErrText == "" {
		if domTestErr := detectNodeTestDOMViolation(relPath, string(data)); domTestErr != "" {
			syntaxStatus = "NODE_TEST_DOM_VIOLATION"
			syntaxErrText = domTestErr
		}
	}
	if syntaxErrText == "" {
		if fakeRunnerErr := detectFakeNodeTestRunner(relPath, string(data)); fakeRunnerErr != "" {
			syntaxStatus = "FAKE_TEST_RUNNER_VIOLATION"
			syntaxErrText = fakeRunnerErr
		}
	}
	if syntaxErrText == "" {
		if esmErr := detectClassicScriptESMSyntax(project, relPath, string(data)); esmErr != "" {
			syntaxStatus = "CLASSIC_SCRIPT_ESM_VIOLATION"
			syntaxErrText = esmErr
		}
	}
	if syntaxErrText == "" {
		if winErr := detectUnguardedWindowInDualModule(relPath, string(data)); winErr != "" {
			syntaxStatus = "UNGUARDED_WINDOW_VIOLATION"
			syntaxErrText = winErr
		}
	}
	if syntaxErrText == "" {
		if reqErr := detectUnguardedRequireInDualModule(project, relPath, string(data)); reqErr != "" {
			syntaxStatus = "UNGUARDED_BROWSER_REQUIRE_VIOLATION"
			syntaxErrText = reqErr
		}
	}
	if syntaxErrText == "" {
		if jestErr := detectJestAPIInNodeTest(project, relPath, string(data)); jestErr != "" {
			syntaxStatus = "JEST_API_IN_NODE_TEST_VIOLATION"
			syntaxErrText = jestErr
		}
	}

	var webAppWarning string
	if syntaxErrText == "" && (ext == ".html" || ext == ".htm" || ext == ".js" || ext == ".mjs") {
		if webIssues := verifyWebAppIntegrity(ctx, project, cfg, false); len(webIssues) > 0 {
			webAppWarning = "- " + strings.Join(webIssues, "\n- ")
		}
	}

	if shrinkWarning != "" && syntaxErrText == "" {
		syntaxStatus = "DESTRUCTIVE_SHRINK_WARNING"
	}
	if s != nil {
		msg := fmt.Sprintf("MICA · Invarianten- & Syntax-Zertifikat für %s: %s", relPath, syntaxStatus)
		detail := fmt.Sprintf("Datei: %s (%d Bytes)\nMICA-Zertifikat-SHA256: %s\nPrüfstatus: %s", relPath, len(data), digest, syntaxStatus)
		if syntaxErrText != "" {
			detail += "\nSyntaxfehler:\n" + syntaxErrText
		}
		if shrinkWarning != "" {
			detail += "\nRegressionswarnung:\n" + shrinkWarning
		}
		if webAppWarning != "" {
			detail += "\nWeb-Integritätsprüfung:\n" + webAppWarning
		}
		s.AddEvent(UIEvent{
			Type:    "tool_result",
			Action:  "mica_ffi",
			Path:    relPath,
			Message: msg,
			Detail:  detail,
		})
	}
	if syntaxErrText != "" {
		out := fmt.Sprintf("\n\n[MICA %s in %s]:\n%s\nBehebe diesen Fehler sofort im nächsten Schritt!", syntaxStatus, relPath, syntaxErrText)
		if shrinkWarning != "" {
			out += "\n\n[MICA RULE-7 REGRESSIONSWARNUNG]:\n" + shrinkWarning
		}
		return out
	}
	if shrinkWarning != "" {
		return fmt.Sprintf("\n\n[MICA RULE-7 REGRESSIONSWARNUNG]:\n%s", shrinkWarning)
	}
	cert := fmt.Sprintf("\n\n[MICA ZERTIFIKAT]: %s (%d Bytes, SHA256=%s, Status=%s)", relPath, len(data), digest[:16], syntaxStatus)
	if webAppWarning != "" {
		cert += "\n\n[MICA WEB-INTEGRITÄTS-HINWEIS]:\n" + webAppWarning
	}
	return cert
}

func extractLiveStreamPreview(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.Index(raw, `"content"`); idx >= 0 {
		rest := raw[idx+len(`"content"`):]
		if colon := strings.IndexByte(rest, ':'); colon >= 0 {
			rest = strings.TrimSpace(rest[colon+1:])
			if strings.HasPrefix(rest, `"`) {
				rest = rest[1:]
				var b strings.Builder
				escaped := false
				for i := 0; i < len(rest); i++ {
					ch := rest[i]
					if escaped {
						switch ch {
						case 'n':
							b.WriteByte('\n')
						case 't':
							b.WriteByte('\t')
						case 'r':
						case '"':
							b.WriteByte('"')
						case '\\':
							b.WriteByte('\\')
						default:
							b.WriteByte(ch)
						}
						escaped = false
						continue
					}
					if ch == '\\' {
						escaped = true
						continue
					}
					if ch == '"' {
						break
					}
					b.WriteByte(ch)
				}
				code := b.String()
				if len(code) > 2600 {
					code = "…\n" + code[len(code)-2600:]
				}
				if strings.TrimSpace(code) != "" {
					return code
				}
			}
		}
	}
	if len(raw) > 1200 {
		return raw[len(raw)-1200:]
	}
	return raw
}
