'use strict';
const assert = require('node:assert/strict');
const http = require('node:http');
const vscode = require('vscode');
const fs = require('node:fs/promises');
const path = require('node:path');
async function run() {
  const project = vscode.workspace.workspaceFolders[0].uri.fsPath;
  const posts = []; let running = false, pending = null, missingTask = false, failFirstSteer = true;
  const server = http.createServer((req, res) => {
    let data = ''; req.on('data', b => { data += b; }); req.on('end', () => {
      if (req.url === '/api/events') { res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.write(': connected\n\n'); return; }
      const body = data ? JSON.parse(data) : undefined;
      if (body) posts.push({ path: req.url, body });
      let result;
      switch (req.url.split('?')[0]) {
        case '/api/ping': result = { app: 'LocalCode', capabilities: ['stop-task-v1', 'steering-v1'] }; break;
        case '/api/status': result = { root_dir: project, models: [{ name: 'fixture-model' }], selected_model: 'fixture-model', system_language: 'de', editing_engine: 'native' }; break;
        case '/api/threads': result = { threads: [{ id: 't1', project, title: 'Fixture', model: 'fixture-model' }] }; break;
        case '/api/snapshot': result = { project, current_thread: missingTask ? 'other-task' : 't1', model: 'fixture-model', running, run_id: 'r1', pending, events: [{ id: 'e1', thread_id: 't1', type: 'final', message: 'Hello from fixture' }] }; break;
        case '/api/chat': running = true; result = { ok: true }; break;
        case '/api/steer': if (failFirstSteer) { failFirstSteer = false; req.socket.destroy(); return; } result = { ok: true }; break;
        case '/api/approve': pending = null; result = { ok: true }; break;
        default: res.statusCode = 404; result = { error: req.url };
      }
      res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(result));
    });
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const config = vscode.workspace.getConfiguration('localcode');
  await config.update('serverUrl', 'http://127.0.0.1:' + server.address().port, vscode.ConfigurationTarget.Global);
  await config.update('language', 'auto', vscode.ConfigurationTarget.Global);
  const extension = vscode.extensions.getExtension('inetconnector.localcode'); assert.ok(extension, 'extension discovered');
  const api = await extension.activate();
  try {
    await api.connect();
    assert.equal(api.language, 'de'); assert.equal(api.thread, 't1'); assert.equal(api.connected, true);
    const commands = await vscode.commands.getCommands(true);
    for (const name of ['open', 'openEditor', 'newTask', 'addSelection', 'start', 'settings']) assert.ok(commands.includes('localcode.' + name), name);
    await vscode.commands.executeCommand('localcode.openEditor');
    const file = path.join(project, 'hello.js'); await fs.writeFile(file, 'const value = 42;\n');
    const editor = await vscode.window.showTextDocument(vscode.Uri.file(file));
    editor.selection = new vscode.Selection(0, 6, 0, 11);
    await api.addContext(); assert.equal(api.attachments[0].text, 'value');
    await api.send({ message: 'Explain the selection', model: 'fixture-model' });
    const chat = posts.find(p => p.path === '/api/chat');
    assert.equal(chat.body.project, project); assert.equal(chat.body.thread_id, 't1'); assert.match(chat.body.message, /value/);
    await assert.rejects(api.send({ message: 'Use German and focus on naming', model: 'fixture-model' }));
    await api.send({ message: 'Use German and focus on naming', model: 'fixture-model' });
    assert.equal(posts.filter(p => p.path === '/api/chat').length, 1, 'steering does not start another run');
    assert.equal(posts.find(p => p.path === '/api/steer').body.run_id, 'r1');
    assert.equal(posts.filter(p => p.path === '/api/steer')[0].body.id, posts.filter(p => p.path === '/api/steer')[1].body.id, 'uncertain retries retain the same id');
    pending = { id: 'approval-1', message: 'Write hello.js', preview: '+ test', action: 'write_file' };
    await api.approve({ id: 'approval-1', decision: 'once' });
    assert.equal(posts.find(p => p.path === '/api/approve').body.decision, 'once');
    await assert.rejects(api.approve({ id: 'stale', decision: 'once' }), /pendingStale/);
    await api.openFile(file);
    missingTask = true;
    await assert.rejects(api.refresh(), /taskGone/);
    assert.equal(api.thread, ''); assert.deepEqual(api.events, [], 'never display fallback task history');
    console.log('LOCALCODE HOST E2E PASSED: activation, commands, editor webview, DE auto language, selection, task-bound chat, priority steering, approval, file opening');
  } finally { api.editor?.dispose(); api.dispose(); server.closeAllConnections(); await new Promise(resolve => server.close(resolve)); }
}
module.exports = { run };
