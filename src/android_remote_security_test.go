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
		"setWebChromeClient(new WebChromeClient()",
		"onShowFileChooser",
		"FileChooserParams.parseResult",
		"REQUEST_FILE_CHOOSER",
		"addJavascriptInterface(new AndroidBridge(), \"LocalCodeAndroid\")",
		"@JavascriptInterface",
		"RecognizerIntent.ACTION_RECOGNIZE_SPEECH",
		"REQUEST_SPEECH",
		"JSONObject.quote",
		"sameRemoteOrigin(request.getUrl())",
		"address.isLoopbackAddress()",
		"address.isLinkLocalAddress()",
		"address.isSiteLocalAddress()",
		"expectedFingerprint.equalsIgnoreCase(observed)",
		"validFingerprint(expectedFingerprint)",
		"value.matches(\"[0-9A-F]{64}\")",
		"isLiteralIPv4(host)",
		"manualFingerprint",
		"expectedFingerprint = fp;",
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

	manualStart := strings.Index(text, "open.setOnClickListener")
	manualEnd := strings.Index(text, "discoveryPanel.addView(open")
	if manualStart < 0 || manualEnd <= manualStart {
		t.Fatal("manual Android Remote handler could not be isolated")
	}
	manualHandler := text[manualStart:manualEnd]
	if strings.Contains(manualHandler, `expectedFingerprint = ""`) {
		t.Fatal("manual Android Remote must not clear certificate pinning")
	}
	if !strings.Contains(manualHandler, "validFingerprint(fp)") || !strings.Contains(manualHandler, "expectedFingerprint = fp;") {
		t.Fatal("manual Android Remote must require and retain the operator-provided TLS fingerprint")
	}
}

func TestAndroidRemoteDeclaresLauncherIcon(t *testing.T) {
	manifestPath := filepath.Join("..", "android", "app", "src", "main", "AndroidManifest.xml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(data)
	for _, required := range []string{
		`android:icon="@drawable/ic_launcher"`,
		`android:roundIcon="@drawable/ic_launcher"`,
	} {
		if !strings.Contains(manifest, required) {
			t.Fatalf("Android launcher icon manifest entry missing: %s", required)
		}
	}
	iconPath := filepath.Join("..", "android", "app", "src", "main", "res", "drawable", "ic_launcher.xml")
	icon, err := os.ReadFile(iconPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(icon), `android:viewportWidth="108"`) || !strings.Contains(string(icon), `#06b6d4`) {
		t.Fatal("Android launcher vector icon does not look like the LocalCode icon")
	}
}
