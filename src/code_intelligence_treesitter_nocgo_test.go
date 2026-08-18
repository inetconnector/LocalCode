// SPDX-License-Identifier: Apache-2.0
//go:build !cgo

package main

import "testing"

func TestTreeSitterFallbackIsDisabledWithoutCGO(t *testing.T) {
	facts, ok := codeGraphTreeSitterFacts("worker.ts", "export function runTask() {}")
	if ok {
		t.Fatalf("tree-sitter must not claim availability without CGO: %#v", facts)
	}
}
