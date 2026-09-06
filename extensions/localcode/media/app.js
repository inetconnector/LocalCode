'use strict';
(() => {
  const vscode = acquireVsCodeApi();
  const $ = id => document.getElementById(id);
  let state = {}, strings = {}, previousEvents = '', previousModels = '', pendingID = '', sending = false;
  const saved = vscode.getState() || {};
  $('prompt').value = saved.prompt || '';
  const post = message => vscode.postMessage(message);
  const text = key => strings[key] || key;
  function persist() { vscode.setState({ prompt: $('prompt').value, model: $('model').value }); }
  function button(label, action) { const b = document.createElement('button'); b.textContent = label; b.addEventListener('click', action); return b; }
  function inline(node, value) {
    for (const part of value.split(/(`[^`\n]+`|\*\*[^*\n]+\*\*)/g)) {
      const tag = part.startsWith('`') && part.endsWith('`') ? 'code' : part.startsWith('**') && part.endsWith('**') ? 'strong' : null;
      if (!tag) node.append(document.createTextNode(part));
      else { const el = document.createElement(tag); el.textContent = tag === 'code' ? part.slice(1, -1) : part.slice(2, -2); node.append(el); }
    }
  }
  function markdown(node, value) {
    const parts = String(value || '').split(/```([^\n]*)\n([\s\S]*?)```/g);
    for (let i = 0; i < parts.length; i += 3) {
      if (parts[i]) { const p = document.createElement('div'); inline(p, parts[i]); node.append(p); }
      if (parts[i + 2] !== undefined) {
        const pre = document.createElement('pre'); pre.className = 'code'; pre.textContent = parts[i + 2];
        node.append(button(text('copy'), () => navigator.clipboard.writeText(pre.textContent)), pre);
      }
    }
  }
  function diff(node, value) {
    for (const line of String(value || '').split('\n')) {
      const span = document.createElement('span'); span.textContent = line + '\n';
      span.className = line.startsWith('+') ? 'diff-add' : line.startsWith('-') ? 'diff-del' : line.startsWith('@@') ? 'diff-hunk' : '';
      node.append(span);
    }
  }
  function renderEvents() {
    const signature = JSON.stringify([state.events, state.language]);
    if (signature === previousEvents) return;
    previousEvents = signature;
    const history = $('history'); const bottom = history.scrollHeight - history.scrollTop - history.clientHeight < 100;
    const fragment = document.createDocumentFragment();
    for (const event of state.events || []) {
      if (event.type === 'approval_required') continue;
      const item = document.createElement('article'); item.className = 'event';
      if (['user', 'assistant', 'tool_result', 'tool_start', 'status', 'warning', 'error'].includes(event.type)) item.classList.add(event.type);
      const head = document.createElement('div'); head.className = 'event-head';
      const label = document.createElement('span'); label.textContent = event.type === 'user' ? text('user') : event.action || text('assistant');
      head.append(label, button(text('copy'), () => navigator.clipboard.writeText([event.message, event.detail].filter(Boolean).join('\n')))); item.append(head);
      const body = document.createElement('div'); body.className = 'event-body'; markdown(body, event.message); item.append(body);
      if (event.detail || event.preview || event.command) {
        const details = document.createElement('details'), summary = document.createElement('summary'), pre = document.createElement('pre');
        summary.textContent = text('details'); diff(pre, [event.command, event.preview, event.detail].filter(Boolean).join('\n'));
        details.append(summary, pre); item.append(details);
      }
      if (event.path) item.append(button(text('openFile'), () => post({ type: 'openFile', path: event.path })));
      fragment.append(item);
    }
    $('events').replaceChildren(fragment);
    if (bottom) history.scrollTop = history.scrollHeight;
  }
  function render() {
    strings = state.strings || strings; document.documentElement.lang = state.language || 'en';
    for (const node of document.querySelectorAll('[data-text]')) node.textContent = text(node.dataset.text);
    for (const node of document.querySelectorAll('[data-title]')) { node.title = text(node.dataset.title); node.setAttribute('aria-label', node.title); }
    $('prompt').placeholder = text('placeholder'); $('model').setAttribute('aria-label', text('model'));
    $('connection').textContent = text(state.connected ? (state.running ? 'running' : 'connected') : 'disconnected');
    $('connection').classList.toggle('online', !!state.connected);
    $('workspace').textContent = state.project ? state.project.split(/[\\/]/).filter(Boolean).pop() : text('chooseWorkspace'); $('workspace').title = state.project || '';
    $('welcome').hidden = !!state.events?.length;
    $('historyLimit').hidden = !state.limited;
    $('engine').textContent = state.engine || 'LocalCode'; $('engine').title = text('noApprovals');
    const models = JSON.stringify([state.models, state.model, state.language]);
    if (models !== previousModels) {
      const current = $('model').value || saved.model || state.model || '';
      const names = [...new Set([state.model, ...(state.models || []).map(m => m.name)].filter(Boolean))];
      $('model').replaceChildren(new Option(text('defaultModel'), ''), ...names.map(name => new Option(name, name)));
      $('model').value = names.includes(current) ? current : state.model || ''; previousModels = models;
    }
    $('context').hidden = !state.attachments?.length;
    $('contextList').replaceChildren(...(state.attachments || []).map(label => { const li = document.createElement('li'); li.textContent = label; return li; }));
    $('approval').hidden = !state.pending;
    pendingID = state.pending?.id || '';
    $('approvalText').textContent = [state.pending?.message, state.pending?.action, state.pending?.path, state.pending?.command, state.pending?.preview].filter(Boolean).join('\n');
    $('send').hidden = false; $('stop').hidden = !state.running;
    $('send').title = text(state.running ? 'steer' : 'send'); $('send').setAttribute('aria-label', $('send').title);
    $('send').disabled = !!state.busy || sending || !$('prompt').value.trim();
    $('approve').disabled = !!state.busy; $('reject').disabled = !!state.busy;
    renderEvents();
  }
  function send() {
    if (sending || state.busy || !$('prompt').value.trim()) return;
    sending = true; render(); post({ type: 'send', message: $('prompt').value, model: $('model').value });
  }
  document.querySelectorAll('[data-action]').forEach(node => node.addEventListener('click', () => { $('menu').open = false; post({ type: node.dataset.action }); }));
  document.querySelectorAll('[data-prompt]').forEach(node => node.addEventListener('click', () => { $('prompt').value = text(node.dataset.prompt); persist(); render(); $('prompt').focus(); }));
  $('prompt').addEventListener('input', () => { persist(); render(); });
  $('prompt').addEventListener('paste', () => { setTimeout(() => { persist(); render(); }, 0); });
  $('prompt').addEventListener('keydown', e => { if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); } });
  $('model').addEventListener('change', persist); $('send').addEventListener('click', send);
  $('approve').addEventListener('click', () => post({ type: 'approve', id: pendingID, decision: 'once' }));
  $('reject').addEventListener('click', () => post({ type: 'approve', id: pendingID, decision: 'reject' }));
  window.addEventListener('message', ({ data }) => {
    if (data.type === 'state') { state = data; if (!data.busy) sending = false; render(); }
    if (data.type === 'sent') { if ($('prompt').value === data.prompt) $('prompt').value = ''; sending = false; $('error').hidden = true; $('notice').hidden = !data.queued; $('notice').textContent = data.queued ? text('queued') : ''; persist(); render(); }
    if (data.type === 'insertText') {
      const p = $('prompt'); const start = p.selectionStart || 0, end = p.selectionEnd || 0, v = p.value || '';
      p.value = v.slice(0, start) + (data.text || '') + v.slice(end);
      p.selectionStart = p.selectionEnd = start + (data.text || '').length;
      persist(); render(); p.focus();
    }
    if (data.type === 'error') { $('error').textContent = data.message; $('error').hidden = false; sending = false; render(); }
  });
  post({ type: 'ready' });
})();
