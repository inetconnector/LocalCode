// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	codeGraphMaxFiles       = 1200
	codeGraphMaxFileBytes   = 384 * 1024
	codeGraphMaxIdentifiers = 1200
	codeGraphTopRelevant    = 20
	codeGraphTopCentral     = 14
	codeGraphMaxNavSymbols  = 10
	codeGraphMaxReferences  = 12
)

type codeGraphFile struct {
	Path        string
	Language    string
	Content     string
	Symbols     []string
	Identifiers map[string]bool
	Imports     []string
	BaseScore   float64
	TaskScore   float64
	Rank        float64
	Inbound     int
	Outbound    int
}

var codeGraphIdentifierPattern = regexp.MustCompile(`[A-Za-z_$][A-Za-z0-9_$]{2,}`)

func repositoryReferenceGraph(project, task string) (string, error) {
	files, err := buildCodeGraphFiles(project, task)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "CODE INTELLIGENCE GRAPH\nNo supported source files were found.\n", nil
	}
	adjacency, reverse := buildCodeGraphEdges(files)
	applyCodeGraphRanks(files, adjacency, reverse)
	return formatCodeGraph(files, adjacency, reverse, task), nil
}

func buildCodeGraphFiles(project, task string) ([]codeGraphFile, error) {
	project = filepath.Clean(project)
	terms := repoIntelTerms(task)
	files := make([]codeGraphFile, 0, 128)
	err := filepath.WalkDir(project, func(pathName string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if pathName == project {
			return nil
		}
		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if repoIntelIgnoredDirs[name] || strings.HasPrefix(name, ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || len(files) >= codeGraphMaxFiles {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		language, ok := repoIntelCodeExtensions[ext]
		if !ok {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() > codeGraphMaxFileBytes {
			return nil
		}
		rel, relErr := filepath.Rel(project, pathName)
		if relErr != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		content, readErr := repoIntelReadLimited(pathName, codeGraphMaxFileBytes)
		if readErr != nil {
			return nil
		}
		item := codeGraphFile{
			Path:        filepath.ToSlash(rel),
			Language:    language,
			Content:     content,
			Symbols:     repoIntelExtractSymbols(content),
			Identifiers: codeGraphIdentifiers(content),
			Imports:     codeGraphExtractImportSpecs(content),
		}
		item.BaseScore = codeGraphTaskScore(item, terms)
		item.TaskScore = item.BaseScore
		files = append(files, item)
		return nil
	})
	return files, err
}

func codeGraphIdentifiers(content string) map[string]bool {
	out := make(map[string]bool)
	for _, id := range codeGraphIdentifierPattern.FindAllString(content, -1) {
		if len(id) < 3 || codeGraphNoiseIdentifier(id) {
			continue
		}
		out[id] = true
		if len(out) >= codeGraphMaxIdentifiers {
			break
		}
	}
	return out
}

func codeGraphNoiseIdentifier(id string) bool {
	switch strings.ToLower(id) {
	case "the", "this", "that", "true", "false", "null", "nil", "none", "string", "number", "object", "return", "import", "package", "public", "private", "protected", "internal", "static", "class", "struct", "interface", "function", "const", "var", "let", "func", "type", "async", "await", "void", "int", "bool", "error", "context", "main", "test":
		return true
	default:
		return false
	}
}

func codeGraphTaskScore(item codeGraphFile, terms []string) float64 {
	pathName := strings.ToLower(item.Path)
	base := strings.ToLower(filepath.Base(item.Path))
	content := strings.ToLower(item.Content)
	score := 0.0
	for _, term := range terms {
		if strings.Contains(base, term) {
			score += 18
		} else if strings.Contains(pathName, term) {
			score += 9
		}
		count := strings.Count(content, term)
		if count > 6 {
			count = 6
		}
		score += float64(count * 3)
	}
	if repoIntelIsEntrypoint(item.Path) {
		score += 4
	}
	if repoIntelIsTestFile(item.Path) {
		score += 2
	}
	return score
}
