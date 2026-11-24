// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAndroidRemoteLocksNavigationToPinnedPrivateOrigin(t *testing.T) {
	path := filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "MainActivity.java")
	data, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "remote", "MainActivity.java")
		data, err = os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
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
	manualEnd := strings.Index(text, "tabManualLayout.addView(open")
	if manualEnd < 0 {
		manualEnd = strings.Index(text, "discoveryPanel.addView(open")
	}
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

func TestAndroidRemoteEnforcesStrictTLSPinningHTTPSKeystoreAndBlackScreenFixes(t *testing.T) {
	javaPath := filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "MainActivity.java")
	javaBytes, err := os.ReadFile(javaPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(javaBytes)

	// Negative security assertions: no TLS pinning bypass, no HTTP fallback, no hardcoded IP, no unconditional debug
	for _, forbidden := range []string{
		`isPrivateHost(`,
		`isPrivateHost(currentRemoteUrl)`,
		`!"http".equalsIgnoreCase(scheme)`,
		`probeLocalCodeEndpoint("http"`,
		`192.168.1.94`,
		`WebView.setWebContentsDebuggingEnabled(true)`,
		`LocalCodeRemote.pairWithCode(`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("Android Remote contains forbidden insecure pattern: %s", forbidden)
		}
	}

	// Positive assertions: strict pinning warning, debug gate, Android Keystore AES-256-GCM, FrameLayout & lifecycle hooks
	for _, required := range []string{
		`ApplicationInfo.FLAG_DEBUGGABLE`,
		`KeyGenParameterSpec`,
		`AES/GCM/NoPadding`,
		`public String getAuthToken()`,
		`public void setAuthToken(String token)`,
		`public void clearAuthToken()`,
		`FrameLayout root = new FrameLayout(this)`,
		`onRenderProcessGone(WebView view, RenderProcessGoneDetail detail)`,
		`onReceivedHttpError(WebView view, WebResourceRequest request, WebResourceResponse errorResponse)`,
		`public void onBackPressed()`,
		`protected void onSaveInstanceState(Bundle outState)`,
		`protected void onRestoreInstanceState(Bundle savedInstanceState)`,
		`showTlsSecurityWarning(observed)`,
		`LocalCodeRemoteControl.pairWithCode(`,
		`handleRelayPairUri(data)`,
		`PREF_RELAY_AUTH_ENC`,
		`getRelayPairingSummary()`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Android Remote hardening/fix missing: %s", required)
		}
	}

	// Manifest assertions: HTTPS-only + configChanges
	manifestPath := filepath.Join("..", "android", "app", "src", "main", "AndroidManifest.xml")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(manifestBytes)
	if strings.Contains(manifest, `android:usesCleartextTraffic="true"`) {
		t.Fatal("AndroidManifest.xml must not enable cleartext traffic")
	}
	if !strings.Contains(manifest, `android:usesCleartextTraffic="false"`) {
		t.Fatal("AndroidManifest.xml must explicitly disable cleartext traffic")
	}
	if !strings.Contains(manifest, `android:configChanges="orientation|screenSize|screenLayout|keyboardHidden"`) {
		t.Fatal("AndroidManifest.xml must declare configChanges to prevent activity recreation black screens")
	}
	if !strings.Contains(manifest, `android:host="relay-pair"`) {
		t.Fatal("AndroidManifest.xml must declare localcode://relay-pair deep link host")
	}

	// Build script assertions: no hardcoded UNC path or password, env-driven signing + VERSION.txt versioning
	scriptPath := filepath.Join("..", "scripts", "build-android.ps1")
	scriptBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	script := string(scriptBytes)
	for _, forbidden := range []string{
		`diskstation`,
		`4711Google`,
	} {
		if strings.Contains(strings.ToLower(script), strings.ToLower(forbidden)) {
			t.Fatalf("scripts/build-android.ps1 still contains hardcoded secret/path %q", forbidden)
		}
	}
	for _, required := range []string{
		`LOCALCODE_ANDROID_KEYSTORE`,
		`LOCALCODE_ANDROID_KEYSTORE_PASS`,
		`LOCALCODE_ANDROID_KEY_ALIAS`,
		`LOCALCODE_ANDROID_KEY_PASS`,
		`android-signing.clixml`,
		`ConfigureReleaseSigning`,
		`Export-Clixml`,
		`Import-Clixml`,
		`android-debug.keystore`,
		`VERSION.txt`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("scripts/build-android.ps1 missing required signing/versioning directive %q", required)
		}
	}
}
