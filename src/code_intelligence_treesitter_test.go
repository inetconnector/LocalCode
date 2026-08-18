// SPDX-License-Identifier: Apache-2.0
//go:build cgo

package main

import (
	"strings"
	"testing"
)

func TestTreeSitterJavaScriptExtractsTopLevelSymbolsWithoutLocalVariables(t *testing.T) {
	content := `export const parseConfig = async (raw) => raw.trim();
export class Runner {
  start() { return parseConfig(" ok "); }
}
function helper() {
  const localOnly = () => 1;
  return localOnly();
}
`
	facts := codeGraphExtractSemanticFacts("worker.js", "JavaScript", content)
	if facts.Source != "tree-sitter/javascript" {
		t.Fatalf("semantic source = %q", facts.Source)
	}
	for _, symbol := range []string{"parseConfig", "Runner", "start", "helper"} {
		if !semanticTestContainsString(facts.Symbols, symbol) {
			t.Fatalf("missing symbol %q in %#v", symbol, facts.Symbols)
		}
	}
	if semanticTestContainsString(facts.Symbols, "localOnly") {
		t.Fatalf("local variable leaked into repository symbol map: %#v", facts.Symbols)
	}
	if !facts.Identifiers["parseConfig"] || !facts.Identifiers["localOnly"] {
		t.Fatalf("structured identifiers missing: %#v", facts.Identifiers)
	}
}

func TestTreeSitterTypeScriptExtractsInterfaceAliasAndArrowFunction(t *testing.T) {
	content := `export interface Store { load(id: string): Promise<string> }
export type StoreID = string;
export const createStore = (): Store => ({ load: async id => id });
export class MemoryStore implements Store {
  async load(id: string) { return id; }
}
`
	facts := codeGraphExtractSemanticFacts("store.ts", "TypeScript", content)
	if facts.Source != "tree-sitter/typescript" {
		t.Fatalf("semantic source = %q", facts.Source)
	}
	for _, symbol := range []string{"Store", "StoreID", "createStore", "MemoryStore", "load"} {
		if !semanticTestContainsString(facts.Symbols, symbol) {
			t.Fatalf("missing symbol %q in %#v", symbol, facts.Symbols)
		}
	}
}

func TestTreeSitterPythonExtractsClassesFunctionsAndPartialTrees(t *testing.T) {
	content := `import os

class Worker:
    def run(self, value):
        return value

def helper(value):
    return Worker().run(value)

# malformed tail should not discard the valid structural facts
if (
`
	facts := codeGraphExtractSemanticFacts("worker.py", "Python", content)
	if facts.Source != "tree-sitter/python" {
		t.Fatalf("semantic source = %q", facts.Source)
	}
	for _, symbol := range []string{"Worker", "run", "helper"} {
		if !semanticTestContainsString(facts.Symbols, symbol) {
			t.Fatalf("missing symbol %q in %#v", symbol, facts.Symbols)
		}
	}
	if !semanticTestContainsString(facts.Imports, "os") {
		t.Fatalf("imports = %#v, want os", facts.Imports)
	}
}

func TestTreeSitterRustAndCppProduceStructuredFacts(t *testing.T) {
	rust := `pub trait Engine { fn run(&self); }
pub struct LocalEngine;
impl Engine for LocalEngine { fn run(&self) {} }
pub fn build_engine() -> LocalEngine { LocalEngine }
`
	rustFacts := codeGraphExtractSemanticFacts("engine.rs", "Rust", rust)
	if rustFacts.Source != "tree-sitter/rust" {
		t.Fatalf("rust semantic source = %q", rustFacts.Source)
	}
	for _, symbol := range []string{"Engine", "LocalEngine", "run", "build_engine"} {
		if !semanticTestContainsString(rustFacts.Symbols, symbol) {
			t.Fatalf("rust missing symbol %q in %#v", symbol, rustFacts.Symbols)
		}
	}

	cpp := `class Engine { public: void run(); };
int build_engine(int value) { return value + 1; }
`
	cppFacts := codeGraphExtractSemanticFacts("engine.cpp", "C++", cpp)
	if cppFacts.Source != "tree-sitter/cpp" {
		t.Fatalf("cpp semantic source = %q", cppFacts.Source)
	}
	for _, symbol := range []string{"Engine", "build_engine"} {
		if !semanticTestContainsString(cppFacts.Symbols, symbol) {
			t.Fatalf("cpp missing symbol %q in %#v", symbol, cppFacts.Symbols)
		}
	}
}

func TestTreeSitterSemanticSourceIsReportedInReferenceGraph(t *testing.T) {
	root := t.TempDir()
	semanticTestWriteFile(t, root+"/worker.ts", `export function startWorker() { return helper(); }
function helper() { return 1; }
`)
	report, err := repositoryReferenceGraph(root, "repair startWorker")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"tree-sitter/typescript", "startWorker", "CODE INTELLIGENCE GRAPH"} {
		if !strings.Contains(report, marker) {
			t.Fatalf("report missing %q:\n%s", marker, report)
		}
	}
}
