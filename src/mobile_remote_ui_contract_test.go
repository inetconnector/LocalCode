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

func TestRemoteQRInstantPairingAndAutoDismissContract(t *testing.T) {
	// 1. Verify PairRemoteDevice broadcasts remote_paired event immediately for Desktop auto-dismiss
	cfg := defaultConfig()
	cfg.RemoteEnabled = true
	state := NewAppState(cfg, NewOllamaClient())
	t.Cleanup(state.Close)
	state.RemoteTLSFingerprint = "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
	ch := state.Subscribe()
	defer state.Unsubscribe(ch)

	code, _, _, err := state.StartRemotePairing()
	if err != nil {
		t.Fatalf("StartRemotePairing failed: %v", err)
	}
	token, dev, err := state.PairRemoteDevice(code, "Pixel 9")
	if err != nil || token == "" || dev.ID == "" {
		t.Fatalf("PairRemoteDevice failed: token=%q dev=%+v err=%v", token, dev, err)
	}
	select {
	case ev := <-ch:
		if ev.Type != "remote_paired" || ev.Action != dev.ID {
			t.Fatalf("expected remote_paired event for %s, got %+v", dev.ID, ev)
		}
	default:
		t.Fatal("expected immediate remote_paired broadcast after PairRemoteDevice")
	}

	// 2. Verify Desktop UI auto-closes pairing modals on remote_paired & polls /api/remote/devices
	indexBytes, err := os.ReadFile(filepath.Join("static", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexHTML := string(indexBytes)
	for _, required := range []string{
		`function closeRemotePairingModals()`,
		`ev.type==='remote_paired'`,
		`state.pairWatchTimer=setInterval`,
		`detailObj.id||req.action||req.id`,
	} {
		if !strings.Contains(indexHTML, required) {
			t.Fatalf("desktop index.html auto-dismiss contract missing %q", required)
		}
	}

	// 3. Verify Remote UI supports instant pairWithCode + hashchange without flashing manual screen
	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	remoteHTML := string(remoteBytes)
	for _, required := range []string{
		`async function pairWithCode(rawCode)`,
		`window.addEventListener('hashchange'`,
		`pairWithCode:async code=>pairWithCode(code)`,
	} {
		if !strings.Contains(remoteHTML, required) {
			t.Fatalf("mobile remote.html instant QR pairing contract missing %q", required)
		}
	}

	// 4. Verify Android manifest singleTop & MainActivity direct pairWithCode handoff
	manifestBytes, err := os.ReadFile(filepath.Join("..", "android", "app", "src", "main", "AndroidManifest.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestBytes), `android:launchMode="singleTop"`) {
		t.Fatal("AndroidManifest.xml must declare singleTop so QR deep-links reuse the activity")
	}
	javaBytes, err := os.ReadFile(filepath.Join("..", "android", "app", "src", "main", "java", "com", "inetconnector", "localcode", "MainActivity.java"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(javaBytes), `LocalCodeRemoteControl.pairWithCode(`) {
		t.Fatal("MainActivity.java must hand scanned QR codes directly to LocalCodeRemoteControl.pairWithCode")
	}
	if strings.Contains(string(javaBytes), `LocalCodeRemote.pairWithCode(`) {
		t.Fatal("MainActivity.java must not call the broken LocalCodeRemote.pairWithCode global")
	}
}

func TestMobileRemoteSecurityAndBehaviorFixes(t *testing.T) {
	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(remoteBytes)

	// 1. Keystore-backed auth token bridge integration
	for _, required := range []string{
		`window.LocalCodeAndroid?.getAuthToken`,
		`window.LocalCodeAndroid?.setAuthToken`,
		`window.LocalCodeAndroid?.clearAuthToken`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("remote.html missing Android Keystore token bridge call %q", required)
		}
	}

	// 2. renderMarkdown must escape HTML before applying Markdown formatting
	rmStart := strings.Index(page, "function renderMarkdown(raw){")
	if rmStart < 0 {
		t.Fatal("renderMarkdown not found in remote.html")
	}
	rmEnd := strings.Index(page[rmStart:], "\nasync function api(")
	if rmEnd < 0 {
		t.Fatal("could not isolate renderMarkdown body")
	}
	rmBody := page[rmStart : rmStart+rmEnd]
	escIdx := strings.Index(rmBody, "esc(String(raw))")
	boldIdx := strings.Index(rmBody, "<strong>$1</strong>")
	if escIdx < 0 || boldIdx < 0 || escIdx > boldIdx {
		t.Fatalf("renderMarkdown must escape raw HTML before Markdown formatting; escIdx=%d boldIdx=%d", escIdx, boldIdx)
	}

	// 3. STT result must NOT auto-send
	voiceIdx := strings.Index(page, "window.localCodeVoiceResult=")
	if voiceIdx < 0 {
		t.Fatal("localCodeVoiceResult handler missing")
	}
	voiceLine := page[voiceIdx : voiceIdx+strings.Index(page[voiceIdx:], "\n")]
	if strings.Contains(voiceLine, "send()") {
		t.Fatalf("localCodeVoiceResult must not auto-send voice transcription: %s", voiceLine)
	}

	// 4. refresh() must gate select-project behind initial restore instead of spamming every 5s
	if !strings.Contains(page, "state.projectInitialized") || !strings.Contains(page, "state.syncedProject") {
		t.Fatal("refresh() must track projectInitialized/syncedProject to avoid 5s select-project spam")
	}
}

func TestMobileRemoteOfflineOutboxContract(t *testing.T) {
	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(remoteBytes)

	for _, required := range []string{
		`localcodeRemoteOutboxV1`,
		`function loadOfflineOutbox()`,
		`function saveOfflineOutbox(`,
		`function enqueueOfflinePrompt(`,
		`async function flushOfflineOutbox()`,
		`window.addEventListener('online'`,
		`outbox_queued:'Offline — Nachricht lokal zwischengespeichert`,
		`outbox_queued:'Offline — prompt queued locally`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile remote.html offline outbox contract missing %q", required)
		}
	}

	// Ensure offline outbox helpers never store auth tokens in the outbox payload
	enqueueIdx := strings.Index(page, "function enqueueOfflinePrompt(")
	flushIdx := strings.Index(page, "async function flushOfflineOutbox(")
	if enqueueIdx < 0 || flushIdx < 0 || flushIdx <= enqueueIdx {
		t.Fatal("could not isolate enqueueOfflinePrompt in remote.html")
	}
	enqueueBody := page[enqueueIdx:flushIdx]
	if strings.Contains(enqueueBody, "state.token") || strings.Contains(enqueueBody, "loadAuthToken") {
		t.Fatal("offline outbox must never persist auth tokens in localStorage")
	}
}

func TestMobileRemotePrioritySteeringContract(t *testing.T) {
	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(remoteBytes)

	for _, required := range []string{
		`steer_sent:'Hinweis an laufenden Agenten gesendet.'`,
		`steer_sent:'Steering note sent to running agent.'`,
		`/remote/api/steer`,
		`state.status?.running&&msg&&attachments.length===0`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile remote.html priority steering contract missing %q", required)
		}
	}
}

func TestMobileRemoteSelfUpdateVoiceBargeInAndFrameCheckContract(t *testing.T) {
	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(remoteBytes)

	for _, required := range []string{
		`id="checkUpdateBtn"`,
		`window.LocalCodeAndroid?.checkForUpdate`,
		`/remote/api/apk-update`,
		`/remote/api/verify-frame`,
		`async function verifyAttachedImageFrame(`,
		`function startVoiceInput(){stopSpeaking();`,
		`check_update:'Auf App-Update prüfen'`,
		`check_update:'Check for App Update'`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile remote.html Wave 7 contract missing %q", required)
		}
	}
}

func TestMobileRemoteUnifiedPushRegistrationContract(t *testing.T) {
	remoteBytes, err := os.ReadFile(filepath.Join("static", "remote.html"))
	if err != nil {
		t.Fatal(err)
	}
	page := string(remoteBytes)

	for _, required := range []string{
		`id="pushEndpointInput"`,
		`id="savePushEndpointBtn"`,
		`/remote/api/push/register-endpoint`,
		`push_endpoint_label:'UnifiedPush-Endpunkt (Zero-Secret)'`,
		`push_endpoint_label:'UnifiedPush endpoint (Zero-Secret)'`,
		`provider:'unifiedpush'`,
	} {
		if !strings.Contains(page, required) {
			t.Fatalf("mobile remote.html Wave 11 UnifiedPush contract missing %q", required)
		}
	}
}
