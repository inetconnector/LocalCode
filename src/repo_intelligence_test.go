// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRepoIntelTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryIntelligenceRanksTaskFilesAndTests(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.25\n")
	writeRepoIntelTestFile(t, root, "main.go", "package main\nfunc main() { StartPaymentServer() }\n")
	writeRepoIntelTestFile(t, root, "payment/service.go", "package payment\n\nfunc CalculateInvoiceTotal() int { return 42 }\nfunc StartPaymentServer() {}\n")
	writeRepoIntelTestFile(t, root, "payment/service_test.go", "package payment\n\nfunc TestCalculateInvoiceTotal(t *testing.T) {}\n")
	writeRepoIntelTestFile(t, root, "unrelated/cache.go", "package unrelated\nfunc EvictCache() {}\n")
	writeRepoIntelTestFile(t, root, "node_modules/poison.go", "package poison\nfunc CalculateInvoiceTotalSecret() {}\n")

	report, err := repositoryIntelligence(root, "fix CalculateInvoiceTotal invoice payment")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"REPOSITORY INTELLIGENCE MAP",
		"payment/service.go",
		"CalculateInvoiceTotal",
		"payment/service_test.go",
		"go test ./...",
		"go vet ./...",
		"successful process exit",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(report, "CalculateInvoiceTotalSecret") || strings.Contains(report, "node_modules/poison.go") {
		t.Fatalf("ignored dependency directory leaked into intelligence report:\n%s", report)
	}
	if strings.Index(report, "payment/service.go") > strings.Index(report, "unrelated/cache.go") && strings.Contains(report, "unrelated/cache.go") {
		t.Fatalf("task-relevant implementation should rank before unrelated source:\n%s", report)
	}
}

func TestRepositoryIntelligenceDetectsDotnetVerification(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "Demo.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\"></Project>\n")
	writeRepoIntelTestFile(t, root, "Program.cs", "public static class Program { public static void Main() {} }\n")
	writeRepoIntelTestFile(t, root, "InvoiceService.cs", "public sealed class InvoiceService { public decimal Total() => 1m; }\n")
	writeRepoIntelTestFile(t, root, "InvoiceServiceTests.cs", "public sealed class InvoiceServiceTests { public void TotalWorks() {} }\n")

	report, err := repositoryIntelligence(root, "repair InvoiceService Total")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"InvoiceService.cs", "InvoiceServiceTests.cs", "dotnet build", "dotnet test --no-build"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestRepositoryIntelligenceUsesPackageScripts(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "package.json", `{"scripts":{"lint":"eslint .","test":"vitest run","build":"vite build"}}`)
	writeRepoIntelTestFile(t, root, "src/app.ts", "export function renderDashboard() { return 'ok' }\n")

	report, err := repositoryIntelligence(root, "change dashboard rendering")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"npm run lint", "npm run test", "npm run build", "renderDashboard"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestReadOnlySubagentIncludesRepositoryIntelligence(t *testing.T) {
	root := t.TempDir()
	writeRepoIntelTestFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.25\n")
	writeRepoIntelTestFile(t, root, "service.go", "package demo\nfunc RepairQueue() {}\n")

	report, err := runReadOnlySubagent(root, Config{}, "repair RepairQueue in service.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"READ-ONLY SUBAGENT HANDOFF", "REPOSITORY INTELLIGENCE", "RepairQueue", "zero exit code alone is not proof"} {
		if !strings.Contains(report, want) {
			t.Fatalf("subagent handoff missing %q:\n%s", want, report)
		}
	}
}

func TestForcedEditReliabilityPipelineBalanced(t *testing.T) {
	cfg := Config{EditingEngine: editingEngineAider, AiderEnabled: true, ResponseSpeed: "balanced"}
	intent := taskIntent{Kind: "edit", OriginalTask: "fix invoice calculation"}
	completed := map[string]bool{}

	if action := forcedEditReliabilityAction(intent, completed, cfg); action == nil || action.Action != "project_info" {
		t.Fatalf("first balanced stage = %#v, want project_info", action)
	}
	completed["project_info"] = true
	if action := forcedEditReliabilityAction(intent, completed, cfg); action == nil || action.Action != "subagent_analyze" {
		t.Fatalf("second balanced stage = %#v, want subagent_analyze", action)
	}
	completed["subagent_analyze"] = true
	action := forcedEditReliabilityAction(intent, completed, cfg)
	if action == nil || action.Action != "engine_edit" {
		t.Fatalf("third balanced stage = %#v, want engine_edit", action)
	}
	if !strings.Contains(action.Task, "LOCALCODE RELIABILITY CONTRACT") {
		t.Fatalf("engine edit task lacks reliability contract: %q", action.Task)
	}
	completed["engine_edit"] = true
	if action := forcedEditReliabilityAction(intent, completed, cfg); action == nil || action.Action != "engine_test" {
		t.Fatalf("fourth balanced stage = %#v, want engine_test", action)
	}
	completed["engine_test"] = true
	if action := forcedEditReliabilityAction(intent, completed, cfg); action != nil {
		t.Fatalf("balanced pipeline should be complete, got %#v", action)
	}
}

func TestForcedEditReliabilityPipelineThorough(t *testing.T) {
	cfg := Config{EditingEngine: editingEngineOpenCode, OpenCodeEnabled: true, ResponseSpeed: "thorough"}
	intent := taskIntent{Kind: "edit", OriginalTask: "refactor parser"}
	completed := map[string]bool{"project_info": true, "subagent_analyze": true, "engine_edit": true}

	if action := forcedEditReliabilityAction(intent, completed, cfg); action == nil || action.Action != "engine_lint" {
		t.Fatalf("thorough post-edit stage = %#v, want engine_lint", action)
	}
	completed["engine_lint"] = true
	if action := forcedEditReliabilityAction(intent, completed, cfg); action == nil || action.Action != "engine_test" {
		t.Fatalf("thorough verification stage = %#v, want engine_test", action)
	}
}

func TestForcedEditReliabilityPipelineFastAndNative(t *testing.T) {
	intent := taskIntent{Kind: "edit", OriginalTask: "change title"}

	fast := Config{EditingEngine: editingEngineAider, AiderEnabled: true, ResponseSpeed: "fast"}
	completed := map[string]bool{"project_info": true}
	if action := forcedEditReliabilityAction(intent, completed, fast); action == nil || action.Action != "engine_edit" {
		t.Fatalf("fast stage = %#v, want engine_edit without subagent", action)
	}
	completed["engine_edit"] = true
	if action := forcedEditReliabilityAction(intent, completed, fast); action != nil {
		t.Fatalf("fast pipeline should finish after edit, got %#v", action)
	}

	native := Config{EditingEngine: editingEngineNative, ResponseSpeed: "balanced"}
	completed = map[string]bool{"project_info": true}
	if action := forcedEditReliabilityAction(intent, completed, native); action == nil || action.Action != "subagent_analyze" {
		t.Fatalf("native balanced preflight = %#v, want subagent_analyze", action)
	}
	completed["subagent_analyze"] = true
	if action := forcedEditReliabilityAction(intent, completed, native); action != nil {
		t.Fatalf("native edits should return control to main tool loop after preflight, got %#v", action)
	}
}

func TestForcedActionForIntentUsesReliabilityPipeline(t *testing.T) {
	cfg := Config{EditingEngine: editingEngineAider, AiderEnabled: true, ResponseSpeed: "balanced"}
	intent := classifyTaskIntent("implementiere eine robuste Warteschlange")
	if intent.Kind != "edit" {
		t.Fatalf("intent = %q, want edit", intent.Kind)
	}
	action := forcedActionForIntent(intent, map[string]bool{}, cfg)
	if action == nil || action.Action != "project_info" {
		t.Fatalf("forced action = %#v, want project_info preflight", action)
	}
}
