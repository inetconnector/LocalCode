// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	subagentMaxMentionedFiles = 8
	subagentMaxSearchTerms    = 10
	subagentMaxReportBytes    = 125000
	subagentMaxParallelReads  = 4
)

type subagentReadJob struct {
	name string
	run  func() (string, error)
}

type subagentReadResult struct {
	name   string
	output string
	err    error
}

// runSubagentReadJobs executes independent read-only repository probes with a
// strict concurrency bound, but always returns results in the same order as the
// input jobs. This gives local models faster repository exploration without
// introducing nondeterministic prompt ordering or parallel mutation hazards.
func runSubagentReadJobs(jobs []subagentReadJob, maxParallel int) []subagentReadResult {
	results := make([]subagentReadResult, len(jobs))
	if len(jobs) == 0 {
		return results
	}
	if maxParallel <= 0 {
		maxParallel = 1
	}
	if maxParallel > len(jobs) {
		maxParallel = len(jobs)
	}
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i := range jobs {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			output, err := jobs[i].run()
			results[i] = subagentReadResult{name: jobs[i].name, output: output, err: err}
		}()
	}
	wg.Wait()
	return results
}

func runReadOnlySubagent(project string, cfg Config, task string) (string, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return "", fmt.Errorf("subagent task is empty")
	}

	if projectIsEffectivelyEmpty(project) {
		return "READ-ONLY SUBAGENT HANDOFF\n" +
			"Scope: " + task + "\n" +
			"Project directory is new/empty: no existing source files and no detected build system.\n" +
			"HANDOFF\n" +
			"- There is nothing to analyze yet; do not keep asking about build systems or existing structure that does not exist.\n" +
			"- Choose a standard, idiomatic project layout for the language/platform named in the task and create it directly (e.g. .NET: a runnable SDK-style .csproj; Node: package.json; Python: pyproject.toml).\n" +
			"- Implement the full requested scope directly with write_file, then verify with the project's own build/run tools.\n", nil
	}

	mentioned := mentionedProjectFiles(task)
	if len(mentioned) > subagentMaxMentionedFiles {
		mentioned = mentioned[:subagentMaxMentionedFiles]
	}
	terms := subagentSearchTerms(task, mentioned)

	coreJobs := []subagentReadJob{
		{
			name: "PROJECT INFO",
			run: func() (string, error) {
				return projectInfo(project, cfg), nil
			},
		},
		{
			name: "PROJECT TREE",
			run: func() (string, error) {
				return projectTree(project, "", 4, 800)
			},
		},
		{
			name: "REPOSITORY INTELLIGENCE",
			run: func() (string, error) {
				return repositoryIntelligence(project, task)
			},
		},
		{
			name: "REFERENCE GRAPH / STATIC CODE NAVIGATION",
			run: func() (string, error) {
				return repositoryReferenceGraph(project, task)
			},
		},
	}
	core := runSubagentReadJobs(coreJobs, subagentMaxParallelReads)

	searchJobs := make([]subagentReadJob, 0, len(terms))
	for _, term := range terms {
		term := term
		searchJobs = append(searchJobs, subagentReadJob{
			name: term,
			run: func() (string, error) {
				return searchProject(project, term, "", 40)
			},
		})
	}
	searchResults := runSubagentReadJobs(searchJobs, subagentMaxParallelReads)

	var b strings.Builder
	b.WriteString("READ-ONLY SUBAGENT HANDOFF\n")
	b.WriteString("Scope: ")
	b.WriteString(task)
	b.WriteString("\nMode: read-only; bounded parallel repository exploration; no file writes, shell commands, network calls, MCP calls, or approvals.\n")
	b.WriteString(fmt.Sprintf("Parallel read limit: %d; result order: deterministic.\n\n", subagentMaxParallelReads))

	for i, result := range core {
		b.WriteString(result.name)
		b.WriteString("\n")
		if result.err != nil {
			switch i {
			case 1:
				b.WriteString("Project tree could not be built: ")
			case 2:
				b.WriteString("Repository intelligence could not be built: ")
			case 3:
				b.WriteString("Reference graph could not be built: ")
			default:
				b.WriteString("Read-only probe failed: ")
			}
			b.WriteString(result.err.Error())
			b.WriteString("\n\n")
			continue
		}
		limits := []int{8000, 12000, 42000, 46000}
		b.WriteString(truncateText(result.output, limits[i]))
		if !strings.HasSuffix(result.output, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("MENTIONED FILES\n")
	if len(mentioned) == 0 {
		b.WriteString("No concrete project files were mentioned.\n\n")
	} else {
		fileJobs := make([]subagentReadJob, 0, len(mentioned))
		for _, path := range mentioned {
			path := path
			fileJobs = append(fileJobs, subagentReadJob{
				name: path,
				run: func() (string, error) {
					return readProjectFile(project, path)
				},
			})
		}
		for _, result := range runSubagentReadJobs(fileJobs, subagentMaxParallelReads) {
			b.WriteString("--- ")
			b.WriteString(result.name)
			b.WriteString(" ---\n")
			if result.err != nil {
				b.WriteString("ERROR: ")
				b.WriteString(result.err.Error())
				b.WriteString("\n")
				continue
			}
			b.WriteString(truncateText(result.output, 9000))
			if !strings.HasSuffix(result.output, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("SEARCH EVIDENCE\n")
	if len(searchResults) == 0 {
		b.WriteString("No useful search terms extracted.\n")
	} else {
		for _, result := range searchResults {
			b.WriteString("--- query: ")
			b.WriteString(result.name)
			b.WriteString(" ---\n")
			if result.err != nil {
				b.WriteString("ERROR: ")
				b.WriteString(result.err.Error())
				b.WriteString("\n")
				continue
			}
			b.WriteString(truncateText(result.output, 7000))
			if !strings.HasSuffix(result.output, "\n") {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\nHANDOFF\n")
	b.WriteString("- Use this as an independent preflight, not as permission to mutate files.\n")
	b.WriteString("- Parallelism is deliberately restricted to independent reads; all mutation, approvals and verification remain serialized through the normal agent controls.\n")
	b.WriteString("- Form a concrete plan from architecture anchors, task-relevant files, graph neighbors, shared symbols, invariants, and likely tests before editing.\n")
	b.WriteString("- Inspect callers/references before changing a high-centrality or shared symbol.\n")
	b.WriteString("- Treat paths and line hits as leads; read the actual implementation before changing it.\n")
	b.WriteString("- Preserve unrelated behavior and public interfaces unless the user explicitly requests a breaking change.\n")
	b.WriteString("- After any later write, compare the observed diff to the intended outcome; a zero exit code alone is not proof of correctness.\n")
	b.WriteString("- Run the smallest relevant verification first, then the broader project-supported lint/test/build checks.\n")
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
