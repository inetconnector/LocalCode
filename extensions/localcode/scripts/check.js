'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
for (const dir of ['src', 'media', 'scripts', 'test']) {
  for (const file of fs.readdirSync(path.join(root, dir)).filter(f => f.endsWith('.js'))) execFileSync(process.execPath, ['--check', path.join(root, dir, file)], { stdio: 'inherit' });
}
const assert = require('node:assert/strict');
for (const [a, b] of [['package.nls.json', 'package.nls.de.json'], ['media/en.json', 'media/de.json']]) assert.deepEqual(Object.keys(require(path.join(root, a))).sort(), Object.keys(require(path.join(root, b))).sort());
const pkg = require('../package.json');
const catalog = require('../package.nls.json');
for (const token of JSON.stringify(pkg).matchAll(/%([a-zA-Z]+)%/g)) assert.ok(catalog[token[1]], token[1]);
console.log('JavaScript syntax and German/English catalog parity passed.');
