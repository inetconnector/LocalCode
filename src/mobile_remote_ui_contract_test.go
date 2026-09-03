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
		`$('#scanQrBtn').onclick=triggerQrScan`,
		`$('#settingsBtn').onclick=`,
		`$('#closeSettingsBtn').onclick=`,
		`$$('.tab').forEach(b=>b.onclick=()=>showView(b.dataset.view))`,
		`host.querySelectorAll('[data-thread]').forEach(b=>b.onclick=()=>selectThread(b.dataset.thread))`,
		`host.querySelectorAll('[data-project]').forEach(b=>b.onclick=()=>`,
		`$('#createProjectBtn').onclick=createProject`,
		`$('#createFolderBtn').onclick=createFolder`,
		`$('#deleteProjectBtn').onclick=deleteSelectedProject`,
		`$('#trashBtn').onclick=()=>showView('trash')`,
		`$('#playStoreBtn').onclick=playStoreBuild`,
		`host.querySelectorAll('[data-manage-project]').forEach(b=>b.onclick=()=>`,
		`host.querySelectorAll('[data-restore]').forEach(b=>b.onclick=()=>restoreTrashProject(b.dataset.restore))`,
		`host.querySelectorAll('[data-purge]').forEach(b=>b.onclick=()=>purgeTrashProject(b.dataset.purge,b.dataset.name))`,
		`host.querySelectorAll('[data-decision]').forEach(b=>b.onclick=()=>approve(b.dataset.decision))`,
		`const modal=$('#approvalModal'),host=$('#approvalDialog'),p=state.pending`,
		`function isTransientRunEvent(ev){return ['progress','action_running'].includes(ev.type)||ev.type==='status'}`,
		`const visible=state.events.filter(ev=>running||!isTransientRunEvent(ev))`,
		`box.querySelectorAll('[data-remove]').forEach(b=>b.onclick=()=>{state.files.splice(Number(b.dataset.remove),1);renderFiles()})`,
		`$('#attachBtn').onclick=()=>$('#fileInput').click()`,
		`$('#voiceBtn').onclick=startVoiceInput`,
		`$('#fileInput').onchange=async e=>{await addFiles(e.target.files||[]);e.target.value=''}`,
		`composer.onsubmit=send`,
		`$('#stopBtn').onclick=async()=>`,
		`$('#sendBtn').onclick=send`,
		`$('#engineSelect').onchange=e=>selectEditingEngine(e.target.value)`,
		`$('#projectSelect').onchange=e=>`,
		`window.localCodeVoiceResult=text=>{appendPromptText(text)`,
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

func TestMobileRemoteHidesTransientRunEventsAfterCompletion(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, required := range []string{
		`function isTransientRunEvent(ev){return ['progress','action_running'].includes(ev.type)||ev.type==='status'}`,
		`const host=$('#reviewView'),running=!!state.status?.running`,
		`const visible=state.events.filter(ev=>running||!isTransientRunEvent(ev))`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile Remote transient run-event cleanup missing %q", required)
		}
	}
}

func TestMobileRemoteStartsOnNewTaskTab(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	newTab := strings.Index(page, `data-view="new" data-i18n="new_task"`)
	tasksTab := strings.Index(page, `data-view="tasks" data-i18n="tasks"`)
	if newTab < 0 || tasksTab < 0 || newTab > tasksTab {
		t.Fatalf("new task tab must be the first visible mobile tab")
	}
	for _, required := range []string{
		`<div id="newView" class="view"></div>`,
		`<div id="tasksView" class="view hidden"></div>`,
		`view:'new'`,
		`const viewOrder=['new','tasks','projects','trash','review']`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile Remote new-task startup contract missing %q", required)
		}
	}
}

func TestMobileRemoteApprovalsUsePopupNotTab(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, forbidden := range []string{
		`data-view="approve"`,
		`id="approveView"`,
		`id="drawerApproveBtn"`,
		`showView('approve')`,
	} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("mobile Remote approval tab must be removed; found %q", forbidden)
		}
	}
	for _, required := range []string{
		`id="approvalModal"`,
		`id="approvalDialog"`,
		`.approval-modal`,
		`if(ev.type==='approval_required')state.pending=ev`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile Remote approval popup contract missing %q", required)
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

func TestAndroidRemotePersistsConnectionAndUsesLanProbeFallback(t *testing.T) {
	path := filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "MainActivity.java")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)

	for _, required := range []string{
		`getSharedPreferences(PREFS_NAME, MODE_PRIVATE)`,
		`loadSavedConnection();`,
		`persistConnection(target, expectedFingerprint);`,
		`clearSavedConnection();`,
		`startLanProbeDiscovery();`,
		`"/remote/api/discovery"`,
		`"/remote/api/ping"`,
		`NetworkInterface.getNetworkInterfaces()`,
		`HttpsURLConnection`,
		`No LocalCode instance found.`,
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Android Remote persistence/discovery marker missing %q", required)
		}
	}
}

func TestMobileRemoteTasksTabConfigurableAndHiddenByDefault(t *testing.T) {
	path := filepath.Join("static", "remote.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)

	for _, required := range []string{
		`id="tasksTabBtn" class="tab hidden" data-view="tasks"`,
		`id="showTasksTabToggle"`,
		`show_tasks_tab:'Tasks-Reiter anzeigen'`,
		`show_tasks_tab:'Show Tasks tab'`,
		`showTasksTab:localStorage.getItem('localcodeRemoteShowTasksTab')==='true'`,
		`function renderNavTabs()`,
		`$('#tasksTabBtn')||document.querySelector('.tab[data-view="tasks"]')`,
		`btn.classList.toggle('hidden',!state.showTasksTab)`,
		`$('#showTasksTabToggle').onchange=`,
		`localStorage.setItem('localcodeRemoteShowTasksTab',state.showTasksTab?'true':'false')`,
		`function effectiveViewOrder(){return state.showTasksTab?viewOrder:viewOrder.filter(v=>v!=='tasks')}`,
		`setShowTasksTab:enable=>`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile Remote tasks tab configurable contract missing %q", required)
		}
	}
}
