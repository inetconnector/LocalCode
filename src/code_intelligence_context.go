// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sort"
	"strings"
)

const (
	codeGraphContextTokenBudget = 2200
	codeGraphContextMaxFiles    = 12
	codeGraphContextRadius      = 6
)

type codeGraphContextChunk struct {
	FileIndex int
	StartLine int
	EndLine   int
	Reason    string
	Score     float64
	Text      string
	Tokens    int
}

// formatCodeGraphContext renders a compact, task-ranked source context before
// the broader graph report. This makes the graph immediately actionable for
// smaller local models: they receive the likely implementation snippets rather
// than having to spend turns reading whole files just to find a definition.
func formatCodeGraphContext(files []codeGraphFile, task string, tokenBudget int) string {
	if tokenBudget <= 0 {
		tokenBudget = codeGraphContextTokenBudget
	}
	chunks := codeGraphContextCandidates(files, task)
	if len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("TASK-RANKED SOURCE CONTEXT\n")
	b.WriteString("Budgeted, local snippets selected from task relevance + graph centrality + semantic definitions.\n")
	used := 0
	selected := 0
	for _, chunk := range chunks {
		if selected >= codeGraphContextMaxFiles {
			break
		}
		if chunk.Tokens <= 0 {
			continue
		}
		if used+chunk.Tokens > tokenBudget {
			continue
		}
		file := files[chunk.FileIndex]
		fmt.Fprintf(&b, "\n--- %s:%d-%d [%s; %s; score=%.1f; ~%d tokens] ---\n", file.Path, chunk.StartLine, chunk.EndLine, file.SemanticSource, chunk.Reason, chunk.Score, chunk.Tokens)
		b.WriteString(chunk.Text)
		if !strings.HasSuffix(chunk.Text, "\n") {
			b.WriteByte('\n')
		}
		used += chunk.Tokens
		selected++
	}
	fmt.Fprintf(&b, "\nCONTEXT BUDGET: ~%d/%d tokens across %d snippet(s).\n", used, tokenBudget, selected)
	return b.String()
}

func codeGraphContextCandidates(files []codeGraphFile, task string) []codeGraphContextChunk {
	terms := repoIntelTerms(task)
	var chunks []codeGraphContextChunk
	for index := range files {
		file := files[index]
		if strings.TrimSpace(file.Content) == "" {
			continue
		}
		lines := codeGraphPreferredDefinitionLines(file, terms)
		if len(lines) == 0 {
			lines = []int{1}
		}
		seenRanges := map[string]bool{}
		for _, candidate := range lines {
			start, end, text := codeGraphSnippet(file.Content, candidate, codeGraphContextRadius)
			if text == "" {
				continue
			}
			key := fmt.Sprintf("%d:%d", start, end)
			if seenRanges[key] {
				continue
			}
			seenRanges[key] = true
			reason := "ranked file"
			bonus := 0.0
			if codeGraphLineMatchesTask(file, candidate, terms) {
				reason = "task-matched definition"
				bonus = 18
			} else if candidate > 1 {
				reason = "semantic definition"
				bonus = 8
			}
			chunks = append(chunks, codeGraphContextChunk{
				FileIndex: index,
				StartLine: start,
				EndLine:   end,
				Reason:    reason,
				Score:     file.TaskScore + file.Rank*10 + bonus,
				Text:      text,
				Tokens:    codeGraphApproxTokens(text),
			})
			if len(seenRanges) >= 3 {
				break
			}
		}
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].Score != chunks[j].Score {
			return chunks[i].Score > chunks[j].Score
		}
		fi, fj := files[chunks[i].FileIndex], files[chunks[j].FileIndex]
		if fi.Path != fj.Path {
			return fi.Path < fj.Path
		}
		return chunks[i].StartLine < chunks[j].StartLine
	})
	return chunks
}

func codeGraphPreferredDefinitionLines(file codeGraphFile, terms []string) []int {
	type candidate struct {
		name  string
		line  int
		score int
	}
	items := make([]candidate, 0, len(file.DefinitionLines))
	for name, line := range file.DefinitionLines {
		if line <= 0 {
			continue
		}
		lower := strings.ToLower(name)
		score := 0
		for _, term := range terms {
			if strings.EqualFold(lower, term) {
				score += 40
			} else if strings.Contains(lower, term) || strings.Contains(term, lower) {
				score += 20
			}
		}
		items = append(items, candidate{name: name, line: line, score: score})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if items[i].line != items[j].line {
			return items[i].line < items[j].line
		}
		return items[i].name < items[j].name
	})
	out := make([]int, 0, 3)
	seen := map[int]bool{}
	for _, item := range items {
		if seen[item.line] {
			continue
		}
		seen[item.line] = true
		out = append(out, item.line)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func codeGraphLineMatchesTask(file codeGraphFile, line int, terms []string) bool {
	for name, definitionLine := range file.DefinitionLines {
		if definitionLine != line {
			continue
		}
		lower := strings.ToLower(name)
		for _, term := range terms {
			if strings.Contains(lower, term) || strings.Contains(term, lower) {
				return true
			}
		}
	}
	return false
}

func codeGraphSnippet(content string, center, radius int) (int, int, string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return 0, 0, ""
	}
	if center < 1 {
		center = 1
	}
	if center > len(lines) {
		center = len(lines)
	}
	start := center - radius
	if start < 1 {
		start = 1
	}
	end := center + radius
	if end > len(lines) {
		end = len(lines)
	}
	var b strings.Builder
	for line := start; line <= end; line++ {
		fmt.Fprintf(&b, "%5d | %s\n", line, lines[line-1])
	}
	return start, end, b.String()
}

func codeGraphApproxTokens(text string) int {
	if text == "" {
		return 0
	}
	// Source code averages roughly 3-4 bytes/token across common coding
	// tokenizers. Use the conservative end so the context stays under budget.
	tokens := (len([]byte(text)) + 2) / 3
	if tokens < 1 {
		return 1
	}
	return tokens
}
