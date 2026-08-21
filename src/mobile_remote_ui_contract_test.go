// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMobileRemoteAllInteractiveControlsAreWired(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, required := range []string{
		`$('#pairForm').onsubmit=pair`,
		`$$('.tab').forEach(b=>b.onclick=()=>showView(b.dataset.view))`,
		`host.querySelectorAll('[data-thread]').forEach(b=>b.onclick=()=>selectThread(b.dataset.thread))`,
		`host.querySelectorAll('[data-project]').forEach(b=>b.onclick=()=>{state.project=b.dataset.project;showView('projects');render()})`,
		`$('#createProjectBtn').onclick=createProject`,
		`$('#createFolderBtn').onclick=createFolder`,
		`$('#deleteProjectBtn').onclick=deleteSelectedProject`,
		`$('#trashBtn').onclick=()=>showView('trash')`,
		`$('#playStoreBtn').onclick=playStoreBuild`,
		`host.querySelectorAll('[data-manage-project]').forEach(b=>b.onclick=()=>{state.project=b.dataset.manageProject;render()})`,
		`host.querySelectorAll('[data-restore]').forEach(b=>b.onclick=()=>restoreTrashProject(b.dataset.restore))`,
		`host.querySelectorAll('[data-purge]').forEach(b=>b.onclick=()=>purgeTrashProject(b.dataset.purge,b.dataset.name))`,
		`host.querySelectorAll('[data-decision]').forEach(b=>b.onclick=()=>approve(b.dataset.decision))`,
		`box.querySelectorAll('[data-remove]').forEach(b=>b.onclick=()=>{state.files.splice(Number(b.dataset.remove),1);renderFiles()})`,
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
			t.Fatalf("mobile Remote interactive control contract missing %q", required)
		}
	}
}

func TestMobileRemoteComposerButtonsDoNotImplicitlySubmit(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, required := range []string{
		`id="attachBtn" class="round" type="button"`,
		`id="voiceBtn" class="round" type="button"`,
		`id="stopBtn" class="round primary hidden" type="button"`,
		`id="sendBtn" class="round send" type="button"`,
		`id="pairBtn" type="submit"`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile Remote button type contract missing %q", required)
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
