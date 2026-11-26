'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');

const { verifyParity, t } = require('../src/i18n');
const {
  validateLoopbackEndpoint,
  resolveFimTemplate,
  buildFimPrompt,
  cleanFimCompletion,
  FimAutocompleteProvider
} = require('../src/fim_autocomplete');
const {
  INLINE_EDIT_SCHEME,
  stripCodeFences,
  extractInlineContext,
  InlineEditController
} = require('../src/inline_edit');

async function startTestHttpServer(t, handler) {
  const server = http.createServer(handler);
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => server.close(resolve)));
  const addr = server.address();
  return `http://127.0.0.1:${addr.port}`;
}

function makeMockDocument(filePath, content, languageId = 'javascript') {
  const text = String(content);
  const lines = text.split('\n');
  const offsetAt = pos => {
    let off = 0;
    for (let i = 0; i < pos.line && i < lines.length; i++) {
      off += lines[i].length + 1;
    }
    return Math.min(text.length, off + (pos.character || 0));
  };
  return {
    uri: { scheme: 'file', fsPath: filePath },
    languageId,
    lineCount: lines.length,
    getText(range) {
      if (!range) return text;
      return text.slice(offsetAt(range.start), offsetAt(range.end));
    },
    lineAt(line) {
      const lineText = lines[line] || '';
      return {
        text: lineText,
        range: {
          start: { line, character: 0 },
          end: { line, character: lineText.length }
        }
      };
    },
    offsetAt
  };
}

test('i18n DE/EN parity and manifest completeness for Inline Edit & FIM', () => {
  assert.equal(verifyParity(), true);
  assert.equal(t('en', 'inlineEditAccept'), 'Accept');
  assert.equal(t('de', 'inlineEditAccept'), 'Übernehmen');

  const pkg = require('../package.json');
  const nlsEn = require('../package.nls.json');
  const nlsDe = require('../package.nls.de.json');

  assert.deepEqual(Object.keys(nlsEn).sort(), Object.keys(nlsDe).sort());

  const cmdIds = new Set((pkg.contributes?.commands || []).map(c => c.command));
  for (const id of [
    'localcode.inlineEdit',
    'localcode.inlineEditAccept',
    'localcode.inlineEditReject',
    'localcode.toggleFim'
  ]) {
    assert.equal(cmdIds.has(id), true, `Missing contributed command: ${id}`);
  }

  const keybindings = pkg.contributes?.keybindings || [];
  assert.equal(
    keybindings.some(k => k.command === 'localcode.inlineEdit' && k.key === 'ctrl+k' && k.mac === 'cmd+k'),
    true,
    'Missing Ctrl+K / Cmd+K keybinding for localcode.inlineEdit'
  );

  const props = pkg.contributes?.configuration?.properties || {};
  for (const settingKey of [
    'localcode.fimEnabled',
    'localcode.fimModel',
    'localcode.fimEndpoint',
    'localcode.fimDebounceMs',
    'localcode.fimLatencyBudgetMs',
    'localcode.fimTemplate',
    'localcode.inlineEditEndpoint',
    'localcode.inlineEditModel'
  ]) {
    assert.ok(props[settingKey], `Missing configuration property: ${settingKey}`);
  }

  for (const match of JSON.stringify(pkg).matchAll(/%([a-zA-Z]+)%/g)) {
    const token = match[1];
    assert.ok(nlsEn[token], `Missing EN translation for %${token}%`);
    assert.ok(nlsDe[token], `Missing DE translation for %${token}%`);
  }
});

test('validateLoopbackEndpoint enforces strict loopback HTTP URLs', () => {
  assert.equal(validateLoopbackEndpoint('http://127.0.0.1:11434').url, 'http://127.0.0.1:11434');
  assert.equal(validateLoopbackEndpoint('http://localhost:8080/').hostname, '127.0.0.1');
  assert.equal(validateLoopbackEndpoint('http://[::1]:11434').url, 'http://[::1]:11434');

  for (const bad of [
    '',
    'https://127.0.0.1:11434',
    'http://192.168.1.50:11434',
    'http://api.openai.com',
    'http://user:pass@127.0.0.1:11434',
    'http://127.0.0.1:11434?x=1',
    'http://127.0.0.1:11434/../etc'
  ]) {
    assert.throws(() => validateLoopbackEndpoint(bad), /fimInvalidEndpoint/);
  }
});

test('buildFimPrompt and cleanFimCompletion handle sentinels and suffix overlap', () => {
  assert.equal(resolveFimTemplate('qwen2.5-coder:1.5b', 'auto'), 'qwen');
  assert.equal(resolveFimTemplate('starcoder2:3b', 'auto'), 'starcoder');
  assert.equal(resolveFimTemplate('codellama:7b-code', 'auto'), 'codellama');

  const qwenPrompt = buildFimPrompt('function add(a, b) {\n  ', '\n}', 'qwen2.5-coder:1.5b');
  assert.equal(qwenPrompt.template, 'qwen');
  assert.ok(qwenPrompt.prompt.startsWith('<|fim_prefix|>'));
  assert.ok(qwenPrompt.prompt.includes('<|fim_suffix|>\n}<|fim_middle|>'));

  const cleaned = cleanFimCompletion('return a + b;\n}<|endoftext|>ignored', '\n}');
  assert.equal(cleaned, 'return a + b;');
  assert.equal(cleanFimCompletion('   ', '}'), '');
});

test('FimAutocompleteProvider performs loopback completion, LRU caching, and timeout enforcement', async t => {
  let requests = 0;
  let lastBody = null;
  let mode = 'fast';

  const baseUrl = await startTestHttpServer(t, async (req, res) => {
    requests++;
    const chunks = [];
    for await (const c of req) chunks.push(c);
    lastBody = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (mode === 'slow') {
      await new Promise(r => setTimeout(r, 250));
      if (res.writableEnded || req.destroyed) return;
    }
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ response: 'return a + b;<|endoftext|>' }));
  });

  const provider = new FimAutocompleteProvider({
    vscode: {
      workspace: { isTrusted: true },
      InlineCompletionItem: function (insertText, range) {
        this.insertText = insertText;
        this.range = range;
      },
      Range: function (start, end) {
        this.start = start;
        this.end = end;
      }
    },
    config: {
      enabled: true,
      model: 'qwen2.5-coder:1.5b',
      endpoint: baseUrl,
      debounceMs: 0,
      latencyBudgetMs: 1200,
      template: 'qwen'
    }
  });
  t.after(() => provider.dispose());

  const doc = makeMockDocument('/repo/math.js', 'function add(a, b) {\n  \n}');
  const pos = { line: 1, character: 2 };

  const items1 = await provider.provideInlineCompletionItems(doc, pos, {}, null);
  assert.equal(items1.length, 1);
  assert.equal(items1[0].insertText, 'return a + b;');
  assert.equal(requests, 1);
  assert.equal(lastBody.raw, true);

  // Second identical call should hit LRU cache without HTTP traffic.
  const items2 = await provider.provideInlineCompletionItems(doc, pos, {}, null);
  assert.equal(items2.length, 1);
  assert.equal(items2[0].insertText, 'return a + b;');
  assert.equal(requests, 1);
  assert.equal(provider.getStats().hits, 1);

  // Timeout budget test on uncached position with strict short budget.
  mode = 'slow';
  provider.clearCache();
  provider.reloadConfig({
    enabled: true,
    model: 'qwen2.5-coder:1.5b',
    endpoint: baseUrl,
    debounceMs: 0,
    latencyBudgetMs: 60,
    template: 'qwen'
  });
  const timedOutItems = await provider.provideInlineCompletionItems(doc, { line: 1, character: 0 }, {}, null);
  assert.deepEqual(timedOutItems, []);
  assert.equal(provider.getStats().timeouts, 1);

  // Non-loopback endpoints are rejected fail-closed without throwing.
  provider.clearCache();
  provider.reloadConfig({
    enabled: true,
    model: 'qwen2.5-coder:1.5b',
    endpoint: 'https://remote.example.com',
    debounceMs: 0,
    latencyBudgetMs: 150
  });
  const blockedItems = await provider.provideInlineCompletionItems(doc, pos, {}, null);
  assert.deepEqual(blockedItems, []);
});

test('InlineEditController generates virtual diff, applies accept(), handles reject(), and enforces workspace boundary', async t => {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), 'lc-inline-edit-'));
  t.after(() => fs.rm(dir, { recursive: true, force: true }));

  const targetFile = path.join(dir, 'sample.js');
  const outsideFile = path.join(os.tmpdir(), 'outside.js');
  const originalContent = 'function greet() {\n  return "hi";\n}\n';
  await fs.writeFile(targetFile, originalContent, 'utf8');

  const baseUrl = await startTestHttpServer(t, async (req, res) => {
    const chunks = [];
    for await (const c of req) chunks.push(c);
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    assert.ok(body.prompt.includes('Make greet accept name'));
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(
      JSON.stringify({
        response: '```javascript\nfunction greet(name) {\n  return `hi ${name}`;\n}\n```'
      })
    );
  });

  const executed = [];
  const appliedEdits = [];
  const messages = [];

  function WorkspaceEdit() {
    this.edits = [];
    this.replace = (uri, range, newText) => this.edits.push({ uri, range, newText });
  }

  let nextPromptDecision = 'PreviewOnly';
  const mockVscode = {
    EventEmitter: function () {
      this.event = () => ({ dispose() {} });
      this.fire = () => {};
      this.dispose = () => {};
    },
    Uri: {
      file: p => ({ scheme: 'file', fsPath: p }),
      parse: s => ({ scheme: INLINE_EDIT_SCHEME, toString: () => s })
    },
    WorkspaceEdit,
    workspace: {
      isTrusted: true,
      workspaceFolders: [{ name: 'repo', uri: { scheme: 'file', fsPath: dir } }],
      applyEdit: async edit => {
        appliedEdits.push(edit);
        return true;
      }
    },
    window: {
      showInputBox: async () => 'Make greet accept name',
      showInformationMessage: async msg => {
        messages.push(msg);
        return nextPromptDecision;
      }
    },
    commands: {
      executeCommand: async (...args) => {
        executed.push(args);
      }
    }
  };

  assert.equal(stripCodeFences('```js\nconst x = 1;\n```'), 'const x = 1;');

  const ctrl = new InlineEditController({
    vscode: mockVscode,
    getLanguage: () => 'en',
    getWorkspaceRoot: () => dir,
    getConfig: () => ({
      endpoint: baseUrl,
      model: 'qwen2.5-coder:1.5b',
      timeoutMs: 2000
    })
  });
  t.after(() => ctrl.dispose());

  const doc = makeMockDocument(targetFile, originalContent, 'javascript');
  const ctx = extractInlineContext(doc, {
    isEmpty: false,
    start: { line: 0, character: 0 },
    end: { line: 2, character: 1 }
  });
  assert.equal(ctx.selectedText, 'function greet() {\n  return "hi";\n}');

  const editor = {
    document: doc,
    selection: {
      isEmpty: false,
      start: { line: 0, character: 0 },
      end: { line: 2, character: 1 },
      active: { line: 0, character: 0 }
    }
  };

  // Run -> Preview -> Accept
  const res = await ctrl.run(editor);
  assert.equal(res.status, 'preview');
  assert.equal(executed.length, 1);
  assert.equal(executed[0][0], 'vscode.diff');
  const proposedUri = executed[0][2];
  assert.equal(
    ctrl.provider.provideTextDocumentContent(proposedUri),
    'function greet(name) {\n  return `hi ${name}`;\n}\n'
  );

  const acceptRes = await ctrl.accept();
  assert.equal(acceptRes.status, 'applied');
  assert.equal(appliedEdits.length, 1);
  assert.equal(ctrl.provider.provideTextDocumentContent(proposedUri), '');

  // Run -> Reject
  const res2 = await ctrl.run(editor, 'Make greet accept name');
  assert.equal(res2.status, 'preview');
  const rejectRes = await ctrl.reject();
  assert.equal(rejectRes.status, 'rejected');
  assert.equal(appliedEdits.length, 1);

  // Outside workspace must reject fail-closed
  const outsideDoc = makeMockDocument(outsideFile, 'const bad = true;\n');
  await assert.rejects(
    () =>
      ctrl.run(
        {
          document: outsideDoc,
          selection: { isEmpty: false, start: { line: 0, character: 0 }, end: { line: 0, character: 5 } }
        },
        'Refactor'
      ),
    /inlineEditOutsideWorkspace/
  );
});

test('runLoopbackAdversarialReview enforces loopback SSRF guard and parses live AgentOS verdicts (DE/EN)', async t => {
  const { runLoopbackAdversarialReview, formatAdversarialSummary } = require('../src/adversarial_review');

  // 1. Fail-closed SSRF guard blocks non-loopback AgentOS URL
  await assert.rejects(
    () => runLoopbackAdversarialReview({ agentosUrl: 'http://169.254.169.254:8010', codeOrDiff: 'x = 1\n' }),
    /invalidEndpoint/
  );

  // 2. Real loopback HTTP roundtrip
  const baseUrl = await startTestHttpServer(t, async (req, res) => {
    assert.equal(req.url, '/v1/routing/adversarial-review');
    const chunks = [];
    for await (const c of req) chunks.push(c);
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    assert.equal(body.author_provider, 'anthropic');
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(
      JSON.stringify({
        verdict: 'APPROVED',
        reviewer_provider: 'openai',
        reviewer_model: 'openai-o3-pro',
        finding_counts: { critical: 0, high: 0, medium: 0, total: 0 },
        findings: []
      })
    );
  });

  const report = await runLoopbackAdversarialReview({
    agentosUrl: baseUrl,
    authorProvider: 'anthropic',
    codeOrDiff: 'export const ok = true;\n',
    lang: 'de'
  });
  assert.equal(report.verdict, 'APPROVED');
  assert.match(formatAdversarialSummary(report, 'de'), /Prüfer: openai\/openai-o3-pro/);
  assert.match(formatAdversarialSummary(report, 'en'), /Reviewer: openai\/openai-o3-pro/);
});
