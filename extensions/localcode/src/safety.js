'use strict';
const fs = require('node:fs/promises');
const path = require('node:path');

function samePath(a, b) {
  if (!a || !b) return false;
  const clean = p => process.platform === 'win32' ? path.resolve(p).toLowerCase() : path.resolve(p);
  return clean(a) === clean(b);
}
async function contained(root, file) {
  let canonicalRoot = await fs.realpath(root);
  let canonicalFile = await fs.realpath(file);
  if (process.platform === 'win32') {
    if (canonicalRoot.length >= 2 && canonicalRoot[1] === ':') {
      canonicalRoot = canonicalRoot[0].toUpperCase() + canonicalRoot.slice(1);
    }
    if (canonicalFile.length >= 2 && canonicalFile[1] === ':') {
      canonicalFile = canonicalFile[0].toUpperCase() + canonicalFile.slice(1);
    }
  }
  const relative = path.relative(canonicalRoot, canonicalFile);
  if (relative === '..' || relative.startsWith('..' + path.sep) || path.isAbsolute(relative)) throw new Error('outsideWorkspace');
  return canonicalFile;
}
function validMessage(m) {
  if (!m || typeof m !== 'object' || Array.isArray(m)) return false;
  const simple = ['ready', 'refresh', 'newTask', 'chooseTask', 'chooseWorkspace', 'addContext', 'addDiagnostics', 'clearContext', 'start', 'settings', 'desktop', 'review', 'stop'];
  if (simple.includes(m.type)) return Object.keys(m).length === 1;
  if (m.type === 'send') return typeof m.message === 'string' && m.message.trim().length > 0 && m.message.length <= 100000 && typeof m.model === 'string' && m.model.length <= 256 && Object.keys(m).length === 3;
  if (m.type === 'approve') return typeof m.id === 'string' && m.id.length <= 128 && ['once', 'reject'].includes(m.decision) && Object.keys(m).length === 3;
  if (m.type === 'openFile') return typeof m.path === 'string' && m.path.length <= 4096 && Object.keys(m).length === 2;
  return false;
}
module.exports = { samePath, contained, validMessage };
