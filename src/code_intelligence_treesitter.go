// SPDX-License-Identifier: Apache-2.0
//go:build cgo

package main

import (
	"path/filepath"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type codeGraphTreeSitterSpec struct {
	Language *treesitter.Language
	Label    string
}

func codeGraphTreeSitterSpecForPath(path string) (codeGraphTreeSitterSpec, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".mjs", ".cjs":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_javascript.Language()), Label: "javascript"}, true
	case ".ts":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_typescript.LanguageTypescript()), Label: "typescript"}, true
	case ".tsx":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_typescript.LanguageTSX()), Label: "tsx"}, true
	case ".py":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_python.Language()), Label: "python"}, true
	case ".rs":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_rust.Language()), Label: "rust"}, true
	case ".c":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_c.Language()), Label: "c"}, true
	case ".h", ".hh", ".hpp", ".cc", ".cpp", ".cxx":
		return codeGraphTreeSitterSpec{Language: treesitter.NewLanguage(tree_sitter_cpp.Language()), Label: "cpp"}, true
	default:
		return codeGraphTreeSitterSpec{}, false
	}
}

func codeGraphTreeSitterFacts(path, content string) (codeGraphSemanticFacts, bool) {
	spec, ok := codeGraphTreeSitterSpecForPath(path)
	if !ok || spec.Language == nil {
		return codeGraphSemanticFacts{}, false
	}
	parser := treesitter.NewParser()
	if parser == nil {
		return codeGraphSemanticFacts{}, false
	}
	defer parser.Close()
	if err := parser.SetLanguage(spec.Language); err != nil {
		return codeGraphSemanticFacts{}, false
	}
	source := []byte(content)
	tree := parser.Parse(source, nil)
	if tree == nil {
		return codeGraphSemanticFacts{}, false
	}
	defer tree.Close()
	root := tree.RootNode()
	if root == nil {
		return codeGraphSemanticFacts{}, false
	}

	facts := codeGraphSemanticFacts{
		Identifiers:     make(map[string]bool),
		Imports:         codeGraphExtractImportSpecs(content),
		DefinitionLines: make(map[string]int),
		Source:          "tree-sitter/" + spec.Label,
	}
	seenSymbols := make(map[string]bool)

	var walk func(*treesitter.Node, int)
	walk = func(node *treesitter.Node, depth int) {
		if node == nil || depth > 180 {
			return
		}
		kind := node.Kind()
		if codeGraphTreeSitterIdentifierKind(kind) {
			name := codeGraphTreeSitterNodeIdentifier(node, source, 0)
			if name != "" && len(facts.Identifiers) < codeGraphMaxIdentifiers && !codeGraphNoiseIdentifier(name) {
				facts.Identifiers[name] = true
			}
		}
		if len(facts.Symbols) < codeGraphMaxSymbolsPerFile && codeGraphTreeSitterSymbolNode(node) {
			name := codeGraphTreeSitterSymbolName(node, source)
			if name != "" && !seenSymbols[name] && !codeGraphNoiseIdentifier(name) {
				seenSymbols[name] = true
				facts.Symbols = append(facts.Symbols, name)
				facts.DefinitionLines[name] = int(node.StartPosition().Row) + 1
			}
		}
		for i := uint(0); i < node.NamedChildCount(); i++ {
			walk(node.NamedChild(i), depth+1)
			if len(facts.Identifiers) >= codeGraphMaxIdentifiers && len(facts.Symbols) >= codeGraphMaxSymbolsPerFile {
				break
			}
		}
	}
	walk(root, 0)

	// Tree-sitter intentionally returns useful partial trees for malformed code.
	// Accept those facts as long as the parser found at least one structural fact;
	// syntax/build verification remains authoritative later in the agent pipeline.
	if len(facts.Symbols) == 0 && len(facts.Identifiers) == 0 && len(facts.Imports) == 0 {
		return codeGraphSemanticFacts{}, false
	}
	return facts, true
}

func codeGraphTreeSitterIdentifierKind(kind string) bool {
	switch kind {
	case "identifier", "type_identifier", "field_identifier", "property_identifier", "namespace_identifier", "shorthand_property_identifier_pattern":
		return true
	default:
		return strings.HasSuffix(kind, "_identifier") && !strings.Contains(kind, "builtin")
	}
}

func codeGraphTreeSitterSymbolNode(node *treesitter.Node) bool {
	if node == nil {
		return false
	}
	kind := node.Kind()
	switch kind {
	case "function_declaration", "generator_function_declaration", "function_definition",
		"class_declaration", "abstract_class_declaration", "class_definition", "class_specifier",
		"method_definition", "method_declaration",
		"interface_declaration", "type_alias_declaration", "enum_declaration",
		"function_item", "struct_item", "enum_item", "trait_item", "type_item", "const_item", "static_item", "mod_item",
		"struct_specifier", "union_specifier", "enum_specifier", "type_definition", "alias_declaration", "concept_definition", "namespace_definition":
		return true
	case "variable_declarator":
		return codeGraphTreeSitterTopLevel(node)
	case "declaration":
		return codeGraphTreeSitterTopLevel(node)
	default:
		return false
	}
}

func codeGraphTreeSitterTopLevel(node *treesitter.Node) bool {
	parent := node.Parent()
	for hops := 0; parent != nil && hops < 5; hops++ {
		switch parent.Kind() {
		case "program", "module", "translation_unit", "source_file":
			return true
		case "export_statement", "lexical_declaration", "variable_declaration", "declaration_list":
			parent = parent.Parent()
			continue
		case "statement_block", "block", "compound_statement", "function_definition", "function_declaration", "method_definition", "class_body":
			return false
		default:
			parent = parent.Parent()
		}
	}
	return false
}

func codeGraphTreeSitterSymbolName(node *treesitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	if name := node.ChildByFieldName("name"); name != nil {
		if value := codeGraphTreeSitterNodeIdentifier(name, source, 0); value != "" {
			return value
		}
	}
	for _, field := range []string{"declarator", "type", "left"} {
		if candidate := node.ChildByFieldName(field); candidate != nil {
			if value := codeGraphTreeSitterNodeIdentifier(candidate, source, 0); value != "" {
				return value
			}
		}
	}
	return codeGraphTreeSitterNodeIdentifier(node, source, 0)
}

func codeGraphTreeSitterNodeIdentifier(node *treesitter.Node, source []byte, depth int) string {
	if node == nil || depth > 12 {
		return ""
	}
	if codeGraphTreeSitterIdentifierKind(node.Kind()) {
		start, end := int(node.StartByte()), int(node.EndByte())
		if start >= 0 && end > start && end <= len(source) {
			value := strings.TrimSpace(string(source[start:end]))
			if codeGraphIdentifierPattern.MatchString(value) && codeGraphIdentifierPattern.FindString(value) == value {
				return value
			}
		}
	}
	for i := uint(0); i < node.NamedChildCount(); i++ {
		if value := codeGraphTreeSitterNodeIdentifier(node.NamedChild(i), source, depth+1); value != "" {
			return value
		}
	}
	return ""
}
