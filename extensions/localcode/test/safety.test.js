'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const { contained, validMessage } = require('../src/safety');
test('canonical workspace boundary rejects traversal and junction escapes', async t => {
  const base = await fs.mkdtemp(path.join(os.tmpdir(), 'lc-ide-test-'));
  t.after(() => fs.rm(base, { recursive: true, force: true }));
  const root = path.join(base, 'workspace'), outside = path.join(base, 'outside');
  await fs.mkdir(root); await fs.mkdir(outside); await fs.writeFile(path.join(root, 'safe.txt'), 'ok'); await fs.writeFile(path.join(outside, 'secret.txt'), 'test');
  assert.equal(await contained(root, path.join(root, 'safe.txt')), await fs.realpath(path.join(root, 'safe.txt')));
  await assert.rejects(contained(root, path.join(root, '..', 'outside', 'secret.txt')), /outsideWorkspace/);
  await fs.symlink(outside, path.join(root, 'escape'), process.platform === 'win32' ? 'junction' : 'dir');
  await assert.rejects(contained(root, path.join(root, 'escape', 'secret.txt')), /outsideWorkspace/);
});
test('bridge accepts narrow actions and forbids arbitrary endpoint, command and persistent approvals', () => {
  assert.ok(validMessage({ type: 'send', message: 'hello', model: '' }));
  assert.ok(validMessage({ type: 'approve', id: 'a', decision: 'once' }));
  for (const input of [null, [], { type: 'executeCommand', command: 'workbench.action.terminal.new' }, { type: 'refresh', url: 'http://evil' }, { type: 'send', message: '', model: '' }, { type: 'send', message: 'x'.repeat(100001), model: '' }, { type: 'approve', id: 'a', decision: 'global' }]) assert.equal(validMessage(input), false);
});
