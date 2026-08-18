// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type codeGraphRelationKind string

const (
	codeGraphRelationImport     codeGraphRelationKind = "imports"
	codeGraphRelationReference codeGraphRelationKind = "references"
	codeGraphRelationCall      codeGraphRelationKind = "calls"
	codeGraphRelationInherit   codeGraphRelationKind = "inherits"
	codeGraphRelationImplement codeGraphRelationKind = "implements"
	codeGraphRelationTestOf    codeGraphRelationKind = "test-of"
)

type codeGraphRelation struct {
	Kinds   map[codeGraphRelationKind]bool
	Symbols map[string]bool
}

type codeGraphRelations []map[int]*codeGraphRelation

func (r *codeGraphRelation) add(kind codeGraphRelationKind, symbol string) {
	if r.Kinds == nil {
		r.Kinds = make(map[codeGraphRelationKind]bool)
	}
	if r.Symbols == nil {
		r.Symbols = make(map[string]bool)
	}
	r.Kinds[kind] = true
	if symbol = strings.TrimSpace(symbol); symbol != "" {
		r.Symbols[symbol] = true
	}
}

func (r *codeGraphRelation) weight() float64 {
	if r == nil {
		return 0
	}
	weight := 0.0
	for kind := range r.Kinds {
		switch kind {
		case codeGraphRelationImport:
			weight += 2.4
		case codeGraphRelationReference:
			weight += 1.0
		case codeGraphRelationCall:
			weight += 3.2
		case codeGraphRelationInherit, codeGraphRelationImplement:
			weight += 4.0
		case codeGraphRelationTestOf:
			weight += 2.8
		}
	}
	if symbolCount := len(r.Symbols); symbolCount > 1 {
		bonus := float64(symbolCount-1) * 0.35
		if bonus > 1.4 {
			bonus = 1.4
		}
		weight += bonus
	}
	return weight
}

func (r *codeGraphRelation) labels() []string {
	if r == nil {
		return nil
	}
	order := []codeGraphRelationKind{
		codeGraphRelationImplement,
		codeGraphRelationInherit,
		codeGraphRelationCall,
		codeGraphRelationImport,
		codeGraphRelationTestOf,
		codeGraphRelationReference,
	}
	labels := make([]string, 0, len(r.Kinds))
	for _, kind := range order {
		if r.Kinds[kind] {
			labels = append(labels, string(kind))
		}
	}
	return labels
}

func (r *codeGraphRelation) symbolList(limit int) []string {
	if r == nil || len(r.Symbols) == 0 || limit <= 0 {
		return nil
	}
	out := make([]string, 0, len(r.Symbols))
	for symbol := range r.Symbols {
		out = append(out, symbol)
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func buildCodeGraphRelations(files []codeGraphFile) codeGraphRelations {
	definitions := make(map[string][]int)
	declares := make([]map[string]bool, len(files))
	for i := range files {
		declares[i] = make(map[string]bool)
		for _, symbol := range files[i].Symbols {
			if len(symbol) < 3 || codeGraphNoiseIdentifier(symbol) {
				continue
			}
			definitions[symbol] = append(definitions[symbol], i)
			declares[i][symbol] = true
		}
	}

	relations := make(codeGraphRelations, len(files))
	for i := range relations {
		relations[i] = make(map[int]*codeGraphRelation)
	}
	add := func(source, target int, kind codeGraphRelationKind, symbol string) {
		if source == target {
			return
		}
		relation := relations[source][target]
		if relation == nil {
			relation = &codeGraphRelation{}
			relations[source][target] = relation
		}
		relation.add(kind, symbol)
	}

	// Imports are the strongest deterministic namespace/package evidence and
	// are established before symbol relations so ambiguous names can use them
	// for target disambiguation.
	for source := range files {
		for target := range files {
			if source != target && codeGraphImportMatchesTarget(files[source], files[target]) {
				add(source, target, codeGraphRelationImport, "")
			}
		}
	}

	for source := range files {
		for identifier := range files[source].Identifiers {
			targets := definitions[identifier]
			if len(targets) == 0 {
				continue
			}
			sourceDeclares := declares[source][identifier]
			for _, target := range targets {
				if source == target {
					continue
				}
				allowed := false
				if len(targets) == 1 {
					allowed = !sourceDeclares
				} else {
					_, imported := relations[source][target]
					allowed = imported || codeGraphSamePackageArea(files[source], files[target])
				}
				if !allowed {
					continue
				}
				add(source, target, codeGraphRelationReference, identifier)
				if codeGraphLooksLikeCall(files[source].Content, identifier) {
					add(source, target, codeGraphRelationCall, identifier)
				}
				if codeGraphLooksLikeImplementation(files[source].Content, identifier) {
					add(source, target, codeGraphRelationImplement, identifier)
				} else if codeGraphLooksLikeInheritance(files[source].Content, identifier) {
					add(source, target, codeGraphRelationInherit, identifier)
				}
			}
		}
	}

	// Test relationships are useful for context selection even when a test only
	// exercises behavior indirectly. Pair by conventional filename where safe,
	// and otherwise annotate an already-proven dependency/reference relation.
	for source := range files {
		if !repoIntelIsTestFile(files[source].Path) {
			continue
		}
		for target := range files {
			if source == target || repoIntelIsTestFile(files[target].Path) {
				continue
			}
			_, alreadyRelated := relations[source][target]
			if alreadyRelated || codeGraphLikelyTestPair(files[source].Path, files[target].Path) {
				add(source, target, codeGraphRelationTestOf, "")
			}
		}
	}
	return relations
}

func codeGraphRelationAdjacency(files []codeGraphFile, relations codeGraphRelations) ([]map[int]bool, []map[int]bool) {
	adjacency := make([]map[int]bool, len(files))
	reverse := make([]map[int]bool, len(files))
	inWeight := make([]float64, len(files))
	outWeight := make([]float64, len(files))
	for i := range files {
		adjacency[i] = make(map[int]bool)
		reverse[i] = make(map[int]bool)
	}
	for source := range relations {
		for target, relation := range relations[source] {
			if target < 0 || target >= len(files) || relation == nil || relation.weight() <= 0 {
				continue
			}
			adjacency[source][target] = true
			reverse[target][source] = true
			weight := relation.weight()
			outWeight[source] += weight
			inWeight[target] += weight
		}
	}
	for i := range files {
		files[i].Outbound = len(adjacency[i])
		files[i].Inbound = len(reverse[i])
		files[i].OutboundWeight = outWeight[i]
		files[i].InboundWeight = inWeight[i]
	}
	return adjacency, reverse
}

func codeGraphLooksLikeCall(content, symbol string) bool {
	if symbol == "" {
		return false
	}
	pattern := regexp.MustCompile(`(?m)(?:\b|\.\s*)` + regexp.QuoteMeta(symbol) + `\s*(?:\[[^\]\n]*\]\s*)?\(`)
	return pattern.MatchString(content)
}

func codeGraphLooksLikeImplementation(content, symbol string) bool {
	if symbol == "" {
		return false
	}
	quoted := regexp.QuoteMeta(symbol)
	patterns := []string{
		`(?mi)\bimplements\s+[^\n{]*\b` + quoted + `\b`,
		`(?mi)\bimpl(?:\s*<[^>]*>)?\s+` + quoted + `\s+for\b`,
	}
	for _, raw := range patterns {
		if regexp.MustCompile(raw).MatchString(content) {
			return true
		}
	}
	return false
}

func codeGraphLooksLikeInheritance(content, symbol string) bool {
	if symbol == "" {
		return false
	}
	quoted := regexp.QuoteMeta(symbol)
	patterns := []string{
		`(?mi)\bextends\s+` + quoted + `\b`,
		`(?m)^\s*class\s+[A-Za-z_$][A-Za-z0-9_$]*\s*\([^\n)]*\b` + quoted + `\b[^\n)]*\)\s*:`,
		`(?m)^\s*(?:class|struct)\s+[A-Za-z_][A-Za-z0-9_]*[^\n{]*:\s*(?:public\s+|protected\s+|private\s+)?` + quoted + `\b`,
	}
	for _, raw := range patterns {
		if regexp.MustCompile(raw).MatchString(content) {
			return true
		}
	}
	return false
}

func codeGraphLikelyTestPair(testPath, targetPath string) bool {
	if !repoIntelIsTestFile(testPath) || repoIntelIsTestFile(targetPath) {
		return false
	}
	testDir := filepath.ToSlash(filepath.Dir(testPath))
	targetDir := filepath.ToSlash(filepath.Dir(targetPath))
	if testDir != targetDir {
		return false
	}
	strip := func(path string) string {
		base := strings.ToLower(filepath.Base(path))
		ext := strings.ToLower(filepath.Ext(base))
		base = strings.TrimSuffix(base, ext)
		base = strings.TrimSuffix(base, "_test")
		base = strings.TrimSuffix(base, ".test")
		base = strings.TrimSuffix(base, ".spec")
		return base
	}
	return strip(testPath) != "" && strip(testPath) == strip(targetPath)
}
