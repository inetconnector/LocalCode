// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestRepositoryReferenceGraphFindsDefinitionsReferencesAndCentrality(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "go.mod", "module example.com/graph\n\ngo 1.25\n")
	writeRepoIntelTestFile(t, root, "billing/service.go", `package billing

type InvoiceService struct{}
func (InvoiceService) CalculateInvoiceTotal() int { return SharedTaxRate() }
func SharedTaxRate() int { return 19 }
`)
	writeRepoIntelTestFile(t, root, "checkout/handler.go", `package checkout
import "example.com/graph/billing"
func Checkout() int {
    s := billing.InvoiceService{}
    return s.CalculateInvoiceTotal()
}
`)
	writeRepoIntelTestFile(t, root, "report/report.go", `package report
import "example.com/graph/billing"
func MonthlyReport() int { return billing.SharedTaxRate() }
`)
	writeRepoIntelTestFile(t, root, "billing/service_test.go", `package billing
func TestCalculateInvoiceTotal(t *testing.T) { _ = InvoiceService{}.CalculateInvoiceTotal() }
`)
	writeRepoIntelTestFile(t, root, "unrelated/cache.go", `package unrelated
func EvictCacheEntry() {}
`)

	report, err := repositoryReferenceGraph(root, "fix CalculateInvoiceTotal invoice billing")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"CODE INTELLIGENCE GRAPH",
		"TASK-RELEVANT GRAPH NEIGHBORHOOD",
		"HIGH-CENTRALITY API / ARCHITECTURE ANCHORS",
		"STATIC SYMBOL NAVIGATION FOR TASK",
		"definition CalculateInvoiceTotal",
		"billing/service.go",
		"checkout/handler.go",
		"reference",
		"PageRank-style centrality",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("graph report missing %q:\n%s", want, report)
		}
	}
	if strings.Index(report, "billing/service.go") > strings.Index(report, "unrelated/cache.go") && strings.Contains(report, "unrelated/cache.go") {
		t.Fatalf("task-relevant billing implementation should rank before unrelated cache:\n%s", report)
	}
}

func TestRepositoryReferenceGraphPropagatesRelevanceToSharedDependency(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "feature/controller.ts", `import { normalizeAccountId } from '../shared/identity'
export function updateCustomerAccount(id: string) { return normalizeAccountId(id) }
`)
	writeRepoIntelTestFile(t, root, "shared/identity.ts", `export function normalizeAccountId(id: string) { return id.trim().toLowerCase() }
`)
	writeRepoIntelTestFile(t, root, "other/random.ts", `export function randomUtility() { return 1 }
`)

	report, err := repositoryReferenceGraph(root, "change customer account update behavior")
	if err != nil {
		t.Fatal(err)
	}
	feature := strings.Index(report, "feature/controller.ts")
	shared := strings.Index(report, "shared/identity.ts")
	other := strings.Index(report, "other/random.ts")
	if feature < 0 || shared < 0 {
		t.Fatalf("expected feature and dependency in graph report:\n%s", report)
	}
	if other >= 0 && shared > other {
		t.Fatalf("shared dependency should outrank unrelated source after graph propagation:\n%s", report)
	}
}

func TestCodeGraphDefinitionAndReferenceLines(t *testing.T) {
	content := "package demo\nfunc ImportantOperation() {}\nfunc caller() { ImportantOperation() }\n"
	if got := codeGraphDefinitionLine(content, "ImportantOperation"); got != 2 {
		t.Fatalf("definition line = %d, want 2", got)
	}
	lines := codeGraphReferenceLines(content, "ImportantOperation", 5)
	if len(lines) != 2 || lines[0] != 2 || lines[1] != 3 {
		t.Fatalf("reference lines = %#v, want [2 3]", lines)
	}
}

func TestReadOnlySubagentIncludesReferenceGraph(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.25\n")
	writeRepoIntelTestFile(t, root, "service.go", "package demo\nfunc RepairQueue() {}\n")
	writeRepoIntelTestFile(t, root, "worker.go", "package demo\nfunc Worker() { RepairQueue() }\n")

	report, err := runReadOnlySubagent(root, Config{}, "repair RepairQueue in service.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"REFERENCE GRAPH / STATIC CODE NAVIGATION", "CODE INTELLIGENCE GRAPH", "RepairQueue", "Inspect callers/references"} {
		if !strings.Contains(report, want) {
			t.Fatalf("subagent report missing %q:\n%s", want, report)
		}
	}
}
