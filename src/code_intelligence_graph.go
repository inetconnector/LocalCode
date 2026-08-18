// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	err := filepath.WalkDir(project, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == project {
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
		rel, relErr := filepath.Rel(project, path)
		if relErr != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil
		}
		content, readErr := repoIntelReadLimited(path, codeGraphMaxFileBytes)
		if readErr != nil {
			return nil
		}
		item := codeGraphFile{
			Path:        filepath.ToSlash(rel),
			Language:    language,
			Content:     content,
			Symbols:     repoIntelExtractSymbols(content),
			Identifiers: codeGraphIdentifiers(content),
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
	path := strings.ToLower(item.Path)
	base := strings.ToLower(filepath.Base(item.Path))
	content := strings.ToLower(item.Content)
	score := 0.0
	for _, term := range terms {
		if strings.Contains(base, term) {
			score += 18
		} else if strings.Contains(path, term) {
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

func buildCodeGraphEdges(files []codeGraphFile) ([]map[int]bool, []map[int]bool) {
	definitions := make(map[string][]int)
	for i := range files {
		for _, symbol := range files[i].Symbols {
			if len(symbol) < 3 || codeGraphNoiseIdentifier(symbol) {
				continue
			}
			definitions[symbol] = append(definitions[symbol], i)
		}
	}
	adjacency := make([]map[int]bool, len(files))
	reverse := make([]map[int]bool, len(files))
	for i := range files {
		adjacency[i] = make(map[int]bool)
		reverse[i] = make(map[int]bool)
	}
	for source := range files {
		for identifier := range files[source].Identifiers {
			for _, target := range definitions[identifier] {
				if source == target {
					continue
				}
				adjacency[source][target] = true
				reverse[target][source] = true
			}
		}
	}
	for i := range files {
		files[i].Outbound = len(adjacency[i])
		files[i].Inbound = len(reverse[i])
	}
	return adjacency, reverse
}

func applyCodeGraphRanks(files []codeGraphFile, adjacency, reverse []map[int]bool) {
	n := len(files)
	if n == 0 {
		return
	}
	ranks := make([]float64, n)
	for i := range ranks {
		ranks[i] = 1.0 / float64(n)
	}
	for iteration := 0; iteration < 10; iteration++ {
		next := make([]float64, n)
		base := 0.15 / float64(n)
		for i := range next {
			next[i] = base
		}
		for source := range files {
			if len(adjacency[source]) == 0 {
				share := 0.85 * ranks[source] / float64(n)
				for target := range next {
					next[target] += share
				}
				continue
			}
			share := 0.85 * ranks[source] / float64(len(adjacency[source]))
			for target := range adjacency[source] {
				next[target] += share
			}
		}
		ranks = next
	}
	maxRank := 0.0
	for _, rank := range ranks {
		if rank > maxRank {
			maxRank = rank
		}
	}
	if maxRank == 0 {
		maxRank = 1
	}
	for i := range files {
		files[i].Rank = ranks[i] / maxRank
		files[i].TaskScore = files[i].BaseScore + files[i].Rank*16 + float64(min(files[i].Inbound, 12))*1.5
	}
	// Propagate task relevance one hop in both directions. This surfaces shared
	// APIs called by a task-relevant file as well as callers likely to require
	// coordinated changes, without letting graph centrality dominate the task.
	baseScores := make([]float64, n)
	for i := range files {
		baseScores[i] = files[i].BaseScore
	}
	for source, score := range baseScores {
		if score <= 0 {
			continue
		}
		boost := minFloat(12, score*0.28)
		for target := range adjacency[source] {
			files[target].TaskScore += boost
		}
		for caller := range reverse[source] {
			files[caller].TaskScore += boost * 0.75
		}
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func formatCodeGraph(files []codeGraphFile, adjacency, reverse []map[int]bool, task string) string {
	indices := make([]int, len(files))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(a, b int) bool {
		i, j := indices[a], indices[b]
		if files[i].TaskScore != files[j].TaskScore {
			return files[i].TaskScore > files[j].TaskScore
		}
		return files[i].Path < files[j].Path
	})
	central := append([]int(nil), indices...)
	sort.SliceStable(central, func(a, b int) bool {
		i, j := central[a], central[b]
		if files[i].Rank != files[j].Rank {
			return files[i].Rank > files[j].Rank
		}
		if files[i].Inbound != files[j].Inbound {
			return files[i].Inbound > files[j].Inbound
		}
		return files[i].Path < files[j].Path
	})

	var b strings.Builder
	b.WriteString("CODE INTELLIGENCE GRAPH\n")
	fmt.Fprintf(&b, "Indexed %d source files. Ranking combines task evidence, cross-file symbol references, and PageRank-style centrality.\n", len(files))
	b.WriteString("This index is deterministic and local; it remains available when no language server is installed.\n\n")
	b.WriteString("TASK-RELEVANT GRAPH NEIGHBORHOOD\n")
	for pos, idx := range indices {
		if pos >= codeGraphTopRelevant {
			break
		}
		item := files[idx]
		fmt.Fprintf(&b, "- %s (%s, relevance=%.1f, centrality=%.2f, refs in/out=%d/%d)", item.Path, item.Language, item.TaskScore, item.Rank, item.Inbound, item.Outbound)
		if len(item.Symbols) > 0 {
			symbols := item.Symbols
			if len(symbols) > 12 {
				symbols = symbols[:12]
			}
			b.WriteString(" symbols: " + strings.Join(symbols, ", "))
		}
		b.WriteString("\n")
		for _, neighbor := range codeGraphTopNeighbors(idx, files, adjacency, reverse, 4) {
			b.WriteString("    ↳ " + neighbor + "\n")
		}
	}

	b.WriteString("\nHIGH-CENTRALITY API / ARCHITECTURE ANCHORS\n")
	for pos, idx := range central {
		if pos >= codeGraphTopCentral {
			break
		}
		item := files[idx]
		fmt.Fprintf(&b, "- %s centrality=%.2f refs-in=%d symbols=%s\n", item.Path, item.Rank, item.Inbound, strings.Join(limitStrings(item.Symbols, 10), ", "))
	}

	b.WriteString("\nSTATIC SYMBOL NAVIGATION FOR TASK\n")
	navigation := codeGraphTaskNavigation(files, reverse, task)
	if navigation == "" {
		b.WriteString("- No task term matched a declared symbol exactly enough for static navigation. Use search_text/read_file or an installed language server if deeper semantic resolution is needed.\n")
	} else {
		b.WriteString(navigation)
	}
	b.WriteString("\nGRAPH RELIABILITY RULES\n")
	b.WriteString("- Before changing a shared high-centrality symbol, inspect its callers/references and paired tests.\n")
	b.WriteString("- Prefer files with both task evidence and graph connectivity over isolated textual matches.\n")
	b.WriteString("- Static navigation is a conservative fallback, not a substitute for compiler/type-checker verification.\n")
	return truncateText(b.String(), 46000)
}

func codeGraphTopNeighbors(index int, files []codeGraphFile, adjacency, reverse []map[int]bool, limit int) []string {
	type neighbor struct {
		Index int
		Kind  string
		Score float64
	}
	items := make([]neighbor, 0, len(adjacency[index])+len(reverse[index]))
	seen := map[int]bool{}
	for target := range adjacency[index] {
		seen[target] = true
		items = append(items, neighbor{Index: target, Kind: "uses", Score: files[target].TaskScore + files[target].Rank*5})
	}
	for caller := range reverse[index] {
		if seen[caller] {
			continue
		}
		items = append(items, neighbor{Index: caller, Kind: "used-by", Score: files[caller].TaskScore + files[caller].Rank*5})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		return files[items[i].Index].Path < files[items[j].Index].Path
	})
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Kind+" "+files[item.Index].Path)
	}
	return out
}

func codeGraphTaskNavigation(files []codeGraphFile, reverse []map[int]bool, task string) string {
	terms := repoIntelTerms(task)
	type match struct {
		FileIndex int
		Symbol    string
		Score     float64
	}
	var matches []match
	seen := map[string]bool{}
	for i := range files {
		for _, symbol := range files[i].Symbols {
			lower := strings.ToLower(symbol)
			matched := false
			for _, term := range terms {
				if len(term) >= 4 && (strings.Contains(lower, term) || strings.Contains(term, lower)) {
					matched = true
					break
				}
			}
			key := files[i].Path + "\x00" + symbol
			if matched && !seen[key] {
				seen[key] = true
				matches = append(matches, match{FileIndex: i, Symbol: symbol, Score: files[i].TaskScore + files[i].Rank*10 + float64(files[i].Inbound)})
			}
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return files[matches[i].FileIndex].Path < files[matches[j].FileIndex].Path
	})
	if len(matches) > codeGraphMaxNavSymbols {
		matches = matches[:codeGraphMaxNavSymbols]
	}
	var b strings.Builder
	for _, item := range matches {
		file := files[item.FileIndex]
		line := codeGraphDefinitionLine(file.Content, item.Symbol)
		fmt.Fprintf(&b, "- definition %s → %s", item.Symbol, file.Path)
		if line > 0 {
			fmt.Fprintf(&b, ":%d", line)
		}
		b.WriteString("\n")
		callers := make([]int, 0, len(reverse[item.FileIndex]))
		for caller := range reverse[item.FileIndex] {
			if files[caller].Identifiers[item.Symbol] {
				callers = append(callers, caller)
			}
		}
		sort.SliceStable(callers, func(i, j int) bool {
			if files[callers[i]].TaskScore != files[callers[j]].TaskScore {
				return files[callers[i]].TaskScore > files[callers[j]].TaskScore
			}
			return files[callers[i]].Path < files[callers[j]].Path
		})
		if len(callers) > codeGraphMaxReferences {
			callers = callers[:codeGraphMaxReferences]
		}
		for _, caller := range callers {
			lines := codeGraphReferenceLines(files[caller].Content, item.Symbol, 3)
			if len(lines) == 0 {
				b.WriteString("    reference ← " + files[caller].Path + "\n")
				continue
			}
			for _, line := range lines {
				fmt.Fprintf(&b, "    reference ← %s:%d\n", files[caller].Path, line)
			}
		}
	}
	return b.String()
}

func codeGraphDefinitionLine(content, symbol string) int {
	for index, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, symbol) {
			continue
		}
		for _, pattern := range repoIntelSymbolPatterns {
			match := pattern.FindStringSubmatch(line)
			if len(match) >= 2 && match[1] == symbol {
				return index + 1
			}
		}
	}
	return 0
}

func codeGraphReferenceLines(content, symbol string, limit int) []int {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(symbol) + `\b`)
	var lines []int
	for index, line := range strings.Split(content, "\n") {
		if pattern.MatchString(line) {
			lines = append(lines, index+1)
			if len(lines) >= limit {
				break
			}
		}
	}
	return lines
}

func limitStrings(values []string, limit int) []string {
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}
