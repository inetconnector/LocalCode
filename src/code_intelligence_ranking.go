// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func applyCodeGraphRanks(files []codeGraphFile, adjacency, reverse []map[int]bool) {
	applyCodeGraphRanksInternal(files, nil, adjacency, reverse)
}

func applyCodeGraphRelationRanks(files []codeGraphFile, relations codeGraphRelations, adjacency, reverse []map[int]bool) {
	applyCodeGraphRanksInternal(files, relations, adjacency, reverse)
}

func applyCodeGraphRanksInternal(files []codeGraphFile, relations codeGraphRelations, adjacency, reverse []map[int]bool) {
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
			totalWeight := 0.0
			if relations != nil && source < len(relations) {
				for target := range adjacency[source] {
					if relation := relations[source][target]; relation != nil {
						totalWeight += relation.weight()
					}
				}
			}
			if totalWeight <= 0 {
				share := 0.85 * ranks[source] / float64(len(adjacency[source]))
				for target := range adjacency[source] {
					next[target] += share
				}
				continue
			}
			for target := range adjacency[source] {
				relation := relations[source][target]
				if relation == nil || relation.weight() <= 0 {
					continue
				}
				next[target] += 0.85 * ranks[source] * relation.weight() / totalWeight
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
		inboundSignal := float64(min(files[i].Inbound, 12)) * 1.5
		if relations != nil && files[i].InboundWeight > 0 {
			inboundSignal = minFloat(files[i].InboundWeight, 24) * 0.85
		}
		files[i].TaskScore = files[i].BaseScore + files[i].Rank*16 + inboundSignal
	}

	// Propagate task relevance one hop in both directions. Typed relations make
	// a proven call/implementation/test dependency matter more than a generic
	// identifier reference, while task evidence remains the primary signal.
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
			factor := codeGraphPropagationFactor(relations, source, target)
			files[target].TaskScore += boost * factor
		}
		for caller := range reverse[source] {
			factor := codeGraphPropagationFactor(relations, caller, source)
			files[caller].TaskScore += boost * 0.75 * factor
		}
	}
}

func codeGraphPropagationFactor(relations codeGraphRelations, source, target int) float64 {
	if relations == nil || source < 0 || source >= len(relations) {
		return 1
	}
	relation := relations[source][target]
	if relation == nil {
		return 1
	}
	weight := relation.weight()
	if weight <= 1 {
		return 0.85
	}
	factor := 0.75 + weight/5.5
	if factor > 1.75 {
		factor = 1.75
	}
	return factor
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func formatCodeGraph(files []codeGraphFile, adjacency, reverse []map[int]bool, task string) string {
	return formatCodeGraphWithRelations(files, nil, adjacency, reverse, task)
}

func formatCodeGraphWithRelations(files []codeGraphFile, relations codeGraphRelations, adjacency, reverse []map[int]bool, task string) string {
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
		if files[i].InboundWeight != files[j].InboundWeight {
			return files[i].InboundWeight > files[j].InboundWeight
		}
		if files[i].Inbound != files[j].Inbound {
			return files[i].Inbound > files[j].Inbound
		}
		return files[i].Path < files[j].Path
	})

	var b strings.Builder
	b.WriteString("CODE INTELLIGENCE GRAPH\n")
	if relations == nil {
		fmt.Fprintf(&b, "Indexed %d source files. Ranking combines task evidence, import-aware cross-file symbol references, and PageRank-style centrality.\n", len(files))
	} else {
		fmt.Fprintf(&b, "Indexed %d source files. Ranking combines task evidence with weighted typed relations (imports, references, calls, inherits, implements, test-of) and PageRank-style centrality.\n", len(files))
		b.WriteString("Semantic relation strength affects both centrality and task propagation; stronger structural evidence outranks a generic textual reference.\n")
	}
	b.WriteString("Ambiguous symbol names are linked only when package/import evidence disambiguates them; explicit local imports remain visible even when symbol extraction misses an export.\n")
	b.WriteString("This index is deterministic and local; it remains available when no language server is installed.\n\n")
	b.WriteString("TASK-RELEVANT GRAPH NEIGHBORHOOD\n")
	for pos, idx := range indices {
		if pos >= codeGraphTopRelevant {
			break
		}
		item := files[idx]
		fmt.Fprintf(&b, "- %s (%s, relevance=%.1f, centrality=%.2f, refs in/out=%d/%d", item.Path, item.Language, item.TaskScore, item.Rank, item.Inbound, item.Outbound)
		if relations != nil {
			fmt.Fprintf(&b, ", relation-weight in/out=%.1f/%.1f", item.InboundWeight, item.OutboundWeight)
		}
		b.WriteString(")")
		if len(item.Symbols) > 0 {
			symbols := item.Symbols
			if len(symbols) > 12 {
				symbols = symbols[:12]
			}
			b.WriteString(" symbols: " + strings.Join(symbols, ", "))
		}
		if len(item.Imports) > 0 {
			b.WriteString(" imports: " + strings.Join(limitStrings(item.Imports, 5), ", "))
		}
		if item.SemanticSource != "" {
			b.WriteString(" parser: " + item.SemanticSource)
		}
		b.WriteString("\n")
		for _, neighbor := range codeGraphTopNeighborsWithRelations(idx, files, relations, adjacency, reverse, 4) {
			b.WriteString("    ↳ " + neighbor + "\n")
		}
	}

	b.WriteString("\nHIGH-CENTRALITY API / ARCHITECTURE ANCHORS\n")
	for pos, idx := range central {
		if pos >= codeGraphTopCentral {
			break
		}
		item := files[idx]
		if relations == nil {
			fmt.Fprintf(&b, "- %s centrality=%.2f refs-in=%d symbols=%s\n", item.Path, item.Rank, item.Inbound, strings.Join(limitStrings(item.Symbols, 10), ", "))
		} else {
			fmt.Fprintf(&b, "- %s centrality=%.2f refs-in=%d relation-weight-in=%.1f symbols=%s\n", item.Path, item.Rank, item.Inbound, item.InboundWeight, strings.Join(limitStrings(item.Symbols, 10), ", "))
		}
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
	b.WriteString("- Prefer files with task evidence plus import/dependency evidence over isolated textual matches.\n")
	if relations != nil {
		b.WriteString("- Prefer typed call/inheritance/implementation/test evidence over generic references when selecting coordinated edits.\n")
	}
	b.WriteString("- Treat ambiguous same-name symbols conservatively unless package/import evidence identifies the target.\n")
	b.WriteString("- Static navigation is a conservative fallback, not a substitute for compiler/type-checker/LSP verification.\n")
	return truncateText(b.String(), 46000)
}

func codeGraphTopNeighbors(index int, files []codeGraphFile, adjacency, reverse []map[int]bool, limit int) []string {
	return codeGraphTopNeighborsWithRelations(index, files, nil, adjacency, reverse, limit)
}

func codeGraphTopNeighborsWithRelations(index int, files []codeGraphFile, relations codeGraphRelations, adjacency, reverse []map[int]bool, limit int) []string {
	type neighbor struct {
		Index int
		Kind  string
		Score float64
	}
	items := make([]neighbor, 0, len(adjacency[index])+len(reverse[index]))
	seen := map[int]bool{}
	for target := range adjacency[index] {
		seen[target] = true
		kind := "uses"
		weight := 1.0
		if relations != nil && index < len(relations) {
			if relation := relations[index][target]; relation != nil {
				kind = strings.Join(relation.labels(), "+")
				if symbols := relation.symbolList(3); len(symbols) > 0 {
					kind += " [" + strings.Join(symbols, ", ") + "]"
				}
				weight = relation.weight()
			}
		}
		items = append(items, neighbor{Index: target, Kind: kind, Score: files[target].TaskScore + files[target].Rank*5 + weight})
	}
	for caller := range reverse[index] {
		if seen[caller] {
			continue
		}
		kind := "used-by"
		weight := 1.0
		if relations != nil && caller < len(relations) {
			if relation := relations[caller][index]; relation != nil {
				kind = "used-by:" + strings.Join(relation.labels(), "+")
				if symbols := relation.symbolList(3); len(symbols) > 0 {
					kind += " [" + strings.Join(symbols, ", ") + "]"
				}
				weight = relation.weight()
			}
		}
		items = append(items, neighbor{Index: caller, Kind: kind, Score: files[caller].TaskScore + files[caller].Rank*5 + weight})
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
