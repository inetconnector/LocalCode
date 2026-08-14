// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	subagentMaxMentionedFiles = 8
	subagentMaxSearchTerms    = 8
	subagentMaxReportBytes    = 60000
)

func runReadOnlySubagent(project string, cfg Config, task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", fmt.Errorf("subagent task is empty")
	}

	var b strings.Builder
	b.WriteString("READ-ONLY SUBAGENT HANDOFF\n")
	b.WriteString("Scope: ")
	b.WriteString(task)
	b.WriteString("\nMode: read-only; no file writes, shell commands, network calls, MCP calls, or approvals.\n\n")

	b.WriteString("PROJECT INFO\n")
	b.WriteString(truncateText(projectInfo(project, cfg), 8000))
	b.WriteString("\n")

	tree, err := projectTree(project, "", 4, 800)
	if err != nil {
		b.WriteString("PROJECT TREE ERROR\n")
		b.WriteString(err.Error())
		b.WriteString("\n\n")
	} else {
		b.WriteString("PROJECT TREE\n")
		b.WriteString(truncateText(tree, 12000))
		b.WriteString("\n")
	}

	mentioned := mentionedProjectFiles(task)
	if len(mentioned) > subagentMaxMentionedFiles {
		mentioned = mentioned[:subagentMaxMentionedFiles]
	}
	b.WriteString("MENTIONED FILES\n")
	if len(mentioned) == 0 {
		b.WriteString("No concrete project files were mentioned.\n\n")
	} else {
		for _, path := range mentioned {
			b.WriteString("--- ")
			b.WriteString(path)
			b.WriteString(" ---\n")
			content, readErr := readProjectFile(project, path)
			if readErr != nil {
				b.WriteString("ERROR: ")
				b.WriteString(readErr.Error())
				b.WriteString("\n")
				continue
			}
			b.WriteString(truncateText(content, 9000))
			if !strings.HasSuffix(content, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	terms := subagentSearchTerms(task, mentioned)
	b.WriteString("SEARCH EVIDENCE\n")
	if len(terms) == 0 {
		b.WriteString("No useful search terms extracted.\n")
	} else {
		for _, term := range terms {
			b.WriteString("--- query: ")
			b.WriteString(term)
			b.WriteString(" ---\n")
			hits, searchErr := searchProject(project, term, "", 40)
			if searchErr != nil {
				b.WriteString("ERROR: ")
				b.WriteString(searchErr.Error())
				b.WriteString("\n")
				continue
			}
			b.WriteString(truncateText(hits, 7000))
			if !strings.HasSuffix(hits, "\n") {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\nHANDOFF\n")
	b.WriteString("- Use the evidence above as an independent read-only exploration pass.\n")
	b.WriteString("- Treat paths and line hits as leads, not proof of correctness.\n")
	b.WriteString("- After any later write, run the smallest relevant verification command and re-check changed files.\n")
	return truncateText(b.String(), subagentMaxReportBytes), nil
}

func subagentSearchTerms(task string, mentioned []string) []string {
	seen := map[string]bool{}
	var terms []string
	add := func(term string) {
		term = strings.Trim(strings.ToLower(term), " \t\r\n.,;:!?()[]{}\"'")
		if len(term) < 4 || seen[term] || filepath.IsAbs(term) || strings.Contains(term, "://") {
			return
		}
		seen[term] = true
		terms = append(terms, term)
	}
	for _, word := range significantWords(task) {
		add(word)
		if len(terms) >= subagentMaxSearchTerms {
			return terms
		}
	}
	for _, path := range mentioned {
		base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		add(base)
		if len(terms) >= subagentMaxSearchTerms {
			return terms
		}
	}
	sort.Strings(terms)
	if len(terms) > subagentMaxSearchTerms {
		terms = terms[:subagentMaxSearchTerms]
	}
	return terms
}
