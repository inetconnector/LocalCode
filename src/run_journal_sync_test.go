// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

func TestRunJournalGenericTokenRedactionContract(t *testing.T) {
	input := "request token=super-secret-value api_key=another-secret password=hunter2"
	redacted := sanitizeRunJournalText(input, 4096)
	for _, secret := range []string{"super-secret-value", "another-secret", "hunter2"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("run journal leaked secret %q in %q", secret, redacted)
		}
	}
	if count := strings.Count(redacted, "[REDACTED]"); count != 3 {
		t.Fatalf("expected all secret classes to be redacted, got %d in %q", count, redacted)
	}
}
