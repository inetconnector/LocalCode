'use strict';
const vscode = require('vscode');
const fs = require('node:fs/promises');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawn } = require('node:child_process');
const { Client } = require('./client');
const { samePath, contained, validMessage } = require('./safety');
const catalogs = { en: require('../media/en.json'), de: require('../media/de.json') };

class LocalCode {
  constructor(context) {
    this.context = context; this.views = new Set(); this.editor = null; this.client = null; this.connected = false;
    this.thread = ''; this.project = ''; this.events = []; this.pending = null; this.status = {}; this.attachments = [];
    this.busy = false; this.disposed = false; this.generation = 0; this.baselines = new Map(); this.lastEditor = vscode.window.activeTextEditor;
    this.output = vscode.window.createOutputChannel('LocalCode');
    context.subscriptions.push(this.output, this,
      vscode.window.registerWebviewViewProvider('localcode.chat', this, { webviewOptions: { retainContextWhenHidden: true } }),
      vscode.workspace.registerTextDocumentContentProvider('localcode-review', { provideTextDocumentContent: uri => this.baselines.get(uri.toString()) || '' }),
      vscode.window.onDidChangeActiveTextEditor(editor => { if (editor) this.lastEditor = editor; }),
      vscode.workspace.onDidChangeConfiguration(e => { if (e.affectsConfiguration('localcode')) this.run(() => this.connect()); }),
      vscode.workspace.onDidChangeWorkspaceFolders(() => { this.project = ''; this.thread = ''; this.attachments = []; this.run(() => this.connect()); })
    );
    const commands = {
      open: () => vscode.commands.executeCommand('localcode.chat.focus'),
      openRight: async () => { try { await vscode.commands.executeCommand('workbench.action.moveView', { viewId: 'localcode.chat', destination: 'auxiliaryBar' }); } catch {} await vscode.commands.executeCommand('localcode.chat.focus'); },
      openEditor: () => this.openEditor(),
      newTask: () => this.newTask(),
      addSelection: async () => { await this.addContext(); await vscode.commands.executeCommand('localcode.chat.focus'); },
      start: () => this.start(),
      settings: () => vscode.commands.executeCommand('workbench.action.openSettings', '@ext:inetconnector.localcode')
    };
    for (const [name, fn] of Object.entries(commands)) context.subscriptions.push(vscode.commands.registerCommand(`localcode.${name}`, () => this.run(fn)));
  }
  get language() {
    const choice = vscode.workspace.getConfiguration('localcode').get('language', 'auto');
    const value = choice === 'auto' ? (this.status.system_language || vscode.env.language) : choice;
    return String(value).toLowerCase().startsWith('de') ? 'de' : 'en';
  }
  t(key) { return catalogs[this.language][key] || key; }
  async run(fn) { try { return await fn(); } catch (err) { this.error(err); } }
  error(err) {
    const key = err.code === 'ECONNREFUSED' || err.code === 'ECONNRESET' ? 'disconnected' : err.message;
    const text = this.t(key);
    this.broadcast({ type: 'error', message: text });
    // Deliberately exclude response bodies, prompts, paths and credentials from logs.
    this.output.appendLine(`error: ${err.code || (catalogs.en[key] ? key : 'request_failed')}`);
    if (!this.views.size) vscode.window.showErrorMessage(this.t('failure') + ': ' + text);
    return text;
  }
  assertTrust() {
    if (!vscode.workspace.isTrusted) throw new Error('trust');
    if (vscode.env.remoteName || !vscode.workspace.workspaceFolders?.some(f => f.uri.scheme === 'file')) throw new Error('noWorkspace');
  }
  async workspace(choose = false) {
    this.assertTrust();
    const folders = vscode.workspace.workspaceFolders.filter(f => f.uri.scheme === 'file');
    let folder = folders.find(f => samePath(f.uri.fsPath, this.project));
    if (choose || !folder) {
      folder = folders.length === 1 ? folders[0] : (await vscode.window.showQuickPick(folders.map(f => ({ label: f.name, description: f.uri.fsPath, folder: f })), { placeHolder: this.t('chooseWorkspace') }))?.folder;
    }
    if (!folder) return false;
    if (this.status.root_dir) {
      try { await contained(this.status.root_dir, folder.uri.fsPath); }
      catch { throw new Error('backendRoot'); }
    }
    if (!samePath(this.project, folder.uri.fsPath)) { this.project = folder.uri.fsPath; this.thread = ''; this.events = []; this.pending = null; this.attachments = []; }
    return true;
  }
  async resolveWebviewView(view) { await this.attach(view); }
  async openEditor() {
    if (this.editor) { this.editor.reveal(); return; }
    this.editor = vscode.window.createWebviewPanel('localcode.editor', 'LocalCode', vscode.ViewColumn.Beside, { enableScripts: true, retainContextWhenHidden: true });
    this.editor.onDidDispose(() => { this.editor = null; });
    await this.attach(this.editor);
  }
  async attach(view) {
    const webview = view.webview;
    webview.options = { enableScripts: true, localResourceRoots: [vscode.Uri.joinPath(this.context.extensionUri, 'media')] };
    this.views.add(webview);
    view.onDidDispose(() => this.views.delete(webview));
    webview.onDidReceiveMessage(message => this.run(() => this.message(message)));
    const nonce = crypto.randomBytes(24).toString('hex');
    const asset = name => webview.asWebviewUri(vscode.Uri.joinPath(this.context.extensionUri, 'media', name)).toString();
    let html = await fs.readFile(path.join(this.context.extensionPath, 'media', 'index.html'), 'utf8');
    html = html.replaceAll('{{csp}}', webview.cspSource).replaceAll('{{nonce}}', nonce).replaceAll('{{style}}', asset('style.css')).replaceAll('{{script}}', asset('app.js'));
    webview.html = html;
  }
  broadcast(message) { for (const view of this.views) view.postMessage(message); }
  render() {
    this.broadcast({ type: 'state', language: this.language, strings: catalogs[this.language], connected: this.connected, project: this.project, thread: this.thread, events: this.events.slice(-500), limited: this.events.length > 500, pending: this.pending, running: !!this.snapshot?.running, models: this.status.models || [], model: this.snapshot?.model || this.status.selected_model || '', engine: this.status.editing_engine || '', attachments: this.attachments.map(a => a.label), busy: this.busy });
  }
  async connect() {
    this.assertTrust();
    const generation = ++this.generation;
    clearTimeout(this.pollTimer); clearTimeout(this.retryTimer); this.endStream?.(); this.client?.dispose(); this.connected = false;
    this.client = new Client(vscode.workspace.getConfiguration('localcode').get('serverUrl'), text => this.output.appendLine(text));
    this.render();
    const client = this.client;
    const ping = await client.request('/api/ping');
    if (ping.app !== 'LocalCode') throw new Error('invalidResponse');
    this.scopedStop = ping.capabilities?.includes('stop-task-v1');
    this.canSteer = ping.capabilities?.includes('steering-v1');
    const status = await client.request('/api/status');
    if (generation !== this.generation) return;
    this.status = status;
    if (!await this.workspace()) return;
    const threads = await client.request('/api/threads');
    const candidates = (threads.threads || []).filter(t => !t.archived && samePath(t.project, this.project));
    const saved = this.context.workspaceState.get('thread:' + this.project);
    this.thread = candidates.find(t => t.id === (this.thread || saved))?.id || candidates[0]?.id || '';
    this.connected = true;
    await this.refresh();
    this.stream(generation);
    this.schedulePoll(generation);
  }
  stream(generation) {
    if (generation !== this.generation || this.disposed) return;
    this.endStream = this.client.events(event => {
      if (generation !== this.generation || event.thread_id !== this.thread) return;
      if (!this.events.some(e => e.id === event.id)) this.events.push(event);
      this.events = this.events.slice(-2000);
      if (event.type === 'approval_required') this.pending = event;
      this.render();
    }, err => {
      if (generation !== this.generation || this.disposed) return;
      this.output.appendLine(`stream disconnected: ${err.code || 'stream_error'}`);
      this.retryTimer = setTimeout(() => { this.stream(generation); this.run(() => this.refresh()); }, 2500);
    });
  }
  schedulePoll(generation) {
    if (this.disposed || generation !== this.generation) return;
    this.pollTimer = setTimeout(async () => {
      if (this.views.size) await this.run(() => this.refresh());
      this.schedulePoll(generation);
    }, 4000);
  }
  async refresh() {
    const generation = this.generation, thread = this.thread, client = this.client;
    if (!client) return;
    if (!thread) { this.events = []; this.pending = null; this.snapshot = null; this.render(); return; }
    let snapshot;
    try { snapshot = await client.request('/api/snapshot?thread_id=' + encodeURIComponent(thread)); }
    catch (err) { if (generation === this.generation) { this.connected = false; this.render(); } throw err; }
    if (generation !== this.generation || thread !== this.thread) return;
    // The legacy API falls back to the current task for a missing ID. Never display or control that fallback.
    if (snapshot.current_thread !== thread || !samePath(snapshot.project, this.project)) { this.thread = ''; this.events = []; this.pending = null; this.snapshot = null; this.render(); throw new Error('taskGone'); }
    this.connected = true; this.snapshot = snapshot; this.events = snapshot.events || []; this.pending = snapshot.pending; this.render();
  }
  async newTask() {
    if (!this.connected) await this.connect();
    if (!await this.workspace()) return;
    const global = await this.client.request('/api/snapshot');
    if (global.running) throw new Error('busy');
    const result = await this.client.request('/api/new-chat', { project: this.project });
    this.thread = result.thread.id; this.events = []; this.pending = null;
    await this.context.workspaceState.update('thread:' + this.project, this.thread);
    await this.refresh();
    await vscode.commands.executeCommand('localcode.chat.focus');
  }
  async chooseTask() {
    if (!this.connected) await this.connect();
    const result = await this.client.request('/api/threads');
    const tasks = (result.threads || []).filter(t => !t.archived && samePath(t.project, this.project));
    if (!tasks.length) { vscode.window.showInformationMessage(this.t('noTasks')); return; }
    const task = await vscode.window.showQuickPick(tasks.map(t => ({ label: t.title, description: t.model, id: t.id })), { placeHolder: this.t('chooseTask') });
    if (!task) return;
    this.thread = task.id; this.attachments = [];
    await this.context.workspaceState.update('thread:' + this.project, this.thread);
    await this.refresh();
  }
  async addContext() {
    if (!await this.workspace()) return;
    const editor = vscode.window.activeTextEditor || this.lastEditor;
    const items = [];
    if (editor && editor.document.uri.scheme === 'file') {
      try {
        await contained(this.project, editor.document.uri.fsPath);
        const rel = path.relative(this.project, editor.document.uri.fsPath);
        const hasSelection = !editor.selection.isEmpty;
        items.push({
          id: 'active',
          label: `$(file-text) ${this.t('attachActive')}: ${rel}${hasSelection ? ` (L${editor.selection.start.line + 1}-L${editor.selection.end.line + 1})` : ''}`,
          description: this.t(editor.document.isDirty ? 'unsaved' : 'saved')
        });
      } catch {}
    }
    items.push(
      { id: 'choose', label: `$(folder) ${this.t('attachFile')}` },
      { id: 'clipboard', label: `$(clippy) ${this.t('attachClipboard')}` },
      { id: 'diagnostics', label: `$(warning) ${this.t('attachDiagnostics')}` }
    );
    const pick = await vscode.window.showQuickPick(items, { placeHolder: this.t('addContext') });
    if (!pick) return;
    if (pick.id === 'active' && editor) {
      const selection = editor.selection;
      const text = editor.document.getText(selection.isEmpty ? undefined : selection);
      const label = `${path.relative(this.project, editor.document.uri.fsPath)}:${selection.isEmpty ? 1 : selection.start.line + 1} (${this.t(editor.document.isDirty ? 'unsaved' : 'saved')})`;
      this.addAttachment(label, text);
      this.render();
    } else if (pick.id === 'choose') {
      const uris = await vscode.window.showOpenDialog({
        canSelectFiles: true,
        canSelectFolders: false,
        canSelectMany: false,
        defaultUri: vscode.Uri.file(this.project),
        openLabel: this.t('chooseFile')
      });
      if (uris && uris[0]) {
        const file = uris[0].fsPath;
        await contained(this.project, file);
        const content = await fs.readFile(file, 'utf8');
        const label = path.relative(this.project, file);
        this.addAttachment(label, content);
        this.render();
      }
    } else if (pick.id === 'clipboard') {
      const clipText = await vscode.env.clipboard.readText();
      if (!clipText || !clipText.trim()) {
        vscode.window.showInformationMessage(this.t('clipboardEmpty'));
        return;
      }
      this.addAttachment(this.t('clipboardLabel'), clipText);
      this.render();
    } else if (pick.id === 'diagnostics') {
      await this.addDiagnostics();
    }
  }
  async pasteClipboard() {
    const text = await vscode.env.clipboard.readText();
    if (!text || !text.trim()) {
      vscode.window.showInformationMessage(this.t('clipboardEmpty'));
      return;
    }
    this.broadcast({ type: 'insertText', text });
  }
  addAttachment(label, text) {
    if (Buffer.byteLength(text) + this.attachments.reduce((n, a) => n + Buffer.byteLength(a.text), 0) > 65536) throw new Error('contextLimit');
    this.attachments.push({ label, text });
  }
  async addDiagnostics() {
    if (!await this.workspace()) return;
    const lines = [];
    for (const [uri, diagnostics] of vscode.languages.getDiagnostics()) {
      if (uri.scheme !== 'file') continue;
      try { await contained(this.project, uri.fsPath); } catch { continue; }
      for (const d of diagnostics.filter(d => d.severity <= vscode.DiagnosticSeverity.Warning).slice(0, 20)) lines.push(`${path.relative(this.project, uri.fsPath)}:${d.range.start.line + 1}: ${d.message.slice(0, 1000)}`);
      if (lines.length >= 100) break;
    }
    if (!lines.length) { vscode.window.showInformationMessage(this.t('noDiagnostics')); return; }
    this.addAttachment(this.t('diagnostics'), lines.join('\n')); this.render();
  }
  async send(message) {
    if (!this.connected) await this.connect();
    if (!await this.workspace()) return;
    const current = await this.client.request('/api/snapshot');
    if (current.running) {
      if (current.current_thread !== this.thread || !samePath(current.project, this.project)) throw new Error('otherRun');
      if (!this.canSteer) throw new Error('steerUpgrade');
      const prompt = message.message + (this.attachments.length ? '\n\n' + this.t('contextInstruction') + '\n' + JSON.stringify(this.attachments) : '');
      if (Buffer.byteLength(prompt) > 32768) throw new Error('steerLimit');
      if (this.steeringAttempt?.message !== prompt || this.steeringAttempt?.run_id !== current.run_id) this.steeringAttempt = { message: prompt, id: crypto.randomUUID(), thread_id: this.thread, run_id: current.run_id };
      await this.client.request('/api/steer', this.steeringAttempt);
      this.steeringAttempt = null; this.attachments = [];
      this.broadcast({ type: 'sent', prompt: message.message, queued: true });
      await this.refresh(); return;
    }
    const dirty = [];
    for (const doc of vscode.workspace.textDocuments.filter(d => d.isDirty && d.uri.scheme === 'file')) {
      try { await contained(this.project, doc.uri.fsPath); dirty.push(doc); } catch { /* other workspace */ }
    }
    if (dirty.length) {
      if (await vscode.window.showWarningMessage(this.t('dirty'), { modal: true }, this.t('save')) !== this.t('save')) return;
      for (const doc of dirty) if (!await doc.save()) throw new Error('saveFailed');
    }
    if (!this.thread) await this.newTask();
    const prompt = message.message + (this.attachments.length ? '\n\n' + this.t('contextInstruction') + '\n' + JSON.stringify(this.attachments) : '');
    await this.client.request('/api/chat', { message: prompt, model: message.model, project: this.project, thread_id: this.thread, attachments: [] });
    this.attachments = []; this.broadcast({ type: 'sent', prompt: message.message }); await this.refresh();
  }
  async stop() {
    if (!this.scopedStop) throw new Error('stopUpgrade');
    if (!this.thread) throw new Error('noTask');
    const snapshot = await this.client.request('/api/snapshot');
    if (!snapshot.running || snapshot.current_thread !== this.thread) throw new Error('otherRun');
    if (await vscode.window.showWarningMessage(this.t('sharedStop'), { modal: true }, this.t('confirmStop')) !== this.t('confirmStop')) return;
    // Recheck after the dialog. The backend atomically enforces the same run identity.
    await this.client.request('/api/stop-task', { thread_id: this.thread, run_id: snapshot.run_id });
    await this.refresh();
  }
  async approve(message) {
    await this.refresh();
    if (!this.pending || this.pending.id !== message.id) throw new Error('pendingStale');
    await this.client.request('/api/approve', { id: message.id, approve: message.decision === 'once', decision: message.decision });
    this.pending = null; this.render();
  }
  async openFile(file) {
    if (!await this.workspace()) return;
    const full = await contained(this.project, path.resolve(this.project, file));
    await vscode.window.showTextDocument(vscode.Uri.file(full), { preview: true });
  }
  async review() {
    if (!await this.workspace()) return;
    const extension = vscode.extensions.getExtension('vscode.git');
    if (!extension) throw new Error('gitUnavailable');
    const git = (await extension.activate()).getAPI(1);
    const repo = git.repositories.find(r => samePath(r.rootUri.fsPath, this.project));
    if (!repo) throw new Error('notGit');
    await repo.status();
    const changes = [...repo.state.indexChanges, ...repo.state.workingTreeChanges, ...(repo.state.untrackedChanges || [])];
    const unique = [...new Map(changes.map(c => [c.uri.fsPath, c])).values()];
    if (!unique.length) {
      vscode.window.showInformationMessage(this.t('noChanges'));
      this.broadcast({ type: 'error', message: this.t('noChanges') });
      return;
    }
    const pick = await vscode.window.showQuickPick(unique.map(c => ({ label: path.relative(this.project, c.uri.fsPath), change: c })), { placeHolder: this.t('review') });
    if (!pick) return;
    const file = pick.change.uri.fsPath;
    // Check the parent even for deleted paths, without following a junction outside the workspace.
    await contained(this.project, path.dirname(file));
    let exists = true;
    try { await fs.lstat(file); } catch (err) { if (err.code === 'ENOENT') exists = false; else throw err; }
    if (exists) await contained(this.project, file);
    const relative = path.relative(this.project, pick.change.originalUri?.fsPath || file).replaceAll('\\', '/');
    let before = '';
    if (repo.state.HEAD?.commit) {
      try { before = await repo.show(repo.state.HEAD.commit, relative); } catch (err) {
        // New/untracked files have no HEAD blob. Other errors must remain visible.
        if (![1, 7].includes(pick.change.status)) throw err;
      }
    }
    if (Buffer.byteLength(before) > 4 * 1024 * 1024) throw new Error('tooLarge');
    const base = vscode.Uri.parse(`localcode-review:/${crypto.randomBytes(8).toString('hex')}/${encodeURIComponent(path.basename(file))}`);
    this.baselines.set(base.toString(), before);
    const after = exists ? vscode.Uri.file(file) : base.with({ path: base.path + '.deleted' });
    await vscode.commands.executeCommand('vscode.diff', base, after, `${this.t('diff')}: ${pick.label}`);
  }
  async start() {
    this.assertTrust();
    let executable = vscode.workspace.getConfiguration('localcode').get('executablePath', '');
    if (!executable && process.platform === 'win32' && process.env.LOCALAPPDATA) executable = path.join(process.env.LOCALAPPDATA, 'Programs', 'LocalCode', 'LocalCode.exe');
    if (!path.isAbsolute(executable) || (process.platform === 'win32' && !executable.toLowerCase().endsWith('.exe'))) throw new Error('executable');
    try { if (!(await fs.stat(executable)).isFile()) throw new Error(); } catch { throw new Error('executable'); }
    // Shared desktop process: explicit launch, no shell, never killed when the IDE closes.
    const child = spawn(executable, [], { cwd: path.dirname(executable), shell: false, windowsHide: true, detached: true, stdio: 'ignore', env: { ...process.env, LOCALCODE_FAST_START: '1' } });
    await new Promise((resolve, reject) => { child.once('spawn', resolve); child.once('error', reject); });
    child.unref(); this.output.appendLine('LocalCode launch requested');
    vscode.window.showInformationMessage(this.t('launched'));
  }
  async message(message) {
    if (!validMessage(message)) throw new Error('invalidRequest');
    this.assertTrust();
    if (message.type === 'ready') { this.render(); if (!this.client) await this.connect(); return; }
    if (this.busy) return;
    this.busy = true; this.render();
    try {
      switch (message.type) {
        case 'refresh': await this.connect(); break;
        case 'send': await this.send(message); break;
        case 'newTask': await this.newTask(); break;
        case 'chooseTask': await this.chooseTask(); break;
        case 'chooseWorkspace': if (await this.workspace(true)) await this.connect(); break;
        case 'addContext': await this.addContext(); break;
        case 'pasteClipboard': await this.pasteClipboard(); break;
        case 'openRight': await vscode.commands.executeCommand('localcode.openRight'); break;
        case 'addDiagnostics': await this.addDiagnostics(); break;
        case 'clearContext': this.attachments = []; break;
        case 'stop': await this.stop(); break;
        case 'approve': await this.approve(message); break;
        case 'openFile': await this.openFile(message.path); break;
        case 'review': await this.review(); break;
        case 'start': await this.start(); break;
        case 'settings': await vscode.commands.executeCommand('localcode.settings'); break;
        case 'desktop': await vscode.env.openExternal(vscode.Uri.parse(this.client?.base || new Client(vscode.workspace.getConfiguration('localcode').get('serverUrl')).base)); break;
      }
    } finally { this.busy = false; this.render(); }
  }
  dispose() { this.disposed = true; ++this.generation; clearTimeout(this.pollTimer); clearTimeout(this.retryTimer); this.endStream?.(); this.client?.dispose(); this.baselines.clear(); }
}
function activate(context) { return new LocalCode(context); }
module.exports = { activate, LocalCode };
