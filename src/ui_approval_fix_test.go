// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io/fs"
	"strings"
	"testing"
)

func TestApprovalLayoutFixIsLoadedAndKeepsSingleApprovalSurface(t *testing.T) {
	loader, err := fs.ReadFile(staticFS, "static/i18n.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(loader), `/ui_approval_fix.js`) {
		t.Fatal("approval layout fix is not loaded by the desktop UI")
	}

	data, err := fs.ReadFile(staticFS, "static/ui_approval_fix.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, fragment := range []string{
		`#rightBody .output-card.approval{display:none!important}`,
		`const pending = state.pending;`,
		`state.pending = null;`,
		`state.pending = pending;`,
		`top:50%!important`,
		`transform:translate(-50%,-50%)!important`,
		`max-width:300px`,
		`overflow-x:hidden!important`,
		`white-space:pre-wrap!important`,
	} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("approval layout safety contract missing %q", fragment)
		}
	}
}
