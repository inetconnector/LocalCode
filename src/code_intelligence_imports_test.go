// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestCodeGraphDisambiguatesDuplicateSymbolsUsingImports(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "checkout/controller.ts", `import { normalizeId } from '../payments/identity'
export function submitOrder(id: string) { return normalizeId(id) }
`)
	writeRepoIntelTestFile(t, root, "payments/identity.ts", `export function normalizeId(id: string) { return "pay-" + id }
`)
	writeRepoIntelTestFile(t, root, "analytics/identity.ts", `export function normalizeId(id: string) { return "ana-" + id }
`)

	files, err := buildCodeGraphFiles(root, "change submitOrder normalizeId")
	if err != nil {
		t.Fatal(err)
	}
	adjacency, _ := buildCodeGraphEdges(files)
	index := func(want string) int {
		for i := range files {
			if files[i].Path == want {
				return i
			}
		}
		t.Fatalf("missing graph file %s", want)
		return -1
	}
	controller := index("checkout/controller.ts")
	payments := index("payments/identity.ts")
	analytics := index("analytics/identity.ts")
	if !adjacency[controller][payments] {
		t.Fatalf("explicit import should connect controller to payments identity")
	}
	if adjacency[controller][analytics] {
		t.Fatalf("ambiguous normalizeId must not connect controller to unrelated analytics identity")
	}
}

func TestCodeGraphConnectsExplicitImportWithoutExtractedSymbol(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "feature/bootstrap.ts", `import '../shared/register'
export function boot() { return true }
`)
	// The lightweight symbol extractor intentionally does not recognize this
	// object literal declaration. The explicit import should still create a
	// dependency edge.
	writeRepoIntelTestFile(t, root, "shared/register.ts", `export const registry = new Map<string, string>()
`)

	files, err := buildCodeGraphFiles(root, "boot feature")
	if err != nil {
		t.Fatal(err)
	}
	adjacency, _ := buildCodeGraphEdges(files)
	var feature, shared = -1, -1
	for i := range files {
		switch files[i].Path {
		case "feature/bootstrap.ts":
			feature = i
		case "shared/register.ts":
			shared = i
		}
	}
	if feature < 0 || shared < 0 {
		t.Fatalf("missing expected graph files: %#v", files)
	}
	if !adjacency[feature][shared] {
		t.Fatalf("explicit import should create dependency edge even without a recognized target symbol")
	}
}

func TestCodeGraphExtractImportSpecsHandlesGoJSAndPython(t *testing.T) {
	content := `package demo
import (
    "example.com/app/service"
    alias "example.com/app/shared/id"
)
import "example.com/app/other"
// language-mixed fixture below exercises the tolerant parser
import { thing } from '../ui/widget'
const helper = require("./helper")
from ..domain.user import User
`
	imports := codeGraphExtractImportSpecs(content)
	joined := strings.Join(imports, "\n")
	for _, want := range []string{
		"example.com/app/service",
		"example.com/app/shared/id",
		"example.com/app/other",
		"../ui/widget",
		"./helper",
		"../domain/user",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("imports missing %q: %#v", want, imports)
		}
	}
}
