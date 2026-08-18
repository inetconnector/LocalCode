// SPDX-License-Identifier: Apache-2.0

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

const codeGraphMaxSymbolsPerFile = 96

type codeGraphSemanticFacts struct {
	Symbols         []string
	Identifiers     map[string]bool
	Imports         []string
	DefinitionLines map[string]int
	Source          string
}

// codeGraphExtractSemanticFacts uses the strongest deterministic parser LocalCode
// has for each file. Go keeps the standard-library compiler parser; supported
// polyglot files use embedded Tree-sitter grammars when CGO is available. The
// import-aware lexical extractor remains the zero-dependency fallback for other
// languages and for builds where CGO has deliberately been disabled.
func codeGraphExtractSemanticFacts(path, language, content string) codeGraphSemanticFacts {
	fallback := codeGraphLexicalSemanticFacts(content)
	if language == "Go" {
		facts, ok := codeGraphGoASTFacts(path, content)
		if ok {
			return facts
		}
		return fallback
	}
	if facts, ok := codeGraphTreeSitterFacts(path, content); ok {
		return facts
	}
	return fallback
}

func codeGraphLexicalSemanticFacts(content string) codeGraphSemanticFacts {
	facts := codeGraphSemanticFacts{
		Identifiers:     codeGraphIdentifiers(content),
		Imports:         codeGraphExtractImportSpecs(content),
		DefinitionLines: make(map[string]int),
		Source:          "lexical+imports",
	}
	seen := make(map[string]bool)
	lines := strings.Split(content, "\n")
	if len(lines) > 3200 {
		lines = lines[:3200]
	}
	for index, line := range lines {
		if len(line) > 700 {
			continue
		}
		for _, pattern := range repoIntelSymbolPatterns {
			match := pattern.FindStringSubmatch(line)
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if name == "" || seen[name] || codeGraphNoiseIdentifier(name) {
				break
			}
			seen[name] = true
			facts.Symbols = append(facts.Symbols, name)
			facts.DefinitionLines[name] = index + 1
			break
		}
		if len(facts.Symbols) >= codeGraphMaxSymbolsPerFile {
			break
		}
	}
	return facts
}

func codeGraphGoASTFacts(path, content string) (codeGraphSemanticFacts, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.AllErrors|parser.SkipObjectResolution)
	if file == nil {
		return codeGraphSemanticFacts{}, false
	}
	facts := codeGraphSemanticFacts{
		Identifiers:     make(map[string]bool),
		DefinitionLines: make(map[string]int),
		Source:          "go/ast",
	}
	seenSymbols := make(map[string]bool)
	seenImports := make(map[string]bool)
	addSymbol := func(name string, pos token.Pos) {
		name = strings.TrimSpace(name)
		if name == "" || name == "_" || seenSymbols[name] || codeGraphNoiseIdentifier(name) {
			return
		}
		seenSymbols[name] = true
		facts.Symbols = append(facts.Symbols, name)
		line := fset.PositionFor(pos, true).Line
		if line > 0 {
			facts.DefinitionLines[name] = line
		}
	}
	addIdentifier := func(name string) {
		if len(name) < 3 || len(facts.Identifiers) >= codeGraphMaxIdentifiers || codeGraphNoiseIdentifier(name) {
			return
		}
		facts.Identifiers[name] = true
	}

	for _, imp := range file.Imports {
		value, unquoteErr := strconv.Unquote(imp.Path.Value)
		if unquoteErr != nil {
			value = strings.Trim(imp.Path.Value, `"`)
		}
		value = codeGraphNormalizeImportSpec(value)
		if value != "" && !seenImports[value] {
			seenImports[value] = true
			facts.Imports = append(facts.Imports, value)
		}
	}

	for _, decl := range file.Decls {
		switch node := decl.(type) {
		case *ast.FuncDecl:
			addSymbol(node.Name.Name, node.Name.Pos())
			// Keep the raw method name for cross-file reference matching. The
			// existing import/package disambiguation prevents same-name methods in
			// unrelated packages from being connected blindly.
			addIdentifier(node.Name.Name)
		case *ast.GenDecl:
			for _, spec := range node.Specs {
				switch item := spec.(type) {
				case *ast.TypeSpec:
					addSymbol(item.Name.Name, item.Name.Pos())
					addIdentifier(item.Name.Name)
				case *ast.ValueSpec:
					for _, name := range item.Names {
						addSymbol(name.Name, name.Pos())
						addIdentifier(name.Name)
					}
				}
			}
		}
		if len(facts.Symbols) >= codeGraphMaxSymbolsPerFile {
			break
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok {
			addIdentifier(ident.Name)
		}
		return len(facts.Identifiers) < codeGraphMaxIdentifiers
	})

	// parser.ParseFile may return a useful partial AST together with syntax
	// errors. Keep those facts: they are still more structurally grounded than
	// regex-only extraction, while later syntax/build verification remains the
	// authority for correctness.
	_ = err
	return facts, len(facts.Symbols) > 0 || len(facts.Imports) > 0
}
