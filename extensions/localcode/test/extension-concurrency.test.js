'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const Module = require('node:module');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');

function loadExtensionWithMockVscode(workspaceRoot) {
  const executedCommands = [];
  const stateStore = new Map();
  const mockVscode = {
    window: {
      activeTextEditor: null,
      createOutputChannel: () => ({ appendLine() {}, dispose() {} }),
      registerWebviewViewProvider: () => ({ dispose() {} }),
      onDidChangeActiveTextEditor: () => ({ dispose() {} }),
      showErrorMessage: () => Promise.resolve(),
      showInformationMessage: () => Promise.resolve(),
      showWarningMessage: () => Promise.resolve()
    },
    workspace: {
      isTrusted: true,
      workspaceFolders: [{ name: path.basename(workspaceRoot), uri: { scheme: 'file', fsPath: workspaceRoot } }],
      textDocuments: [],
      registerTextDocumentContentProvider: () => ({ dispose() {} }),
      onDidChangeConfiguration: () => ({ dispose() {} }),
      onDidChangeWorkspaceFolders: () => ({ dispose() {} }),
      getConfiguration: () => ({ get: (_, def) => def })
    },
    commands: {
      registerCommand: () => ({ dispose() {} }),
      executeCommand: (...args) => { executedCommands.push(args); return Promise.resolve(); }
    },
    env: { language: 'en', remoteName: undefined },
    Uri: { file: p => ({ scheme: 'file', fsPath: p }), parse: s => ({ toString: () => s }) },
    DiagnosticSeverity: { Warning: 1 }
  };

  const origLoad = Module._load;
  Module._load = function (request, parent, isMain) {
    if (request === 'vscode') return mockVscode;
    return origLoad.call(this, request, parent, isMain);
  };
  const extPath = require.resolve('../src/extension');
  delete require.cache[extPath];
  try {
    const { LocalCode } = require('../src/extension');
    const context = {
      subscriptions: [],
      extensionPath: path.resolve(__dirname, '..'),
      extensionUri: { fsPath: path.resolve(__dirname, '..') },
      workspaceState: {
        get: key => stateStore.get(key),
        update: (key, val) => { stateStore.set(key, val); return Promise.resolve(); }
      }
    };
    return { instance: new LocalCode(context), executedCommands };
  } finally {
    Module._load = origLoad;
  }
}

test('project-scoped run check allows newTask and send while another project is running', async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lc-ext-multi-'));
  t.after(() => fs.rm(base, { recursive: true, force: true }));
  const projectA = path.join(base, 'proj-a');
  const projectB = path.join(base, 'proj-b');
  await fs.mkdir(projectA);
  await fs.mkdir(projectB);

  const { instance } = loadExtensionWithMockVscode(projectA);
  t.after(() => instance.dispose());

  const requests = [];
  let runningProjects = [projectB];
  let snapshotForA = {
    project: projectB,
    current_thread: 'thread-b',
    running: true,
    running_projects: runningProjects,
    events: []
  };

  instance.connected = true;
  instance.project = projectA;
  instance.thread = 'thread-a';
  instance.canSteer = true;
  instance.status = { root_dir: base };
  instance.client = {
    dispose() {},
    async request(url, body) {
      requests.push({ url, body });
      if (url.startsWith('/api/snapshot')) {
        if (url.includes('thread_id=thread-a') || url.includes('thread_id=thread-a-new')) {
          const tid = url.includes('thread-a-new') ? 'thread-a-new' : 'thread-a';
          const projARunning = runningProjects.includes(projectA);
          return {
            project: projectA,
            current_thread: tid,
            running: projARunning,
            running_projects: runningProjects,
            run_id: projARunning ? 'run-a' : '',
            events: []
          };
        }
        return snapshotForA;
      }
      if (url === '/api/new-chat') {
        return { ok: true, thread: { id: 'thread-a-new', project: projectA } };
      }
      if (url === '/api/chat') {
        return { ok: true };
      }
      throw new Error('unexpected url: ' + url);
    }
  };

  // 1. Project B is running -> Project A must NOT be blocked for newTask() or send()
  await instance.newTask();
  assert.equal(instance.thread, 'thread-a-new');
  await instance.send({ message: 'Run build on project A', model: '' });
  assert.ok(requests.some(r => r.url === '/api/chat' && r.body.project === projectA), 'expected /api/chat for project A');

  // 2. When Project A itself is running on another thread -> newTask() must throw busy and send() must throw otherRun
  runningProjects = [projectA, projectB];
  snapshotForA = {
    project: projectA,
    current_thread: 'thread-a-other',
    running: true,
    running_projects: runningProjects,
    events: []
  };
  await assert.rejects(() => instance.newTask(), /busy/);

  instance.thread = 'thread-a-idle';
  instance.client.request = async (url) => {
    if (url.startsWith('/api/snapshot')) {
      return {
        project: projectA,
        current_thread: 'thread-a-idle',
        running: false,
        running_projects: [projectA],
        events: []
      };
    }
    throw new Error('should not dispatch while same project is busy');
  };
  await assert.rejects(() => instance.send({ message: 'Should block', model: '' }), /otherRun/);
});
