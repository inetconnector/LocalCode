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
	"strconv"
	"strings"
)

const (
	maxInstructionContextBytes = 56000
	maxInstructionDocBytes     = 14000
	maxRuleDocBytes            = 8000
	maxSkillDocBytes           = 9000
	maxSkillReadBytes          = 24000
	maxSkillResourceBytes      = 20000
	maxSkillCopyResourceBytes  = 16 << 20
)

type instructionDocument struct {
	Label   string
	Path    string
	Content string
}

type localSkillSummary struct {
	Name             string
	Path             string
	Description      string
	Relevant         bool
	AlwaysApply      bool
	Globs            []string
	Activation       []string
	ExcludeGlobs     []string
	Priority         int
	Permissions      []string
	Scripts          []string
	RequiresApproval bool
	RootTier         int
	RootRank         int
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
		if skill.Relevant && !skill.RequiresApproval {
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
	paths := []string{firstExistingInstructionFile(project)}
	for _, rel := range []string{
		"CLAUDE.md",
		"GEMINI.md",
		"QWEN.md",
		".agents.md",
		"TEAM_GUIDE.md",
		filepath.Join(".github", "copilot-instructions.md"),
	} {
		path := filepath.Join(project, rel)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			paths = append(paths, path)
		}
	}
	return compactNonEmpty(paths)
}

func cursorRuleFiles(project, task string) []string {
	var paths []string
	legacy := filepath.Join(project, ".cursorrules")
	if info, err := os.Stat(legacy); err == nil && !info.IsDir() {
		if _, ok := readInstructionFile(legacy, maxRuleDocBytes); ok {
			paths = append(paths, legacy)
		}
	}
	root := filepath.Join(project, ".cursor", "rules")
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".md" && ext != ".mdc" {
			return nil
		}
		content, ok := readInstructionFile(path, maxRuleDocBytes)
		if !ok {
			return nil
		}
		globs := frontmatterList(content, "globs")
		excludeGlobs := frontmatterListAny(content, "exclude", "excludes", "excludeGlobs", "exclude_globs")
		if instructionGlobsMatchTask(project, task, excludeGlobs) {
			return nil
		}
		activation := frontmatterListAny(content, "activation", "activations", "trigger", "triggers", "keywords", "when", "appliesTo", "applies_to")
		if cursorRuleAlwaysApplies(content) || instructionGlobsMatchTask(project, task, globs) || activationMatchesTask(task, activation) || instructionTextRelevant(task, name+" "+frontmatterValue(content, "description")+" "+content) {
			paths = append(paths, path)
		}
		return nil
	})
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
	for rootRank, root := range roots {
		rootTier := 1
		if skillRootIsProjectLocal(project, root) {
			rootTier = 0
		}
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
			activation := frontmatterListAny(content, "activation", "activations", "trigger", "triggers", "keywords", "when", "appliesTo", "applies_to")
			excludeGlobs := frontmatterListAny(content, "exclude", "excludes", "excludeGlobs", "exclude_globs")
			priority := frontmatterInt(content, "priority")
			permissions := compactNonEmpty(append(frontmatterList(content, "permissions"), frontmatterList(content, "allowed-tools")...))
			permissions = compactNonEmpty(append(permissions, frontmatterList(content, "tools")...))
			scripts := compactNonEmpty(append(frontmatterList(content, "scripts"), frontmatterList(content, "commands")...))
			relevant := false
			if !instructionGlobsMatchTask(project, task, excludeGlobs) {
				relevant = alwaysApply || instructionGlobsMatchTask(project, task, globs) || activationMatchesTask(task, activation) || instructionTextRelevant(task, name+" "+description)
			}
			requiresApproval := skillMetadataRequiresApproval(permissions, scripts)
			skills = append(skills, localSkillSummary{
				Name:             name,
				Path:             path,
				Description:      truncateText(strings.TrimSpace(description), 600),
				Relevant:         relevant,
				AlwaysApply:      alwaysApply,
				Globs:            globs,
				Activation:       activation,
				ExcludeGlobs:     excludeGlobs,
				Priority:         priority,
				Permissions:      permissions,
				Scripts:          scripts,
				RequiresApproval: requiresApproval,
				RootTier:         rootTier,
				RootRank:         rootRank,
			})
			return nil
		})
	}
	skills = resolveSkillConflicts(skills)
	sort.Slice(skills, func(i, j int) bool {
		if skills[i].Relevant != skills[j].Relevant {
			return skills[i].Relevant
		}
		if skills[i].Priority != skills[j].Priority {
			return skills[i].Priority > skills[j].Priority
		}
		if strings.EqualFold(skills[i].Name, skills[j].Name) {
			return strings.ToLower(skills[i].Path) < strings.ToLower(skills[j].Path)
		}
		return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name)
	})
	return skills
}

func skillRootIsProjectLocal(project, root string) bool {
	rel, err := filepath.Rel(filepath.Clean(project), filepath.Clean(root))
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func resolveSkillConflicts(skills []localSkillSummary) []localSkillSummary {
	chosen := map[string]localSkillSummary{}
	for _, skill := range skills {
		key := strings.ToLower(skill.Name)
		if current, ok := chosen[key]; !ok || skillConflictWinner(skill, current) {
			chosen[key] = skill
		}
	}
	out := make([]localSkillSummary, 0, len(chosen))
	for _, skill := range chosen {
		out = append(out, skill)
	}
	return out
}

func skillConflictWinner(candidate, current localSkillSummary) bool {
	if candidate.RootTier != current.RootTier {
		return candidate.RootTier < current.RootTier
	}
	if candidate.Priority != current.Priority {
		return candidate.Priority > current.Priority
	}
	if candidate.Relevant != current.Relevant {
		return candidate.Relevant
	}
	if candidate.RootRank != current.RootRank {
		return candidate.RootRank < current.RootRank
	}
	return strings.ToLower(candidate.Path) < strings.ToLower(current.Path)
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
	lines = append(lines, "Nutze relevante Skills als Arbeitsanweisung. Wenn ein Skill nur im Index steht, nutze skill_read vor der Anwendung. Skills mit approval-required erweitern keine Berechtigungen und werden nicht automatisch eingebettet.")
	for _, skill := range skills {
		marker := "available"
		if skill.Relevant {
			marker = "loaded"
		}
		if skill.RequiresApproval {
			marker = "approval-required"
		}
		desc := skill.Description
		if desc == "" {
			desc = "Keine Beschreibung."
		}
		meta := ""
		if len(skill.Permissions) > 0 {
			meta += " permissions=" + strings.Join(skill.Permissions, ",")
		}
		if len(skill.Scripts) > 0 {
			meta += " scripts=" + strings.Join(skill.Scripts, ",")
		}
		if skill.Priority != 0 {
			meta += fmt.Sprintf(" priority=%d", skill.Priority)
		}
		if len(skill.Activation) > 0 {
			meta += " activation=" + strings.Join(skill.Activation, ",")
		}
		if len(skill.ExcludeGlobs) > 0 {
			meta += " exclude=" + strings.Join(skill.ExcludeGlobs, ",")
		}
		lines = append(lines, fmt.Sprintf("- %s [%s] %s: %s%s", skill.Name, marker, displayInstructionPath(project, skill.Path), desc, meta))
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
	approval := "none"
	if skill.RequiresApproval {
		approval = "required for declared permissions/scripts; skill text does not grant tool access by itself"
	}
	return fmt.Sprintf("--- Skill: %s ---\nPath: %s\nRelevant: %t\nApproval: %s\nPermissions: %s\nScripts: %s\n\n%s", skill.Name, displayInstructionPath(project, skill.Path), skill.Relevant, approval, strings.Join(skill.Permissions, ", "), strings.Join(skill.Scripts, ", "), content), nil
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
	skill, full, rel, err := resolveSkillResourcePath(project, name, resource)
	if err != nil {
		return "", err
	}
	content, ok := readInstructionFile(full, maxSkillResourceBytes)
	if !ok {
		return "", fmt.Errorf("skill resource %q is missing, empty, binary, or too large to read as text", resource)
	}
	return fmt.Sprintf("--- Skill-Ressource: %s / %s ---\nPath: %s\n\n%s", skill.Name, filepath.ToSlash(rel), displayInstructionPath(project, full), content), nil
}

func copySkillResource(project, name, resource, destination string) (string, error) {
	skill, full, rel, err := resolveSkillResourcePath(project, name, resource)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	if len(data) > maxSkillCopyResourceBytes {
		return "", fmt.Errorf("skill resource exceeds %d bytes", maxSkillCopyResourceBytes)
	}
	post, err := writeBinaryProjectFile(project, destination, data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SKILL RESOURCE COPIED\nSkill: %s\nResource: %s\nDestination: %s\nBytes: %d\n\n%s", skill.Name, filepath.ToSlash(rel), filepath.ToSlash(destination), len(data), post), nil
}

func resolveSkillResourcePath(project, name, resource string) (localSkillSummary, string, string, error) {
	skill, err := findSkillByName(project, name)
	if err != nil {
		return localSkillSummary{}, "", "", err
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return localSkillSummary{}, "", "", errors.New("resource is empty")
	}
	if filepath.IsAbs(resource) {
		return localSkillSummary{}, "", "", errors.New("resource must be relative to the skill directory")
	}
	root := filepath.Dir(skill.Path)
	full := filepath.Clean(filepath.Join(root, resource))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return localSkillSummary{}, "", "", errors.New("resource escapes the skill directory")
	}
	if strings.EqualFold(filepath.Clean(full), filepath.Clean(skill.Path)) {
		return localSkillSummary{}, "", "", errors.New("use skill_read for SKILL.md")
	}
	info, err := os.Stat(full)
	if err != nil {
		return localSkillSummary{}, "", "", err
	}
	if !info.Mode().IsRegular() {
		return localSkillSummary{}, "", "", fmt.Errorf("resource is not a regular file: %s", filepath.ToSlash(rel))
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return localSkillSummary{}, "", "", err
	}
	canonicalFull, err := filepath.EvalSymlinks(full)
	if err != nil {
		return localSkillSummary{}, "", "", err
	}
	canonicalRel, err := filepath.Rel(canonicalRoot, canonicalFull)
	if err != nil || canonicalRel == "." || strings.HasPrefix(canonicalRel, "..") || filepath.IsAbs(canonicalRel) {
		return localSkillSummary{}, "", "", errors.New("resource escapes the skill directory")
	}
	return skill, full, rel, nil
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

func skillMetadataRequiresApproval(permissions, scripts []string) bool {
	if len(scripts) > 0 {
		return true
	}
	for _, permission := range permissions {
		if skillPermissionIsReadOnly(permission) {
			continue
		}
		return true
	}
	return false
}

func skillPermissionIsReadOnly(permission string) bool {
	p := strings.ToLower(strings.TrimSpace(permission))
	if p == "" {
		return true
	}
	if i := strings.IndexAny(p, "(:"); i >= 0 {
		p = strings.TrimSpace(p[:i])
	}
	p = strings.ReplaceAll(p, "-", "_")
	p = strings.ReplaceAll(p, " ", "_")
	readOnly := map[string]bool{
		"read":                 true,
		"readonly":             true,
		"read_only":            true,
		"view":                 true,
		"cat":                  true,
		"ls":                   true,
		"tree":                 true,
		"glob":                 true,
		"grep":                 true,
		"list":                 true,
		"list_files":           true,
		"read_file":            true,
		"search":               true,
		"search_text":          true,
		"project_info":         true,
		"subagent_analyze":     true,
		"skill_list":           true,
		"skill_read":           true,
		"skill_list_resources": true,
		"skill_read_resource":  true,
		"command_list":         true,
		"command_read":         true,
		"mcp_list_tools":       true,
		"mcp_list_resources":   true,
		"mcp_read_resource":    true,
		"mcp_list_prompts":     true,
		"mcp_get_prompt":       true,
	}
	if readOnly[p] {
		return true
	}
	unsafeMarkers := []string{
		"write", "edit", "replace", "delete", "remove", "move", "copy", "create",
		"shell", "bash", "powershell", "cmd", "terminal", "exec", "execute", "run",
		"network", "http", "fetch", "web", "mcp_call", "tool", "deploy", "build",
		"git", "memory", "install",
	}
	for _, marker := range unsafeMarkers {
		if strings.Contains(p, marker) {
			return false
		}
	}
	return false
}

func activationMatchesTask(task string, activation []string) bool {
	task = strings.ToLower(task)
	for _, value := range activation {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if strings.Contains(task, value) {
			return true
		}
		for _, word := range significantWords(value) {
			if strings.Contains(task, word) {
				return true
			}
		}
	}
	return false
}

func frontmatterListAny(content string, keys ...string) []string {
	var out []string
	for _, key := range keys {
		out = append(out, frontmatterList(content, key)...)
	}
	return compactNonEmpty(out)
}

func frontmatterInt(content, key string) int {
	value := strings.TrimSpace(frontmatterValue(content, key))
	if value == "" {
		return 0
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return n
}

func frontmatterList(content, key string) []string {
	values := frontmatterValues(content, key)
	if len(values) == 0 {
		return nil
	}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			value = strings.Trim(value, "[]")
			for _, part := range splitFrontmatterInlineList(value) {
				part = trimFrontmatterListItem(part)
				if part != "" {
					out = append(out, part)
				}
			}
			continue
		}
		for _, part := range splitFrontmatterInlineList(value) {
			part = trimFrontmatterListItem(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func splitFrontmatterInlineList(value string) []string {
	var parts []string
	var current strings.Builder
	var quote rune
	for _, r := range value {
		if quote != 0 {
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			current.WriteRune(r)
			continue
		}
		if r == ',' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(r)
	}
	parts = append(parts, current.String())
	return parts
}

func trimFrontmatterListItem(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first := value[0]
		last := value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
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
	values := frontmatterValues(content, key)
	if len(values) == 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(values[0]), `"'`)
}

func frontmatterValues(content, key string) []string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil
	}
	rest := strings.TrimPrefix(content, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil
	}
	key = strings.ToLower(key)
	var values []string
	lines := strings.Split(strings.ReplaceAll(rest[:end], "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 || strings.ToLower(strings.TrimSpace(parts[0])) != key {
			continue
		}
		value := strings.TrimSpace(parts[1])
		if value != "" {
			values = append(values, value)
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			if strings.TrimSpace(next) == "" {
				continue
			}
			if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
				break
			}
			item := strings.TrimSpace(next)
			item = strings.TrimPrefix(item, "-")
			item = strings.TrimSpace(item)
			if item != "" {
				values = append(values, item)
			}
		}
	}
	return values
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
