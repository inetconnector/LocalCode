'use strict';
const { runTests } = require('@vscode/test-electron');
const fs = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
(async () => {
  const temp = await fs.mkdtemp(path.join(os.tmpdir(), 'localcode-ide-host-'));
  const workspace = path.join(temp, 'workspace'); await fs.mkdir(workspace);
  // Keep diagnostics under this isolated temporary directory for failed runs.
  console.log('Isolated host test directory: ' + temp);
  await runTests({
    ...(process.env.LOCALCODE_TEST_IDE ? { vscodeExecutablePath: process.env.LOCALCODE_TEST_IDE } : {}),
    extensionDevelopmentPath: path.resolve(__dirname, '..'),
    extensionTestsPath: path.resolve(__dirname, '../test/host-suite.js'),
    launchArgs: [workspace, '--user-data-dir=' + path.join(temp, 'user-data'), '--extensions-dir=' + path.join(temp, 'extensions'), '--disable-workspace-trust', '--skip-welcome', '--skip-release-notes', '--disable-updates'],
  });
})().catch(err => { console.error(err); process.exitCode = 1; });
