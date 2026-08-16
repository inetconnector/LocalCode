// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestLocalCodeUserAgentUsesBuildVersion(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })
	version = "9.8.7-test"

	got := localCodeUserAgent()
	if !strings.Contains(got, "LocalCode/9.8.7-test") {
		t.Fatalf("user agent does not include build version: %q", got)
	}
	if strings.Contains(got, "LocalCode/6.4.3") {
		t.Fatalf("user agent contains stale hard-coded version: %q", got)
	}
}
