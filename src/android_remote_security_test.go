// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAndroidRemoteLocksNavigationToPinnedPrivateOrigin(t *testing.T) {
	path := filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "remote", "MainActivity.java")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"setWebChromeClient(new WebChromeClient())",
		"sameRemoteOrigin(request.getUrl())",
		"address.isLoopbackAddress()",
		"address.isLinkLocalAddress()",
		"address.isSiteLocalAddress()",
		"expectedFingerprint.equalsIgnoreCase(observed)",
		"MIXED_CONTENT_NEVER_ALLOW",
		"setAllowFileAccess(false)",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Android Remote security invariant missing: %s", required)
		}
	}
	if strings.Contains(text, `if (host.contains(":")) return true`) {
		t.Fatal("Android Remote must not trust every IPv6 address")
	}
	if strings.Contains(text, `if ("https".equalsIgnoreCase(uri.getScheme()))`) {
		t.Fatal("Android Remote must not allow arbitrary HTTPS navigation")
	}
}
