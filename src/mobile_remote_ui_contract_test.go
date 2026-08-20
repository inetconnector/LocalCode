// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileRemoteComposerControlsAreWired(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, required := range []string{
		`$('#pairForm').onsubmit=pair`,
		`$$('.tab').forEach(b=>b.onclick=()=>showView(b.dataset.view))`,
		`$('#attachBtn').onclick=()=>$('#fileInput').click()`,
		`$('#voiceBtn').onclick=startVoiceInput`,
		`$('#fileInput').onchange=async e=>{await addFiles(e.target.files||[]);e.target.value=''}`,
		`composer.onsubmit=send`,
		`$('#stopBtn').onclick=async()=>`,
		`$('#sendBtn').onclick=send`,
		`$('#engineSelect').onchange=e=>selectEditingEngine(e.target.value)`,
		`$('#projectSelect').onchange=e=>`,
		`window.localCodeVoiceResult=text=>appendPromptText(text)`,
		`if(window.LocalCodeAndroid?.startVoiceInput)`,
		`const attachments=state.files.map(({name,mime,size,data})=>({name,mime,size,data}))`,
		`if(state.sending)return`,
		`state.sending=true;updateSendState()`,
		`$('#prompt').value=''`,
		`state.files=[]`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile Remote control contract missing %q", required)
		}
	}
}

func TestAndroidRemoteInputBridgeClosesCallbacksAndSurfacesErrors(t *testing.T) {
	path := filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "remote", "MainActivity.java")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	for _, required := range []string{
		`cancelPendingFileChooser();`,
		`callback.onReceiveValue(null);`,
		`showRemoteError(`,
		`webView.evaluateJavascript("window.alert(" + JSONObject.quote(visibleMessage) + ")", null);`,
		`intent.resolveActivity(getPackageManager()) == null`,
		`No voice input app was found.`,
		`No compatible file picker app was found.`,
		`webView.removeJavascriptInterface("LocalCodeAndroid")`,
		`Locale.getDefault().getLanguage().equalsIgnoreCase("de")`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Android Remote input reliability marker missing %q", required)
		}
	}
}
