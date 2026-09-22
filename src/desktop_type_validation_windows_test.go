// SPDX-License-Identifier: Apache-2.0
//go:build windows

package main

import (
	"context"
	"testing"
)

func TestDesktopTypeRejectsMissingTargetBeforeExecution(t *testing.T) {
	// Cancellation makes accidental command execution return a different error,
	// and prevents this validation test from touching any real desktop window.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, lang := range []string{"de", "en"} {
		for _, target := range [][2]string{{"", "Edit"}, {" \t", "Edit"}, {"Notepad", ""}, {"Notepad", " \r\n\t"}} {
			t.Run(lang+"/"+target[0]+"/"+target[1], func(t *testing.T) {
				cfg := Config{Language: lang}
				out, err := DesktopType(ctx, cfg, target[0], target[1], "text")
				want := localizeConfigText(cfg, "desktop_type benötigt window_title und control_name", "desktop_type requires window_title and control_name")
				if out != "" || err == nil || err.Error() != want {
					t.Fatalf("expected validation before execution: out=%q err=%v want=%q", out, err, want)
				}
			})
		}
	}
}
