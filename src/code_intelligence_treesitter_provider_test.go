// SPDX-License-Identifier: Apache-2.0
//go:build cgo

package main

import "testing"

func TestTreeSitterProviderSelectionCoversPrimaryPolyglotLanguages(t *testing.T) {
	cases := map[string]string{
		"app.js": "javascript",
		"app.jsx": "javascript",
		"app.ts": "typescript",
		"app.tsx": "tsx",
		"app.py": "python",
		"app.rs": "rust",
		"app.c": "c",
		"app.cpp": "cpp",
		"app.hpp": "cpp",
	}
	for path, want := range cases {
		spec, ok := codeGraphTreeSitterSpecForPath(path)
		if !ok || spec.Language == nil || spec.Label != want {
			t.Fatalf("provider for %s = %#v ok=%t, want %s", path, spec, ok, want)
		}
	}
	if _, ok := codeGraphTreeSitterSpecForPath("README.md"); ok {
		t.Fatal("unexpected Tree-sitter provider for markdown")
	}
}
