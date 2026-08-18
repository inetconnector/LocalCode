// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeGraphGoASTFactsExtractsDefinitionsReferencesAndImports(t *testing.T) {
	content := `package demo

import "context"

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) Start(ctx context.Context) error {
	return helper(ctx)
}

func helper(ctx context.Context) error { return nil }
`
	facts := codeGraphExtractSemanticFacts("service.go", "Go", content)
	if facts.Source != "go/ast" {
		t.Fatalf("semantic source = %q, want go/ast", facts.Source)
	}
	for _, symbol := range []string{"Service", "NewService", "Start", "helper"} {
		if !containsString(facts.Symbols, symbol) {
			t.Fatalf("missing symbol %q in %#v", symbol, facts.Symbols)
		}
		if facts.DefinitionLines[symbol] <= 0 {
			t.Fatalf("missing definition line for %q: %#v", symbol, facts.DefinitionLines)
		}
	}
	if !facts.Identifiers["helper"] || !facts.Identifiers["Service"] {
		t.Fatalf("AST identifiers missing expected references: %#v", facts.Identifiers)
	}
	if !containsString(facts.Imports, "context") {
		t.Fatalf("imports = %#v, want context", facts.Imports)
	}
}

func TestCodeGraphSemanticFallbackForTypeScript(t *testing.T) {
	content := "export function runTask() { return helper(); }\nconst helper = () => 1;\n"
	facts := codeGraphExtractSemanticFacts("worker.ts", "TypeScript", content)
	if facts.Source != "lexical+imports" {
		t.Fatalf("semantic source = %q, want lexical+imports", facts.Source)
	}
	if !containsString(facts.Symbols, "runTask") || !containsString(facts.Symbols, "helper") {
		t.Fatalf("fallback symbols = %#v", facts.Symbols)
	}
}

func TestFormatCodeGraphContextPrioritizesTaskMatchedDefinition(t *testing.T) {
	relevant := strings.Repeat("// setup\n", 12) + `func StartWorker() error {
	return nil
}
` + strings.Repeat("// tail\n", 8)
	irrelevant := `package demo
func Unrelated() {}
`
	files := []codeGraphFile{
		{
			Path:            "worker.go",
			Language:        "Go",
			Content:         relevant,
			DefinitionLines: map[string]int{"StartWorker": 13},
			SemanticSource:  "go/ast",
			TaskScore:       30,
			Rank:            0.6,
		},
		{
			Path:            "other.go",
			Language:        "Go",
			Content:         irrelevant,
			DefinitionLines: map[string]int{"Unrelated": 2},
			SemanticSource:  "go/ast",
			TaskScore:       4,
			Rank:            1,
		},
	}
	context := formatCodeGraphContext(files, "fix StartWorker timeout", 900)
	if !strings.Contains(context, "worker.go") || !strings.Contains(context, "StartWorker") {
		t.Fatalf("task-matched context missing:\n%s", context)
	}
	if !strings.Contains(context, "task-matched definition") || !strings.Contains(context, "CONTEXT BUDGET") {
		t.Fatalf("context metadata missing:\n%s", context)
	}
	if strings.Index(context, "worker.go") > strings.Index(context, "other.go") && strings.Contains(context, "other.go") {
		t.Fatalf("task-matched file was not ranked first:\n%s", context)
	}
}

func TestRepositoryReferenceGraphIncludesGoASTContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "worker.go"), `package demo

func StartWorker() error {
	return helper()
}

func helper() error { return nil }
`)
	writeTestFile(t, filepath.Join(root, "worker_test.go"), `package demo

func TestStartWorker() {
	_ = StartWorker()
}
`)

	report, err := repositoryReferenceGraph(root, "repair StartWorker")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"TASK-RANKED SOURCE CONTEXT", "StartWorker", "go/ast", "CODE INTELLIGENCE GRAPH"} {
		if !strings.Contains(report, marker) {
			t.Fatalf("report missing %q:\n%s", marker, report)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
