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

// buildCodeGraphEdges remains the compatibility view used by existing callers
// and tests. The authoritative graph is now typed; this collapses every proven
// relation to the legacy boolean adjacency/reverse representation.
func buildCodeGraphEdges(files []codeGraphFile) ([]map[int]bool, []map[int]bool) {
	relations := buildCodeGraphRelations(files)
	return codeGraphRelationAdjacency(files, relations)
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
