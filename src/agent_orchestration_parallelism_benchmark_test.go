// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	orchestrationBenchmarkTasks          = 4
	defaultOllamaBenchmarkRequests       = 4
	defaultOllamaBenchmarkContextLength  = 4096
	defaultOllamaBenchmarkTimeoutSeconds = 180
)

type ollamaConcurrencyBenchmarkResult struct {
	Model                  string  `json:"model"`
	BaseURL                string  `json:"base_url"`
	ContextLength          int     `json:"context_length"`
	Concurrency            int     `json:"concurrency"`
	Requests               int     `json:"requests"`
	WallMillis             int64   `json:"wall_millis"`
	MeanLatencyMillis      float64 `json:"mean_latency_millis"`
	P95LatencyMillis       int64   `json:"p95_latency_millis"`
	RequestsPerSecond      float64 `json:"requests_per_second"`
	ClientOverlapFactor    float64 `json:"client_overlap_factor"`
	SpeedupVsSequential    float64 `json:"speedup_vs_sequential"`
}

func buildOrchestrationBenchmarkGraph(taskCount int) (AgentTaskGraph, error) {
	proposals := make([]AgentTaskProposal, 0, taskCount)
	for index := 0; index < taskCount; index++ {
		proposals = append(proposals, AgentTaskProposal{
			ID:        fmt.Sprintf("bench-%02d", index+1),
			Role:      "explorer",
			Objective: "Measure governed read-only scheduler dispatch.",
		})
	}
	graph, err := buildAgentTaskGraph("mission-orchestration-benchmark", "", proposals)
	if err != nil {
		return AgentTaskGraph{}, err
	}
	for index := range graph.Tasks {
		role, err := normalizeAgentRole(string(graph.Tasks[index].Role))
		if err != nil {
			return AgentTaskGraph{}, err
		}
		graph.Tasks[index].Capabilities = capabilitiesForAgentRole(role)
		graph.Tasks[index].Budget = AgentBudget{
			ModelCalls:           4,
			ToolCalls:            8,
			EstimatedTokenBudget: 20000,
			TimeSeconds:          60,
		}
	}
	return graph, nil
}

func recordBenchmarkPeak(peak *atomic.Int64, candidate int64) {
	for {
		current := peak.Load()
		if candidate <= current || peak.CompareAndSwap(current, candidate) {
			return
		}
	}
}

// BenchmarkScheduledReadOnlyDispatcherParallelism measures the difference
// between logical readiness/resource capacity and executor concurrency. The
// fixed executor delay makes overlap visible if dispatch becomes concurrent in
// a future implementation; today the synchronous dispatch loop reports a peak
// executor concurrency of one even when more model slots are configured.
func BenchmarkScheduledReadOnlyDispatcherParallelism(b *testing.B) {
	for _, modelSlots := range []int{1, 2, 4} {
		modelSlots := modelSlots
		b.Run(fmt.Sprintf("model-slots-%d", modelSlots), func(b *testing.B) {
			var observedPeak int64
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				graph, err := buildOrchestrationBenchmarkGraph(orchestrationBenchmarkTasks)
				if err != nil {
					b.Fatal(err)
				}
				scheduler := NewAgentScheduler(context.Background(), AgentResourceLimits{ModelInference: modelSlots})
				var inFlight atomic.Int64
				var peak atomic.Int64
				execute := func(context.Context, string, Config, AgentTask) (AgentResult, error) {
					current := inFlight.Add(1)
					recordBenchmarkPeak(&peak, current)
					time.Sleep(250 * time.Microsecond)
					inFlight.Add(-1)
					return AgentResult{Status: AgentResultCompleted, Summary: "synthetic benchmark task completed"}, nil
				}
				run, err := (&AppState{}).runScheduledReadOnlyAgentGraphWithExecutor("benchmark", Config{}, &graph, scheduler, execute)
				scheduler.missionCancel()
				if err != nil {
					b.Fatal(err)
				}
				if len(run.Results) != orchestrationBenchmarkTasks {
					b.Fatalf("results=%d want=%d", len(run.Results), orchestrationBenchmarkTasks)
				}
				if candidate := peak.Load(); candidate > observedPeak {
					observedPeak = candidate
				}
			}
			b.ReportMetric(float64(orchestrationBenchmarkTasks), "logical_ready/op")
			b.ReportMetric(float64(modelSlots), "model_slots")
			b.ReportMetric(float64(observedPeak), "peak_executor_inflight")
			b.ReportMetric(float64(orchestrationBenchmarkTasks), "tasks/op")
			b.ReportMetric(float64(observedPeak)/float64(orchestrationBenchmarkTasks), "executor_to_ready_ratio")
		})
	}
}

func benchmarkEnvInt(name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s=%d outside allowed range %d..%d", name, value, minimum, maximum)
	}
	return value, nil
}

func loopbackBenchmarkURL(raw string) (string, error) {
	base := normalizeOllamaBaseURL(raw)
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	host := strings.TrimSpace(parsed.Hostname())
	if strings.EqualFold(host, "localhost") {
		return base, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("Ollama benchmark requires a loopback endpoint, got %q", base)
	}
	return base, nil
}

func ollamaBenchmarkModelInstalled(models []ModelInfo, model string) bool {
	for _, candidate := range models {
		if strings.TrimSpace(candidate.Name) == model {
			return true
		}
	}
	return false
}

func runOllamaConcurrencyScenario(parent context.Context, client *OllamaClient, model string, requestCount, concurrency int, requestTimeout time.Duration) (ollamaConcurrencyBenchmarkResult, error) {
	result := ollamaConcurrencyBenchmarkResult{
		Model:         model,
		BaseURL:       client.BaseURL,
		ContextLength: client.ContextLength,
		Concurrency:   concurrency,
		Requests:      requestCount,
	}
	latencies := make([]time.Duration, requestCount)
	errs := make([]error, requestCount)
	jobs := make(chan int, requestCount)
	for index := 0; index < requestCount; index++ {
		jobs <- index
	}
	close(jobs)

	workers := concurrency
	if workers > requestCount {
		workers = requestCount
	}
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(workers)
	done.Add(workers)
	startGate := make(chan struct{})
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
		},
		"required":             []string{"ok"},
		"additionalProperties": false,
	}
	messages := []OllamaMessage{{Role: "user", Content: "Return a JSON object with ok=true. No explanation."}}
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-startGate
			for index := range jobs {
				requestCtx, cancel := context.WithTimeout(parent, requestTimeout)
				started := time.Now()
				_, err := client.Chat(requestCtx, model, messages, schema)
				latencies[index] = time.Since(started)
				errs[index] = err
				cancel()
			}
		}()
	}
	ready.Wait()
	started := time.Now()
	close(startGate)
	done.Wait()
	wall := time.Since(started)
	for index, err := range errs {
		if err != nil {
			return result, fmt.Errorf("request %d/%d at concurrency %d failed: %w", index+1, requestCount, concurrency, err)
		}
	}

	sortedLatencies := append([]time.Duration(nil), latencies...)
	sort.Slice(sortedLatencies, func(i, j int) bool { return sortedLatencies[i] < sortedLatencies[j] })
	var sum time.Duration
	for _, latency := range latencies {
		sum += latency
	}
	p95Index := (95*len(sortedLatencies)+99)/100 - 1
	if p95Index < 0 {
		p95Index = 0
	}
	result.WallMillis = wall.Milliseconds()
	result.MeanLatencyMillis = float64(sum) / float64(requestCount) / float64(time.Millisecond)
	result.P95LatencyMillis = sortedLatencies[p95Index].Milliseconds()
	if wall > 0 {
		result.RequestsPerSecond = float64(requestCount) / wall.Seconds()
		result.ClientOverlapFactor = float64(sum) / float64(wall)
	}
	return result, nil
}

// TestOllamaConcurrencyBenchmarkOptIn is intentionally a fixed-work benchmark
// rather than testing.B calibration because local LLM requests are expensive.
// It never starts Ollama, downloads a model, mutates config, or contacts a
// non-loopback endpoint. Enable it explicitly with LOCALCODE_BENCH_OLLAMA=1
// and provide the exact installed model name in LOCALCODE_BENCH_MODEL.
func TestOllamaConcurrencyBenchmarkOptIn(t *testing.T) {
	if strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_OLLAMA")) != "1" {
		t.Skip("set LOCALCODE_BENCH_OLLAMA=1 to run the local Ollama concurrency benchmark")
	}
	model := strings.TrimSpace(os.Getenv("LOCALCODE_BENCH_MODEL"))
	if model == "" {
		t.Fatal("LOCALCODE_BENCH_MODEL must name an already-installed Ollama model")
	}
	requestCount, err := benchmarkEnvInt("LOCALCODE_BENCH_REQUESTS", defaultOllamaBenchmarkRequests, 4, 32)
	if err != nil {
		t.Fatal(err)
	}
	contextLength, err := benchmarkEnvInt("LOCALCODE_BENCH_CONTEXT", defaultOllamaBenchmarkContextLength, 4096, 131072)
	if err != nil {
		t.Fatal(err)
	}
	timeoutSeconds, err := benchmarkEnvInt("LOCALCODE_BENCH_TIMEOUT_SECONDS", defaultOllamaBenchmarkTimeoutSeconds, 10, 1800)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := loopbackBenchmarkURL(os.Getenv("OLLAMA_HOST"))
	if err != nil {
		t.Fatal(err)
	}
	client := NewOllamaClient()
	client.BaseURL = baseURL
	client.ContextLength = contextLength

	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	models, err := client.Tags(probeCtx)
	probeCancel()
	if err != nil {
		t.Fatalf("local Ollama is not reachable at %s: %v", baseURL, err)
	}
	if !ollamaBenchmarkModelInstalled(models, model) {
		t.Fatalf("model %q is not installed at %s; benchmark will not pull it", model, baseURL)
	}

	results := make([]ollamaConcurrencyBenchmarkResult, 0, 3)
	for _, concurrency := range []int{1, 2, 4} {
		result, err := runOllamaConcurrencyScenario(context.Background(), client, model, requestCount, concurrency, time.Duration(timeoutSeconds)*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, result)
	}
	baseline := results[0].WallMillis
	for index := range results {
		if results[index].WallMillis > 0 && baseline > 0 {
			results[index].SpeedupVsSequential = float64(baseline) / float64(results[index].WallMillis)
		}
		encoded, err := json.Marshal(results[index])
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("ORCHESTRATION_BENCH %s", encoded)
	}
}
