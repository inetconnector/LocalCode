// SPDX-License-Identifier: Apache-2.0

package main

import (
	"path"
	"path/filepath"
	"strings"
)

func codeGraphExtractImportSpecs(content string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 12)
	add := func(spec string) {
		spec = codeGraphNormalizeImportSpec(spec)
		if spec == "" || seen[spec] {
			return
		}
		seen[spec] = true
		out = append(out, spec)
	}
	lines := strings.Split(content, "\n")
	if len(lines) > 900 {
		lines = lines[:900]
	}
	inGoImportBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if inGoImportBlock {
			if trimmed == ")" {
				inGoImportBlock = false
				continue
			}
			if spec := codeGraphQuotedImportSpec(trimmed); spec != "" {
				add(spec)
			}
			continue
		}
		if trimmed == "import (" {
			inGoImportBlock = true
			continue
		}
		lower := strings.ToLower(trimmed)
		switch {
		case strings.HasPrefix(lower, "import "):
			if spec := codeGraphQuotedImportSpec(trimmed); spec != "" {
				add(spec)
				continue
			}
			value := strings.TrimSpace(trimmed[len("import "):])
			for _, part := range strings.Split(value, ",") {
				fields := strings.Fields(strings.TrimSpace(part))
				if len(fields) > 0 {
					add(fields[0])
				}
			}
		case strings.HasPrefix(lower, "from "):
			fields := strings.Fields(trimmed)
			if len(fields) >= 2 {
				add(fields[1])
			}
		case strings.Contains(lower, "require("):
			if spec := codeGraphQuotedImportSpec(trimmed); spec != "" {
				add(spec)
			}
		case strings.HasPrefix(lower, "use "):
			value := strings.TrimSpace(strings.TrimSuffix(trimmed[len("use "):], ";"))
			value = strings.Split(value, "::{")[0]
			add(value)
		case strings.HasPrefix(lower, "using "):
			value := strings.TrimSpace(strings.TrimSuffix(trimmed[len("using "):], ";"))
			add(value)
		case strings.HasPrefix(lower, "#include "):
			if spec := codeGraphQuotedImportSpec(trimmed); spec != "" {
				add(spec)
			}
		}
	}
	return out
}

func codeGraphQuotedImportSpec(text string) string {
	for _, quote := range []byte{'"', '\'', '`'} {
		start := strings.IndexByte(text, quote)
		if start < 0 || start+1 >= len(text) {
			continue
		}
		endRel := strings.IndexByte(text[start+1:], quote)
		if endRel < 0 {
			continue
		}
		return text[start+1 : start+1+endRel]
	}
	return ""
}

func codeGraphNormalizeImportSpec(spec string) string {
	spec = strings.TrimSpace(spec)
	spec = strings.Trim(spec, `"'`+"`")
	spec = strings.TrimSuffix(spec, ";")
	spec = strings.ReplaceAll(spec, "\\", "/")
	spec = strings.ReplaceAll(spec, "::", "/")
	if strings.HasPrefix(spec, "crate/") {
		spec = strings.TrimPrefix(spec, "crate/")
	}
	if strings.HasPrefix(spec, "self/") {
		spec = strings.TrimPrefix(spec, "self/")
	}
	// Python relative imports encode directory traversal with leading dots.
	// Convert `.foo.bar` to `./foo/bar` and `..foo` to `../foo` so the
	// path-based resolver can use the same logic as JS/TS relative imports.
	if strings.HasPrefix(spec, ".") && !strings.Contains(spec, "/") {
		dots := 0
		for dots < len(spec) && spec[dots] == '.' {
			dots++
		}
		rest := strings.TrimPrefix(spec[dots:], ".")
		rest = strings.ReplaceAll(rest, ".", "/")
		prefix := "./"
		if dots > 1 {
			prefix = strings.Repeat("../", dots-1)
		}
		return strings.TrimSuffix(prefix+rest, "/")
	}
	if !strings.Contains(spec, "/") && strings.Contains(spec, ".") {
		spec = strings.ReplaceAll(spec, ".", "/")
	}
	return strings.TrimSpace(spec)
}

func buildCodeGraphEdges(files []codeGraphFile) ([]map[int]bool, []map[int]bool) {
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
	adjacency := make([]map[int]bool, len(files))
	reverse := make([]map[int]bool, len(files))
	for i := range files {
		adjacency[i] = make(map[int]bool)
		reverse[i] = make(map[int]bool)
	}

	addEdge := func(source, target int) {
		if source == target || adjacency[source][target] {
			return
		}
		adjacency[source][target] = true
		reverse[target][source] = true
	}

	// First establish explicit dependency edges from imports. This provides
	// package/module evidence even when a language construct is not covered by
	// the lightweight symbol extractor.
	for source := range files {
		for target := range files {
			if source == target {
				continue
			}
			if codeGraphImportMatchesTarget(files[source], files[target]) {
				addEdge(source, target)
			}
		}
	}

	// Then add symbol-reference edges. Unique definitions are safe to connect
	// repo-wide. Ambiguous names are connected only when import evidence (or a
	// same-directory/package relationship) disambiguates the target.
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
				if len(targets) == 1 {
					if !sourceDeclares {
						addEdge(source, target)
					}
					continue
				}
				if adjacency[source][target] || codeGraphSamePackageArea(files[source], files[target]) {
					addEdge(source, target)
				}
			}
		}
	}
	for i := range files {
		files[i].Outbound = len(adjacency[i])
		files[i].Inbound = len(reverse[i])
	}
	return adjacency, reverse
}

func codeGraphImportMatchesTarget(source, target codeGraphFile) bool {
	if len(source.Imports) == 0 {
		return false
	}
	targetPath := strings.ToLower(filepath.ToSlash(target.Path))
	targetNoExt := strings.TrimSuffix(targetPath, strings.ToLower(filepath.Ext(targetPath)))
	targetDir := path.Dir(targetNoExt)
	targetBase := path.Base(targetNoExt)
	sourceDir := path.Dir(strings.ToLower(filepath.ToSlash(source.Path)))

	for _, raw := range source.Imports {
		spec := strings.ToLower(codeGraphNormalizeImportSpec(raw))
		if spec == "" {
			continue
		}
		if strings.HasPrefix(spec, ".") {
			resolved := path.Clean(path.Join(sourceDir, spec))
			if resolved == targetNoExt || resolved == targetDir || resolved+"/index" == targetNoExt {
				return true
			}
			continue
		}
		if spec == targetPath || spec == targetNoExt || spec == targetDir {
			return true
		}
		if targetDir != "." && targetDir != "" && strings.HasSuffix(spec, "/"+targetDir) {
			return true
		}
		if strings.Contains(spec, "/") && strings.HasSuffix(targetNoExt, "/"+spec) {
			return true
		}
		if len(targetBase) >= 4 && (strings.HasSuffix(spec, "/"+targetBase) || spec == targetBase) {
			return true
		}
	}
	return false
}

func codeGraphSamePackageArea(source, target codeGraphFile) bool {
	sourceDir := path.Dir(strings.ToLower(filepath.ToSlash(source.Path)))
	targetDir := path.Dir(strings.ToLower(filepath.ToSlash(target.Path)))
	return sourceDir == targetDir && sourceDir != "."
}
