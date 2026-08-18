// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunSubagentReadJobsIsBoundedAndDeterministic(t *testing.T) {
	var active int32
	var maxActive int32
	jobs := make([]subagentReadJob, 0, 8)
	for i := 0; i < 8; i++ {
		i := i
		jobs = append(jobs, subagentReadJob{
			name: fmt.Sprintf("job-%d", i),
			run: func() (string, error) {
				current := atomic.AddInt32(&active, 1)
				for {
					seen := atomic.LoadInt32(&maxActive)
					if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
						break
					}
				}
				defer atomic.AddInt32(&active, -1)
				// Reverse delays ensure completion order differs from input order.
				time.Sleep(time.Duration(8-i) * 3 * time.Millisecond)
				return fmt.Sprintf("result-%d", i), nil
			},
		})
	}

	results := runSubagentReadJobs(jobs, 3)
	if len(results) != len(jobs) {
		t.Fatalf("got %d results, want %d", len(results), len(jobs))
	}
	if got := atomic.LoadInt32(&maxActive); got > 3 {
		t.Fatalf("parallel read limit exceeded: got %d, want <= 3", got)
	} else if got < 2 {
		t.Fatalf("jobs did not execute concurrently: max active %d", got)
	}
	for i, result := range results {
		wantName := fmt.Sprintf("job-%d", i)
		wantOutput := fmt.Sprintf("result-%d", i)
		if result.name != wantName || result.output != wantOutput || result.err != nil {
			t.Fatalf("result %d = %#v, want name=%q output=%q nil error", i, result, wantName, wantOutput)
		}
	}
}

func TestRunSubagentReadJobsZeroLimitFallsBackToSerial(t *testing.T) {
	var active int32
	var maxActive int32
	jobs := []subagentReadJob{
		{name: "a", run: func() (string, error) {
			current := atomic.AddInt32(&active, 1)
			if current > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, current)
			}
			defer atomic.AddInt32(&active, -1)
			time.Sleep(2 * time.Millisecond)
			return "A", nil
		}},
		{name: "b", run: func() (string, error) {
			current := atomic.AddInt32(&active, 1)
			if current > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, current)
			}
			defer atomic.AddInt32(&active, -1)
			time.Sleep(2 * time.Millisecond)
			return "B", nil
		}},
	}

	results := runSubagentReadJobs(jobs, 0)
	if got := atomic.LoadInt32(&maxActive); got != 1 {
		t.Fatalf("zero limit must fall back to serial execution; max active = %d", got)
	}
	if results[0].output != "A" || results[1].output != "B" {
		t.Fatalf("result order changed: %#v", results)
	}
}

func TestRunSubagentReadJobsPreservesErrorsInInputOrder(t *testing.T) {
	jobs := []subagentReadJob{
		{name: "slow", run: func() (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "first", nil
		}},
		{name: "fast-error", run: func() (string, error) {
			return "partial", fmt.Errorf("boom")
		}},
	}
	results := runSubagentReadJobs(jobs, 2)
	if results[0].name != "slow" || results[0].output != "first" || results[0].err != nil {
		t.Fatalf("unexpected first result: %#v", results[0])
	}
	if results[1].name != "fast-error" || results[1].output != "partial" || results[1].err == nil {
		t.Fatalf("unexpected second result: %#v", results[1])
	}
}
