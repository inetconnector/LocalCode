// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseClawVersionReportUsesGitSHA(t *testing.T) {
	got, err := parseClawVersionReport("STDOUT:\n{\"git_sha\":\"08106b0c3771ef5b4a5aa176acccd460e88b7325\",\"git_sha_short\":\"08106b0\",\"human_readable\":\"Claw Code\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	if got != clawPinnedCommit {
		t.Fatalf("parsed Claw git SHA = %q; want %q", got, clawPinnedCommit)
	}
}

func TestParseClawVersionReportIgnoresCapturedStderr(t *testing.T) {
	raw := "STDOUT:\r\n{\"git_sha\":\"" + clawPinnedCommit + "\"}\r\nSTDERR:\r\nwarning: diagnostic only\r\n"
	got, err := parseClawVersionReport(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != clawPinnedCommit {
		t.Fatalf("parsed Claw git SHA = %q; want %q", got, clawPinnedCommit)
	}
}

func TestParseClawVersionReportFailsClosedWithoutGitSHA(t *testing.T) {
	for _, raw := range []string{
		`{"git_sha_short":"08106b0"}`,
		`{"git_sha":"   "}`,
		`not-json`,
	} {
		if _, err := parseClawVersionReport(raw); err == nil {
			t.Fatalf("invalid Claw version report was accepted: %q", raw)
		}
	}
}

func TestClawVersionVerificationUsesDocumentedJSONCommand(t *testing.T) {
	data, err := os.ReadFile("claw_engine.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{`[]string{"version", "--output-format", "json"}`, "parseClawVersionReport(output)"} {
		if !strings.Contains(text, required) {
			t.Fatalf("Claw version verification missing %q", required)
		}
	}
	if strings.Contains(text, `[]string{"--version"}`) {
		t.Fatal("Claw-specific version verification regressed to generic --version")
	}
}
