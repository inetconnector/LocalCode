// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestCodeGraphRelationsDistinguishImportsCallsAndReferences(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "feature/controller.ts", `import { normalizeAccountId } from '../shared/identity'
export function updateAccount(id: string) { return normalizeAccountId(id) }
`)
	writeRepoIntelTestFile(t, root, "shared/identity.ts", `export function normalizeAccountId(id: string) { return id.trim().toLowerCase() }
`)

	files, err := buildCodeGraphFiles(root, "update account")
	if err != nil {
		t.Fatal(err)
	}
	relations := buildCodeGraphRelations(files)
	source := relationTestFileIndex(t, files, "feature/controller.ts")
	target := relationTestFileIndex(t, files, "shared/identity.ts")
	relation := relations[source][target]
	if relation == nil {
		t.Fatal("expected typed relation")
	}
	for _, kind := range []codeGraphRelationKind{codeGraphRelationImport, codeGraphRelationReference, codeGraphRelationCall} {
		if !relation.Kinds[kind] {
			t.Fatalf("relation missing %s: %#v", kind, relation.Kinds)
		}
	}
	if !relation.Symbols["normalizeAccountId"] {
		t.Fatalf("relation missing symbol evidence: %#v", relation.Symbols)
	}
	if relation.weight() <= 6 {
		t.Fatalf("combined structural relation should be strongly weighted, got %.2f", relation.weight())
	}
}

func TestCodeGraphRelationsDetectInheritanceAndImplementation(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "contracts/base.ts", `export class BaseWorker {}
export interface WorkerContract { run(): void }
`)
	writeRepoIntelTestFile(t, root, "worker/worker.ts", `import { BaseWorker, WorkerContract } from '../contracts/base'
export class Worker extends BaseWorker implements WorkerContract {
  run() {}
}
`)

	files, err := buildCodeGraphFiles(root, "worker contract")
	if err != nil {
		t.Fatal(err)
	}
	relations := buildCodeGraphRelations(files)
	source := relationTestFileIndex(t, files, "worker/worker.ts")
	target := relationTestFileIndex(t, files, "contracts/base.ts")
	relation := relations[source][target]
	if relation == nil {
		t.Fatal("expected worker-to-contract relation")
	}
	if !relation.Kinds[codeGraphRelationInherit] {
		t.Fatalf("missing inherits relation: %#v", relation.Kinds)
	}
	if !relation.Kinds[codeGraphRelationImplement] {
		t.Fatalf("missing implements relation: %#v", relation.Kinds)
	}
}

func TestCodeGraphRelationsAttachTestOfByConventionalFilename(t *testing.T) {
	files := []codeGraphFile{
		{Path: "service.go", Symbols: []string{"Service"}, Identifiers: map[string]bool{}},
		{Path: "service_test.go", Symbols: []string{"TestService"}, Identifiers: map[string]bool{}},
	}
	relations := buildCodeGraphRelations(files)
	relation := relations[1][0]
	if relation == nil || !relation.Kinds[codeGraphRelationTestOf] {
		t.Fatalf("expected conventional test-of relation, got %#v", relation)
	}
}

func TestTypedRelationStrengthAffectsTaskPropagation(t *testing.T) {
	files := []codeGraphFile{
		{
			Path:        "feature.go",
			Content:     "func Feature() { StrongDependency(); _ = WeakDependency }",
			Symbols:     []string{"Feature"},
			Identifiers: map[string]bool{"StrongDependency": true, "WeakDependency": true},
			BaseScore:   30,
		},
		{
			Path:        "strong.go",
			Content:     "func StrongDependency() {}",
			Symbols:     []string{"StrongDependency"},
			Identifiers: map[string]bool{"StrongDependency": true},
		},
		{
			Path:        "weak.go",
			Content:     "var WeakDependency = 1",
			Symbols:     []string{"WeakDependency"},
			Identifiers: map[string]bool{"WeakDependency": true},
		},
	}
	relations := buildCodeGraphRelations(files)
	adjacency, reverse := codeGraphRelationAdjacency(files, relations)
	applyCodeGraphRelationRanks(files, relations, adjacency, reverse)
	if !relations[0][1].Kinds[codeGraphRelationCall] {
		t.Fatalf("expected strong dependency to be a call: %#v", relations[0][1])
	}
	if relations[0][2].Kinds[codeGraphRelationCall] {
		t.Fatalf("weak dependency must remain a generic reference: %#v", relations[0][2])
	}
	if files[1].TaskScore <= files[2].TaskScore {
		t.Fatalf("call target should receive stronger propagated relevance: strong=%.2f weak=%.2f", files[1].TaskScore, files[2].TaskScore)
	}
}

func TestRepositoryReferenceGraphReportsTypedRelationEvidence(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "feature/controller.ts", `import { executeOrder } from '../domain/order'
export function submitOrder() { return executeOrder() }
`)
	writeRepoIntelTestFile(t, root, "domain/order.ts", `export function executeOrder() { return true }
`)

	report, err := repositoryReferenceGraph(root, "change submitOrder executeOrder")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"weighted typed relations",
		"imports, references, calls, inherits, implements, test-of",
		"calls+imports+references",
		"relation-weight in/out",
		"parser:",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("typed graph report missing %q:\n%s", want, report)
		}
	}
}

func relationTestFileIndex(t *testing.T, files []codeGraphFile, path string) int {
	t.Helper()
	for i := range files {
		if files[i].Path == path {
			return i
		}
	}
	t.Fatalf("missing graph file %s", path)
	return -1
}
