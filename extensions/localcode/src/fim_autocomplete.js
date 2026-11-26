'use strict';

const http = require('node:http');

const DEFAULT_FIM_MODEL = 'qwen2.5-coder:1.5b';
const DEFAULT_FIM_ENDPOINT = 'http://127.0.0.1:11434';
const DEFAULT_DEBOUNCE_MS = 120;
const DEFAULT_LATENCY_BUDGET_MS = 150;
const DEFAULT_CACHE_CAPACITY = 128;
const MAX_PREFIX_CHARS = 4096;
const MAX_SUFFIX_CHARS = 2048;
const MAX_RESPONSE_BYTES = 1024 * 1024;

const FIM_SENTINELS = [
  '<|fim_prefix|>',
  '<|fim_suffix|>',
  '<|fim_middle|>',
  '<|endoftext|>',
  '<|file_sep|>',
  '<|cursor|>',
  '<fim_prefix>',
  '<fim_suffix>',
  '<fim_middle>',
  '<EOT>',
  '<PRE>',
  '<SUF>',
  '<MID>'
];

function validateLoopbackEndpoint(rawUrl) {
  if (typeof rawUrl !== 'string' || !rawUrl.trim()) {
    throw new Error('fimInvalidEndpoint');
  }
  if (/\.\.|[\r\n\0\\]/.test(rawUrl)) {
    throw new Error('fimInvalidEndpoint');
  }
  let parsed;
  try {
    parsed = new URL(rawUrl.trim());
  } catch {
    throw new Error('fimInvalidEndpoint');
  }
  if (
    parsed.protocol !== 'http:' ||
    !['127.0.0.1', 'localhost', '[::1]'].includes(parsed.hostname) ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash
  ) {
    throw new Error('fimInvalidEndpoint');
  }
  if (parsed.pathname.includes('..') || /[\r\n\0]/.test(parsed.pathname)) {
    throw new Error('fimInvalidEndpoint');
  }
  if (parsed.hostname === 'localhost') {
    parsed.hostname = '127.0.0.1';
  }
  const cleanPath = parsed.pathname === '/' ? '' : parsed.pathname.replace(/\/+$/, '');
  return {
    origin: parsed.origin,
    hostname: parsed.hostname,
    port: parsed.port || '80',
    basePath: cleanPath,
    url: parsed.origin + cleanPath
  };
}

function resolveFimTemplate(model, templateSetting = 'auto') {
  const explicit = String(templateSetting || 'auto').toLowerCase().trim();
  if (explicit === 'qwen' || explicit === 'starcoder' || explicit === 'codellama') {
    return explicit;
  }
  const lowerModel = String(model || '').toLowerCase();
  if (lowerModel.includes('starcoder')) return 'starcoder';
  if (lowerModel.includes('codellama') || lowerModel.includes('code-llama')) return 'codellama';
  return 'qwen';
}

function buildFimPrompt(prefix, suffix, model = DEFAULT_FIM_MODEL, templateSetting = 'auto') {
  const clippedPrefix = String(prefix || '').slice(-MAX_PREFIX_CHARS);
  const clippedSuffix = String(suffix || '').slice(0, MAX_SUFFIX_CHARS);
  const template = resolveFimTemplate(model, templateSetting);

  if (template === 'starcoder') {
    return {
      template,
      prompt: `<fim_prefix>${clippedPrefix}<fim_suffix>${clippedSuffix}<fim_middle>`,
      stop: ['<|endoftext|>', '<fim_prefix>', '<fim_suffix>', '<fim_middle>', '<file_sep>', '\n\n\n']
    };
  }
  if (template === 'codellama') {
    return {
      template,
      prompt: `<PRE> ${clippedPrefix} <SUF>${clippedSuffix} <MID>`,
      stop: ['<EOT>', '<PRE>', '<SUF>', '<MID>', '</s>', '\n\n\n']
    };
  }
  return {
    template: 'qwen',
    prompt: `<|fim_prefix|>${clippedPrefix}<|fim_suffix|>${clippedSuffix}<|fim_middle|>`,
    stop: ['<|endoftext|>', '<|fim_prefix|>', '<|fim_suffix|>', '<|fim_middle|>', '<|file_sep|>', '\n\n\n']
  };
}

function cleanFimCompletion(raw, suffix = '') {
  if (typeof raw !== 'string' || !raw) return '';
  let text = raw;
  for (const token of FIM_SENTINELS) {
    if (text.includes(token)) {
      text = text.split(token)[0];
    }
  }
  text = text.replace(/\r\n/g, '\n');

  // Trim overlap if the model duplicated the start of the existing suffix.
  const suffixHead = String(suffix || '').replace(/\r\n/g, '\n').slice(0, 128);
  if (suffixHead.length > 0 && text.length > 0) {
    const maxOverlap = Math.min(text.length, suffixHead.length);
    for (let len = maxOverlap; len >= 2; len--) {
      if (text.endsWith(suffixHead.slice(0, len))) {
        text = text.slice(0, -len);
        break;
      }
    }
  }
  if (!text.trim()) return '';
  return text;
}

class LRUCache {
  constructor(capacity = DEFAULT_CACHE_CAPACITY) {
    this.capacity = Math.max(1, Number(capacity) || DEFAULT_CACHE_CAPACITY);
    this.map = new Map();
  }
  get(key) {
    if (!this.map.has(key)) return undefined;
    const value = this.map.get(key);
    this.map.delete(key);
    this.map.set(key, value);
    return value;
  }
  set(key, value) {
    if (this.map.has(key)) {
      this.map.delete(key);
    } else if (this.map.size >= this.capacity) {
      const oldestKey = this.map.keys().next().value;
      this.map.delete(oldestKey);
    }
    this.map.set(key, value);
  }
  has(key) {
    return this.map.has(key);
  }
  clear() {
    this.map.clear();
  }
  get size() {
    return this.map.size;
  }
}

function resolveFimRequestSpec(endpointInfo, model, fimPrompt) {
  const pathLower = endpointInfo.basePath.toLowerCase();
  if (pathLower.endsWith('/completion') || pathLower.endsWith('/infill') || (!pathLower && endpointInfo.port === '8080')) {
    const targetPath = pathLower ? endpointInfo.basePath : '/completion';
    return {
      url: endpointInfo.origin + targetPath,
      body: {
        prompt: fimPrompt.prompt,
        n_predict: 64,
        temperature: 0.0,
        stop: fimPrompt.stop,
        stream: false
      }
    };
  }
  if (pathLower.endsWith('/v1/completions')) {
    return {
      url: endpointInfo.origin + endpointInfo.basePath,
      body: {
        model,
        prompt: fimPrompt.prompt,
        max_tokens: 64,
        temperature: 0.0,
        stop: fimPrompt.stop,
        stream: false
      }
    };
  }
  const targetPath = pathLower.endsWith('/api/generate') ? endpointInfo.basePath : `${endpointInfo.basePath}/api/generate`;
  return {
    url: endpointInfo.origin + targetPath,
    body: {
      model,
      prompt: fimPrompt.prompt,
      raw: true,
      stream: false,
      options: {
        num_predict: 64,
        temperature: 0.0,
        stop: fimPrompt.stop
      }
    }
  };
}

function extractCompletionText(json) {
  if (!json || typeof json !== 'object') return '';
  if (typeof json.response === 'string') return json.response;
  if (typeof json.content === 'string') return json.content;
  if (typeof json.completion === 'string') return json.completion;
  if (Array.isArray(json.choices) && json.choices[0]) {
    if (typeof json.choices[0].text === 'string') return json.choices[0].text;
    if (json.choices[0].message && typeof json.choices[0].message.content === 'string') {
      return json.choices[0].message.content;
    }
  }
  return '';
}

function localLoopbackPostJson(rawTargetUrl, payload, signal, timeoutMs) {
  const endpointInfo = validateLoopbackEndpoint(rawTargetUrl);
  const data = JSON.stringify(payload);
  const byteLength = Buffer.byteLength(data, 'utf8');

  return new Promise((resolve, reject) => {
    if (signal && signal.aborted) {
      const err = new Error('aborted');
      err.code = 'ABORT_ERR';
      reject(err);
      return;
    }

    let settled = false;
    const finish = (fn, arg) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (signal && onAbort) {
        signal.removeEventListener('abort', onAbort);
      }
      fn(arg);
    };

    const req = http.request(
      endpointInfo.url,
      {
        method: 'POST',
        agent: false,
        headers: {
          Accept: 'application/json',
          'Content-Type': 'application/json',
          'Content-Length': byteLength
        }
      },
      res => {
        const chunks = [];
        let total = 0;
        res.on('data', chunk => {
          total += chunk.length;
          if (total > MAX_RESPONSE_BYTES) {
            const err = new Error('tooLarge');
            req.destroy(err);
            return;
          }
          chunks.push(chunk);
        });
        res.on('error', err => finish(reject, err));
        res.on('end', () => {
          if (res.statusCode < 200 || res.statusCode >= 300) {
            const err = new Error(`HTTP ${res.statusCode}`);
            err.statusCode = res.statusCode;
            finish(reject, err);
            return;
          }
          const raw = Buffer.concat(chunks).toString('utf8');
          try {
            finish(resolve, JSON.parse(raw));
          } catch {
            finish(reject, new Error('invalidResponse'));
          }
        });
      }
    );

    const onAbort = () => {
      const err = new Error('aborted');
      err.code = 'ABORT_ERR';
      req.destroy(err);
      finish(reject, err);
    };
    if (signal) {
      signal.addEventListener('abort', onAbort, { once: true });
    }

    const effectiveTimeout = Math.max(10, Number(timeoutMs) || DEFAULT_LATENCY_BUDGET_MS);
    const timer = setTimeout(() => {
      const err = new Error('timeout');
      err.code = 'ETIMEDOUT';
      req.destroy(err);
      finish(reject, err);
    }, effectiveTimeout);

    req.on('error', err => finish(reject, err));
    req.end(data);
  });
}

function getDocumentPrefixSuffix(document, position) {
  if (typeof document.offsetAt === 'function' && typeof document.getText === 'function') {
    const full = document.getText();
    const offset = document.offsetAt(position);
    return {
      prefix: full.slice(Math.max(0, offset - MAX_PREFIX_CHARS), offset),
      suffix: full.slice(offset, offset + MAX_SUFFIX_CHARS)
    };
  }
  const lineCount = document.lineCount || 0;
  const lines = [];
  for (let i = 0; i < lineCount; i++) {
    lines.push(document.lineAt(i).text);
  }
  const beforeLines = lines.slice(0, position.line);
  const currentLine = lines[position.line] || '';
  const afterLines = lines.slice(position.line + 1);
  const prefix = [...beforeLines, currentLine.slice(0, position.character)].join('\n');
  const suffix = [currentLine.slice(position.character), ...afterLines].join('\n');
  return {
    prefix: prefix.slice(-MAX_PREFIX_CHARS),
    suffix: suffix.slice(0, MAX_SUFFIX_CHARS)
  };
}

class FimAutocompleteProvider {
  constructor(options = {}) {
    this.vscode = options.vscode || null;
    this.log = options.log || (() => {});
    this.getConfigFn = options.getConfig || null;
    this.cache = new LRUCache(options.cacheCapacity || DEFAULT_CACHE_CAPACITY);
    this.pendingTimer = null;
    this.pendingReject = null;
    this.activeController = null;
    this.disposed = false;
    this.config = this._readConfig(options.config);
    this._stats = {
      hits: 0,
      misses: 0,
      aborts: 0,
      timeouts: 0,
      errors: 0,
      lastLatencyMs: 0
    };
  }

  _readConfig(override) {
    if (override) {
      return {
        enabled: override.enabled !== undefined ? Boolean(override.enabled) : true,
        model: override.model || DEFAULT_FIM_MODEL,
        endpoint: override.endpoint || DEFAULT_FIM_ENDPOINT,
        debounceMs: override.debounceMs !== undefined ? Number(override.debounceMs) : DEFAULT_DEBOUNCE_MS,
        latencyBudgetMs: override.latencyBudgetMs !== undefined ? Number(override.latencyBudgetMs) : DEFAULT_LATENCY_BUDGET_MS,
        template: override.template || 'auto'
      };
    }
    if (this.getConfigFn) {
      return this._readConfig(this.getConfigFn());
    }
    if (this.vscode?.workspace?.getConfiguration) {
      const cfg = this.vscode.workspace.getConfiguration('localcode');
      return {
        enabled: cfg.get('fimEnabled', true),
        model: cfg.get('fimModel', DEFAULT_FIM_MODEL),
        endpoint: cfg.get('fimEndpoint', DEFAULT_FIM_ENDPOINT),
        debounceMs: cfg.get('fimDebounceMs', DEFAULT_DEBOUNCE_MS),
        latencyBudgetMs: cfg.get('fimLatencyBudgetMs', DEFAULT_LATENCY_BUDGET_MS),
        template: cfg.get('fimTemplate', 'auto')
      };
    }
    return {
      enabled: true,
      model: DEFAULT_FIM_MODEL,
      endpoint: DEFAULT_FIM_ENDPOINT,
      debounceMs: DEFAULT_DEBOUNCE_MS,
      latencyBudgetMs: DEFAULT_LATENCY_BUDGET_MS,
      template: 'auto'
    };
  }

  reloadConfig(override) {
    this.config = this._readConfig(override);
  }

  clearCache() {
    this.cache.clear();
  }

  getStats() {
    return { ...this._stats, cacheSize: this.cache.size };
  }

  cancelPending() {
    if (this.pendingTimer) {
      clearTimeout(this.pendingTimer);
      this.pendingTimer = null;
    }
    if (this.pendingReject) {
      const resolveEmpty = this.pendingReject;
      this.pendingReject = null;
      this._stats.aborts++;
      resolveEmpty(false);
    }
    if (this.activeController) {
      this._stats.aborts++;
      this.activeController.abort();
      this.activeController = null;
    }
  }

  _makeCompletionItem(text, position) {
    if (this.vscode && typeof this.vscode.InlineCompletionItem === 'function' && typeof this.vscode.Range === 'function') {
      return new this.vscode.InlineCompletionItem(text, new this.vscode.Range(position, position));
    }
    return {
      insertText: text,
      range: { start: position, end: position }
    };
  }

  _waitDebounce(debounceMs, token) {
    if (debounceMs <= 0) {
      return Promise.resolve(!token?.isCancellationRequested);
    }
    return new Promise(resolve => {
      let tokenSub = null;
      const done = ok => {
        if (this.pendingTimer) {
          clearTimeout(this.pendingTimer);
          this.pendingTimer = null;
        }
        if (this.pendingReject === done) {
          this.pendingReject = null;
        }
        if (tokenSub && typeof tokenSub.dispose === 'function') {
          tokenSub.dispose();
        }
        resolve(ok);
      };
      this.pendingReject = done;
      if (token && typeof token.onCancellationRequested === 'function') {
        tokenSub = token.onCancellationRequested(() => {
          this._stats.aborts++;
          done(false);
        });
      }
      this.pendingTimer = setTimeout(() => done(true), debounceMs);
    });
  }

  async provideInlineCompletionItems(document, position, _context, token) {
    if (this.disposed) return [];
    const cfg = this.getConfigFn ? this._readConfig() : this.config;
    if (!cfg.enabled) return [];
    if (this.vscode?.workspace && this.vscode.workspace.isTrusted === false) return [];
    if (!document || document.uri?.scheme !== 'file') return [];
    if (token?.isCancellationRequested) return [];

    // Immediately cancel any previous pending debounce or in-flight request on new keystroke.
    this.cancelPending();

    const { prefix, suffix } = getDocumentPrefixSuffix(document, position);
    if (!prefix.trim() && !suffix.trim()) return [];

    const languageId = document.languageId || 'plaintext';
    const cacheKey = `${cfg.model}\0${cfg.template}\0${languageId}\0${prefix}\0${suffix}`;
    const cached = this.cache.get(cacheKey);
    if (cached !== undefined) {
      this._stats.hits++;
      return cached ? [this._makeCompletionItem(cached, position)] : [];
    }
    this._stats.misses++;

    // Validate loopback strictly BEFORE waiting or sending anything (zero cloud leakage).
    let endpointInfo;
    try {
      endpointInfo = validateLoopbackEndpoint(cfg.endpoint);
    } catch (err) {
      this._stats.errors++;
      this.log(`fim blocked non-loopback endpoint: ${err.message}`);
      return [];
    }

    const proceed = await this._waitDebounce(cfg.debounceMs, token);
    if (!proceed || this.disposed || token?.isCancellationRequested) {
      return [];
    }

    const controller = new AbortController();
    this.activeController = controller;
    let tokenSub = null;
    if (token && typeof token.onCancellationRequested === 'function') {
      tokenSub = token.onCancellationRequested(() => {
        this._stats.aborts++;
        controller.abort();
      });
    }

    const started = Date.now();
    try {
      const fimPrompt = buildFimPrompt(prefix, suffix, cfg.model, cfg.template);
      const spec = resolveFimRequestSpec(endpointInfo, cfg.model, fimPrompt);
      const json = await localLoopbackPostJson(spec.url, spec.body, controller.signal, cfg.latencyBudgetMs);
      this._stats.lastLatencyMs = Date.now() - started;

      if (controller.signal.aborted || token?.isCancellationRequested || this.disposed) {
        return [];
      }

      const rawText = extractCompletionText(json);
      const cleaned = cleanFimCompletion(rawText, suffix);
      this.cache.set(cacheKey, cleaned);
      if (!cleaned) return [];
      return [this._makeCompletionItem(cleaned, position)];
    } catch (err) {
      this._stats.lastLatencyMs = Date.now() - started;
      if (err && (err.code === 'ABORT_ERR' || err.message === 'aborted')) {
        return [];
      }
      if (err && (err.code === 'ETIMEDOUT' || err.message === 'timeout')) {
        this._stats.timeouts++;
        return [];
      }
      this._stats.errors++;
      this.log(`fim request error: ${err.code || err.message}`);
      return [];
    } finally {
      if (this.activeController === controller) {
        this.activeController = null;
      }
      if (tokenSub && typeof tokenSub.dispose === 'function') {
        tokenSub.dispose();
      }
    }
  }

  dispose() {
    this.disposed = true;
    this.cancelPending();
    this.cache.clear();
  }
}

module.exports = {
  DEFAULT_FIM_MODEL,
  DEFAULT_FIM_ENDPOINT,
  DEFAULT_DEBOUNCE_MS,
  DEFAULT_LATENCY_BUDGET_MS,
  DEFAULT_CACHE_CAPACITY,
  LRUCache,
  FimAutocompleteProvider,
  validateLoopbackEndpoint,
  resolveFimTemplate,
  buildFimPrompt,
  cleanFimCompletion,
  localLoopbackPostJson
};
