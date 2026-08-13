// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxInstructionContextBytes = 56000
	maxInstructionDocBytes     = 14000
	maxRuleDocBytes            = 8000
	maxSkillDocBytes           = 9000
	maxSkillReadBytes          = 24000
	maxSkillResourceBytes      = 20000
)

type instructionDocument struct {
	Label   string
	Path    string
	Content string
}

type localSkillSummary struct {
	Name        string
	Path        string
	Description string
	Relevant    bool
	AlwaysApply bool
	Globs       []string
}

func projectInstructionContext(project, task string) string {
	project = filepath.Clean(project)
	task = strings.TrimSpace(task)
	var docs []instructionDocument
	seen := map[string]bool{}
	add := func(label, path string, limit int) {
		content, ok := readInstructionFile(path, limit)
		if !ok {
			return
		}
		abs, err := filepath.Abs(path)
		if err == nil {
			path = filepath.Clean(abs)
		}
		key := strings.ToLower(path)
		if seen[key] {
			return
		}
		seen[key] = true
		docs = append(docs, instructionDocument{Label: label, Path: path, Content: content})
	}

	for _, path := range globalInstructionFiles() {
		add("Globale Anweisungen", path, maxInstructionDocBytes)
	}
	for _, path := range projectInstructionFiles(project) {
		add("Projektanweisungen", path, maxInstructionDocBytes)
	}
	for _, path := range []string{filepath.Join(project, "README.md"), filepath.Join(project, "STATE.md")} {
		add("Projektdokument", path, maxInstructionDocBytes)
	}
	for _, path := range cursorRuleFiles(project, task) {
		add("Cursor-Regel", path, maxRuleDocBytes)
	}

	skills := localSkillSummaries(project, task)
	for _, skill := range skills {
		if skill.Relevant {
			add("Relevanter Skill", skill.Path, maxSkillDocBytes)
		}
	}

	var parts []string
	for _, doc := range docs {
		parts = append(parts, fmt.Sprintf("--- %s: %s ---\n%s", doc.Label, displayInstructionPath(project, doc.Path), doc.Content))
	}
	if len(skills) > 0 {
		parts = append(parts, localSkillIndex(project, skills))
	}
	if len(parts) == 0 {
		return "Keine Projektdokumente oder Regel-/Skill-Dateien vorhanden."
	}
	return truncateText(strings.Join(parts, "\n\n"), maxInstructionContextBytes)
}

func readInstructionFile(path string, limit int) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil || !isProbablyText(data) {
		return "", false
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", false
	}
	return truncateText(content, limit), true
}

func globalInstructionFiles() []string {
	var out []string
	for _, dir := range []string{appDataDir(), codexHomeDir()} {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		out = append(out, firstExistingInstructionFile(dir))
	}
	return compactNonEmpty(out)
}

func codexHomeDir() string {
	if override := strings.TrimSpace(os.Getenv("CODEX_HOME")); override != "" {
		return filepath.Clean(override)
	}
	return filepath.Join(userProfileDir(), ".codex")
}

func firstExistingInstructionFile(dir string) string {
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func projectInstructionFiles(project string) []string {
	return compactNonEmpty([]string{firstExistingInstructionFile(project), firstExistingFallbackInstructionFile(project)})
}

func firstExistingFallbackInstructionFile(project string) string {
	for _, name := range []string{"CLAUDE.md", ".agents.md", "TEAM_GUIDE.md"} {
		path := filepath.Join(project, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

func cursorRuleFiles(project, task string) []string {
	root := filepath.Join(project, ".cursor", "rules")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".mdc" {
			continue
		}
		path := filepath.Join(root, name)
		content, ok := readInstructionFile(path, maxRuleDocBytes)
		if !ok {
			continue
		}
		globs := frontmatterList(content, "globs")
		if cursorRuleAlwaysApplies(content) || instructionGlobsMatchTask(project, task, globs) || instructionTextRelevant(task, name+" "+frontmatterValue(content, "description")+" "+content) {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func cursorRuleAlwaysApplies(content string) bool {
	value := strings.ToLower(frontmatterValue(content, "alwaysApply"))
	return value == "true" || value == "yes" || value == "1"
}

func localSkillSummaries(project, task string) []localSkillSummary {
	roots := availableSkillRoots(project)
	var skills []localSkillSummary
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(d.Name(), "SKILL.md") {
				return nil
			}
			content, ok := readInstructionFile(path, maxSkillDocBytes)
			if !ok {
				return nil
			}
			name := filepath.Base(filepath.Dir(path))
			description := frontmatterValue(content, "description")
			if description == "" {
				description = firstMarkdownParagraph(content)
			}
			globs := frontmatterList(content, "globs")
			alwaysApply := cursorRuleAlwaysApplies(content)
			relevant := alwaysApply || instructionGlobsMatchTask(project, task, globs) || instructionTextRelevant(task, name+" "+description)
			skills = append(skills, localSkillSummary{
				Name:        name,
				Path:        path,
				Description: truncateText(strings.TrimSpace(description), 600),
				Relevant:    relevant,
				AlwaysApply: alwaysApply,
				Globs:       globs,
			})
			return nil
		})
	}
	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Relevant != skills[j].Relevant {
			return skills[i].Relevant
		}
		if strings.EqualFold(skills[i].Name, skills[j].Name) {
			return strings.ToLower(skills[i].Path) < strings.ToLower(skills[j].Path)
		}
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills
}

func availableSkillRoots(project string) []string {
	var roots []string
	for _, root := range []string{
		filepath.Join(project, ".codex", "skills"),
		filepath.Join(project, ".cursor", "skills"),
		filepath.Join(project, ".opencode", "skills"),
		filepath.Join(project, "skills"),
		filepath.Join(appDataDir(), "skills"),
		filepath.Join(codexHomeDir(), "skills"),
		filepath.Join(userProfileDir(), ".cursor", "skills"),
		filepath.Join(userProfileDir(), ".opencode", "skills"),
	} {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			roots = append(roots, root)
		}
	}
	return roots
}

func localSkillIndex(project string, skills []localSkillSummary) string {
	var lines []string
	lines = append(lines, "--- Verfügbare Skills ---")
	lines = append(lines, "Nutze relevante Skills als Arbeitsanweisung. Wenn ein Skill nur im Index steht, nutze skill_read vor der Anwendung.")
	for _, skill := range skills {
		marker := "available"
		if skill.Relevant {
			marker = "loaded"
		}
		desc := skill.Description
		if desc == "" {
			desc = "Keine Beschreibung."
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s: %s", skill.Name, marker, displayInstructionPath(project, skill.Path), desc))
	}
	return strings.Join(lines, "\n")
}

func formatSkillList(project, query string) string {
	skills := localSkillSummaries(project, query)
	if len(skills) == 0 {
		return "Keine Skill-Verzeichnisse mit SKILL.md gefunden."
	}
	return localSkillIndex(project, skills)
}

func readSkillByName(project, name string) (string, error) {
	skill, err := findSkillByName(project, name)
	if err != nil {
		return "", err
	}
	return formatSkillRead(project, skill)
}

func findSkillByName(project, name string) (localSkillSummary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return localSkillSummary{}, errors.New("skill is empty")
	}
	skills := localSkillSummaries(project, "")
	var partial *localSkillSummary
	for i := range skills {
		skill := &skills[i]
		if strings.EqualFold(skill.Name, name) || strings.EqualFold(filepath.ToSlash(skill.Path), filepath.ToSlash(name)) || strings.EqualFold(displayInstructionPath(project, skill.Path), filepath.ToSlash(name)) {
			return *skill, nil
		}
		if partial == nil && strings.Contains(strings.ToLower(skill.Name), strings.ToLower(name)) {
			partial = skill
		}
	}
	if partial != nil {
		return *partial, nil
	}
	return localSkillSummary{}, fmt.Errorf("skill %q not found", name)
}

func formatSkillRead(project string, skill localSkillSummary) (string, error) {
	content, ok := readInstructionFile(skill.Path, maxSkillReadBytes)
	if !ok {
		return "", fmt.Errorf("skill %s cannot be read", skill.Name)
	}
	return fmt.Sprintf("--- Skill: %s ---\nPath: %s\nRelevant: %t\n\n%s", skill.Name, displayInstructionPath(project, skill.Path), skill.Relevant, content), nil
}

func listSkillResources(project, name string) (string, error) {
	skill, err := findSkillByName(project, name)
	if err != nil {
		return "", err
	}
	root := filepath.Dir(skill.Path)
	var lines []string
	lines = append(lines, fmt.Sprintf("--- Skill-Ressourcen: %s ---", skill.Name))
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || strings.EqualFold(path, skill.Path) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		info, statErr := d.Info()
		size := int64(0)
		if statErr == nil {
			size = info.Size()
		}
		lines = append(lines, fmt.Sprintf("- %s (%d bytes)", filepath.ToSlash(rel), size))
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(lines) == 1 {
		lines = append(lines, "Keine zusätzlichen Ressourcen gefunden.")
	}
	return strings.Join(lines, "\n"), nil
}

func readSkillResource(project, name, resource string) (string, error) {
	skill, err := findSkillByName(project, name)
	if err != nil {
		return "", err
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return "", errors.New("resource is empty")
	}
	if filepath.IsAbs(resource) {
		return "", errors.New("resource must be relative to the skill directory")
	}
	root := filepath.Dir(skill.Path)
	full := filepath.Clean(filepath.Join(root, resource))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", errors.New("resource escapes the skill directory")
	}
	if strings.EqualFold(filepath.Clean(full), filepath.Clean(skill.Path)) {
		return "", errors.New("use skill_read for SKILL.md")
	}
	content, ok := readInstructionFile(full, maxSkillResourceBytes)
	if !ok {
		return "", fmt.Errorf("skill resource %q is missing, empty, binary, or too large to read as text", resource)
	}
	return fmt.Sprintf("--- Skill-Ressource: %s / %s ---\nPath: %s\n\n%s", skill.Name, filepath.ToSlash(rel), displayInstructionPath(project, full), content), nil
}

func instructionTextRelevant(task, text string) bool {
	taskWords := significantWords(task)
	if len(taskWords) == 0 {
		return false
	}
	text = strings.ToLower(text)
	for _, word := range taskWords {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func frontmatterList(content, key string) []string {
	value := strings.TrimSpace(frontmatterValue(content, key))
	if value == "" {
		return nil
	}
	value = strings.Trim(value, "[]")
	parts := strings.Split(value, ",")
	var out []string
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func instructionGlobsMatchTask(project, task string, globs []string) bool {
	if len(globs) == 0 {
		return false
	}
	for _, mentioned := range mentionedProjectFiles(task) {
		for _, glob := range globs {
			if instructionGlobMatch(project, mentioned, glob) {
				return true
			}
		}
	}
	return false
}

func instructionGlobMatch(project, mentioned, glob string) bool {
	mentioned = filepath.ToSlash(strings.TrimSpace(mentioned))
	glob = filepath.ToSlash(strings.TrimSpace(glob))
	if mentioned == "" || glob == "" {
		return false
	}
	if filepath.IsAbs(mentioned) {
		if rel, err := filepath.Rel(project, mentioned); err == nil {
			mentioned = filepath.ToSlash(rel)
		}
	}
	patterns := []string{glob}
	if strings.HasPrefix(glob, "**/") {
		patterns = append(patterns, strings.TrimPrefix(glob, "**/"))
	}
	candidates := []string{mentioned, filepath.Base(mentioned)}
	for _, pattern := range patterns {
		for _, candidate := range candidates {
			if ok, _ := pathGlobMatch(pattern, candidate); ok {
				return true
			}
		}
	}
	return false
}

func pathGlobMatch(pattern, value string) (bool, error) {
	pattern = filepath.ToSlash(pattern)
	value = filepath.ToSlash(value)
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, `\*\*`, ".*")
	re = strings.ReplaceAll(re, `\*`, `[^/]*`)
	re = strings.ReplaceAll(re, `\?`, `[^/]`)
	return regexp.MatchString("^"+re+"$", value)
}

func significantWords(text string) []string {
	matches := regexp.MustCompile(`[\pL\pN][\pL\pN._-]{2,}`).FindAllString(strings.ToLower(text), -1)
	stop := map[string]bool{
		"und": true, "oder": true, "das": true, "die": true, "der": true, "den": true, "dem": true, "ein": true, "eine": true, "einen": true, "mit": true, "für": true, "fuer": true, "von": true, "nach": true, "bitte": true, "mach": true,
		"the": true, "and": true, "or": true, "for": true, "with": true, "from": true, "this": true, "that": true, "please": true, "make": true, "create": true, "change": true,
	}
	seen := map[string]bool{}
	var out []string
	for _, word := range matches {
		if stop[word] || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	return out
}

func frontmatterValue(content, key string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return ""
	}
	rest := strings.TrimPrefix(content, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	key = strings.ToLower(key)
	for _, line := range strings.Split(rest[:end], "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || strings.ToLower(strings.TrimSpace(parts[0])) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	}
	return ""
}

func firstMarkdownParagraph(content string) string {
	for _, block := range strings.Split(strings.TrimSpace(content), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" || strings.HasPrefix(block, "---") || strings.HasPrefix(block, "#") {
			continue
		}
		return strings.Join(strings.Fields(block), " ")
	}
	return ""
}

func displayInstructionPath(project, path string) string {
	if rel, err := filepath.Rel(project, path); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
		return filepath.ToSlash(rel)
	}
	return filepath.Clean(path)
}

func compactNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
