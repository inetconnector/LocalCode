'use strict';

const crypto = require('node:crypto');
const path = require('node:path');
const { contained } = require('./safety');
const { t } = require('./i18n');
const {
  DEFAULT_FIM_ENDPOINT,
  DEFAULT_FIM_MODEL,
  validateLoopbackEndpoint,
  localLoopbackPostJson
} = require('./fim_autocomplete');

const INLINE_EDIT_SCHEME = 'localcode-inline';
const MAX_INLINE_CONTEXT_BYTES = 64 * 1024; // 64 KiB
const DEFAULT_INLINE_TIMEOUT_MS = 45000;

function stripCodeFences(raw) {
  if (typeof raw !== 'string') return '';
  let text = raw.replace(/\r\n/g, '\n');
  const trimmed = text.trim();
  const fenceMatch = trimmed.match(/^```[^\n]*\n([\s\S]*?)\n?```$/);
  if (fenceMatch) {
    return fenceMatch[1];
  }
  return text;
}

function clipUtf8FromEnd(str, maxBytes) {
  if (maxBytes <= 0 || !str) return '';
  if (Buffer.byteLength(str, 'utf8') <= maxBytes) return str;
  // Slice from end while ensuring we never split a multi-byte UTF-8 sequence.
  let lo = 0;
  let hi = str.length;
  while (lo < hi) {
    const mid = Math.floor((lo + hi) / 2);
    if (Buffer.byteLength(str.slice(mid), 'utf8') > maxBytes) {
      lo = mid + 1;
    } else {
      hi = mid;
    }
  }
  return str.slice(lo);
}

function clipUtf8FromStart(str, maxBytes) {
  if (maxBytes <= 0 || !str) return '';
  if (Buffer.byteLength(str, 'utf8') <= maxBytes) return str;
  let lo = 0;
  let hi = str.length;
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (Buffer.byteLength(str.slice(0, mid), 'utf8') <= maxBytes) {
      lo = mid;
    } else {
      hi = mid - 1;
    }
  }
  return str.slice(0, lo);
}

function extractInlineContext(document, selection, maxContextBytes = MAX_INLINE_CONTEXT_BYTES) {
  const fullText = document.getText();
  let targetRange = selection;
  if (!selection || selection.isEmpty) {
    const activeLine = selection?.active?.line ?? 0;
    if (typeof document.lineAt === 'function') {
      targetRange = document.lineAt(activeLine).range;
    } else {
      targetRange = selection;
    }
  }

  const startOffset = document.offsetAt(targetRange.start);
  const endOffset = document.offsetAt(targetRange.end);
  const selectedText = fullText.slice(startOffset, endOffset);
  const selectedBytes = Buffer.byteLength(selectedText, 'utf8');
  if (selectedBytes > maxContextBytes) {
    throw new Error('inlineEditContextLimit');
  }

  const remaining = maxContextBytes - selectedBytes;
  const prefixBudget = Math.floor(remaining * 0.6);
  const suffixBudget = remaining - prefixBudget;

  const rawPrefix = fullText.slice(0, startOffset);
  const rawSuffix = fullText.slice(endOffset);

  const prefix = clipUtf8FromEnd(rawPrefix, prefixBudget);
  const suffix = clipUtf8FromStart(rawSuffix, suffixBudget);

  return {
    targetRange,
    startOffset,
    endOffset,
    fullText,
    selectedText,
    prefix,
    suffix,
    totalBytes: Buffer.byteLength(prefix, 'utf8') + selectedBytes + Buffer.byteLength(suffix, 'utf8')
  };
}

function buildProposedDocumentText(fullText, startOffset, endOffset, replacement) {
  return fullText.slice(0, startOffset) + replacement + fullText.slice(endOffset);
}

function buildInlineEditPrompt({ fileName, languageId, instruction, selectedText, prefix, suffix }) {
  return [
    'You are an expert code editor. Apply the user instruction to the selected code.',
    'Return ONLY the replacement code for the selected region. Do not include explanations or markdown fences.',
    `File: ${fileName} (${languageId || 'plaintext'})`,
    `Instruction: ${instruction}`,
    '',
    '<<<PREFIX_CONTEXT>>>',
    prefix,
    '<<<SELECTED_CODE>>>',
    selectedText,
    '<<<SUFFIX_CONTEXT>>>',
    suffix,
    '<<<REPLACEMENT_CODE>>>'
  ].join('\n');
}

function resolveInlineEditRequest(rawEndpoint, model, params) {
  let info;
  try {
    info = validateLoopbackEndpoint(rawEndpoint);
  } catch {
    throw new Error('inlineEditInvalidEndpoint');
  }
  const lowerPath = info.basePath.toLowerCase();

  if (lowerPath.endsWith('/api/inline-edit')) {
    return {
      url: info.origin + info.basePath,
      body: {
        file: params.fileName,
        language: params.languageId,
        instruction: params.instruction,
        selection: params.selectedText,
        prefix: params.prefix,
        suffix: params.suffix,
        model
      }
    };
  }

  const prompt = buildInlineEditPrompt(params);

  if (lowerPath.endsWith('/v1/chat/completions')) {
    return {
      url: info.origin + info.basePath,
      body: {
        model,
        temperature: 0.1,
        stream: false,
        messages: [
          {
            role: 'system',
            content: 'You are a precise inline code editor. Return ONLY the replacement code for the selected region without explanation.'
          },
          { role: 'user', content: prompt }
        ]
      }
    };
  }

  if (lowerPath.endsWith('/completion') || (!lowerPath && info.port === '8080')) {
    const targetPath = lowerPath ? info.basePath : '/completion';
    return {
      url: info.origin + targetPath,
      body: {
        prompt,
        n_predict: 1024,
        temperature: 0.1,
        stream: false
      }
    };
  }

  const targetPath = lowerPath.endsWith('/api/generate') ? info.basePath : `${info.basePath}/api/generate`;
  return {
    url: info.origin + targetPath,
    body: {
      model,
      prompt,
      stream: false,
      options: {
        temperature: 0.1,
        num_predict: 1024
      }
    }
  };
}

function extractInlineEditResult(json) {
  if (!json || typeof json !== 'object') return '';
  if (typeof json.replacement === 'string') return stripCodeFences(json.replacement);
  if (typeof json.code === 'string') return stripCodeFences(json.code);
  if (typeof json.response === 'string') return stripCodeFences(json.response);
  if (typeof json.content === 'string') return stripCodeFences(json.content);
  if (Array.isArray(json.choices) && json.choices[0]) {
    if (json.choices[0].message && typeof json.choices[0].message.content === 'string') {
      return stripCodeFences(json.choices[0].message.content);
    }
    if (typeof json.choices[0].text === 'string') {
      return stripCodeFences(json.choices[0].text);
    }
  }
  return '';
}

class InlineEditContentProvider {
  constructor(vscodeApi) {
    this.contents = new Map();
    this._emitter = vscodeApi && typeof vscodeApi.EventEmitter === 'function' ? new vscodeApi.EventEmitter() : null;
    this.onDidChange = this._emitter ? this._emitter.event : undefined;
  }

  provideTextDocumentContent(uri) {
    return this.contents.get(uri.toString()) || '';
  }

  set(uri, content) {
    this.contents.set(uri.toString(), content);
    if (this._emitter) {
      this._emitter.fire(uri);
    }
  }

  delete(uri) {
    if (uri) {
      this.contents.delete(uri.toString());
    }
  }

  clear() {
    this.contents.clear();
  }

  dispose() {
    this.clear();
    if (this._emitter && typeof this._emitter.dispose === 'function') {
      this._emitter.dispose();
    }
  }
}

class InlineEditController {
  constructor(options = {}) {
    this.vscode = options.vscode;
    this.getLanguage = options.getLanguage || (() => 'en');
    this.getWorkspaceRoot = options.getWorkspaceRoot || (() => '');
    this.getConfig = options.getConfig || null;
    this.log = options.log || (() => {});
    this.provider = new InlineEditContentProvider(this.vscode);
    this.pendingEdit = null;
    this.activeController = null;
  }

  _t(key) {
    return t(this.getLanguage(), key);
  }

  _resolveSettings() {
    if (this.getConfig) {
      const cfg = this.getConfig() || {};
      return {
        endpoint: cfg.endpoint || DEFAULT_FIM_ENDPOINT,
        model: cfg.model || DEFAULT_FIM_MODEL,
        timeoutMs: cfg.timeoutMs || DEFAULT_INLINE_TIMEOUT_MS
      };
    }
    if (this.vscode?.workspace?.getConfiguration) {
      const cfg = this.vscode.workspace.getConfiguration('localcode');
      const inlineEndpoint = cfg.get('inlineEditEndpoint', '');
      const fimEndpoint = cfg.get('fimEndpoint', DEFAULT_FIM_ENDPOINT);
      const inlineModel = cfg.get('inlineEditModel', '');
      const fimModel = cfg.get('fimModel', DEFAULT_FIM_MODEL);
      return {
        endpoint: inlineEndpoint || fimEndpoint || DEFAULT_FIM_ENDPOINT,
        model: inlineModel || fimModel || DEFAULT_FIM_MODEL,
        timeoutMs: DEFAULT_INLINE_TIMEOUT_MS
      };
    }
    return {
      endpoint: DEFAULT_FIM_ENDPOINT,
      model: DEFAULT_FIM_MODEL,
      timeoutMs: DEFAULT_INLINE_TIMEOUT_MS
    };
  }

  async _resolveWorkspaceForFile(filePath) {
    const explicitRoot = this.getWorkspaceRoot();
    if (explicitRoot) {
      try {
        await contained(explicitRoot, filePath);
        return explicitRoot;
      } catch {
        throw new Error('inlineEditOutsideWorkspace');
      }
    }
    const folders = (this.vscode?.workspace?.workspaceFolders || []).filter(f => f.uri?.scheme === 'file');
    for (const folder of folders) {
      try {
        await contained(folder.uri.fsPath, filePath);
        return folder.uri.fsPath;
      } catch {
        // Try next workspace folder
      }
    }
    throw new Error('inlineEditOutsideWorkspace');
  }

  async run(editorArg, instructionOverride) {
    const vscode = this.vscode;
    if (vscode?.workspace && vscode.workspace.isTrusted === false) {
      throw new Error('inlineEditTrustRequired');
    }

    const editor = editorArg || vscode?.window?.activeTextEditor;
    if (!editor || !editor.document || editor.document.uri?.scheme !== 'file') {
      throw new Error('inlineEditNoEditor');
    }

    const filePath = editor.document.uri.fsPath;
    const workspaceRoot = await this._resolveWorkspaceForFile(filePath);
    const contextData = extractInlineContext(editor.document, editor.selection);

    let instruction = instructionOverride;
    if (instruction === undefined) {
      instruction = await vscode.window.showInputBox({
        prompt: this._t('inlineEditPrompt'),
        placeHolder: this._t('inlineEditPlaceholder'),
        ignoreFocusOut: true
      });
    }
    if (!instruction || !String(instruction).trim()) {
      return { status: 'cancelled' };
    }

    const settings = this._resolveSettings();
    const relPath = path.relative(workspaceRoot, filePath).replace(/\\/g, '/');
    const reqSpec = resolveInlineEditRequest(settings.endpoint, settings.model, {
      fileName: relPath,
      languageId: editor.document.languageId || 'plaintext',
      instruction: String(instruction).trim(),
      selectedText: contextData.selectedText,
      prefix: contextData.prefix,
      suffix: contextData.suffix
    });

    if (this.activeController) {
      this.activeController.abort();
    }
    const controller = new AbortController();
    this.activeController = controller;

    const executeRequest = async () => {
      try {
        return await localLoopbackPostJson(reqSpec.url, reqSpec.body, controller.signal, settings.timeoutMs);
      } catch (err) {
        if (err && (err.code === 'ETIMEDOUT' || err.message === 'timeout')) {
          throw new Error('inlineEditTimeout');
        }
        throw err;
      } finally {
        if (this.activeController === controller) {
          this.activeController = null;
        }
      }
    };

    let responseJson;
    if (typeof vscode?.window?.withProgress === 'function') {
      responseJson = await vscode.window.withProgress(
        {
          location: vscode.ProgressLocation?.Notification ?? 15,
          title: this._t('inlineEditProgress'),
          cancellable: true
        },
        (_progress, cancelToken) => {
          if (cancelToken && typeof cancelToken.onCancellationRequested === 'function') {
            cancelToken.onCancellationRequested(() => controller.abort());
          }
          return executeRequest();
        }
      );
    } else {
      responseJson = await executeRequest();
    }

    if (controller.signal.aborted) {
      return { status: 'cancelled' };
    }

    const replacement = extractInlineEditResult(responseJson);
    if (!replacement) {
      throw new Error('inlineEditEmptyResponse');
    }

    const proposedText = buildProposedDocumentText(
      contextData.fullText,
      contextData.startOffset,
      contextData.endOffset,
      replacement
    );

    // Discard previous pending virtual diff if any.
    if (this.pendingEdit) {
      this.provider.delete(this.pendingEdit.proposedUri);
    }

    const id = crypto.randomBytes(6).toString('hex');
    const baseName = encodeURIComponent(path.basename(filePath));
    const proposedUri = vscode.Uri.parse(`${INLINE_EDIT_SCHEME}:/${id}/${baseName}`);
    this.provider.set(proposedUri, proposedText);

    this.pendingEdit = {
      documentUri: editor.document.uri,
      targetRange: contextData.targetRange,
      replacement,
      proposedUri,
      originalText: contextData.fullText,
      proposedText
    };

    const diffTitle = `${this._t('inlineEditDiffTitle')}: ${relPath}`;
    await vscode.commands.executeCommand('vscode.diff', editor.document.uri, proposedUri, diffTitle);

    const acceptLabel = this._t('inlineEditAccept');
    const rejectLabel = this._t('inlineEditReject');
    const decision = await vscode.window.showInformationMessage(
      this._t('inlineEditReviewPrompt'),
      acceptLabel,
      rejectLabel
    );

    if (decision === acceptLabel) {
      return this.accept();
    }
    if (decision === rejectLabel) {
      return this.reject();
    }
    return { status: 'preview', proposedUri };
  }

  async accept() {
    const vscode = this.vscode;
    if (!this.pendingEdit) {
      if (vscode?.window?.showInformationMessage) {
        vscode.window.showInformationMessage(this._t('inlineEditNoPending'));
      }
      return { status: 'none' };
    }
    const pending = this.pendingEdit;
    const edit = new vscode.WorkspaceEdit();
    edit.replace(pending.documentUri, pending.targetRange, pending.replacement);
    const applied = await vscode.workspace.applyEdit(edit);
    if (!applied) {
      throw new Error('inlineEditApplyFailed');
    }
    this.provider.delete(pending.proposedUri);
    this.pendingEdit = null;
    if (vscode?.window?.showInformationMessage) {
      vscode.window.showInformationMessage(this._t('inlineEditApplied'));
    }
    return { status: 'applied', replacement: pending.replacement };
  }

  async reject() {
    const vscode = this.vscode;
    if (!this.pendingEdit) {
      if (vscode?.window?.showInformationMessage) {
        vscode.window.showInformationMessage(this._t('inlineEditNoPending'));
      }
      return { status: 'none' };
    }
    const pending = this.pendingEdit;
    this.provider.delete(pending.proposedUri);
    this.pendingEdit = null;
    if (vscode?.window?.showInformationMessage) {
      vscode.window.showInformationMessage(this._t('inlineEditRejected'));
    }
    return { status: 'rejected' };
  }

  dispose() {
    if (this.activeController) {
      this.activeController.abort();
      this.activeController = null;
    }
    this.pendingEdit = null;
    this.provider.dispose();
  }
}

module.exports = {
  INLINE_EDIT_SCHEME,
  MAX_INLINE_CONTEXT_BYTES,
  stripCodeFences,
  extractInlineContext,
  buildProposedDocumentText,
  buildInlineEditPrompt,
  resolveInlineEditRequest,
  extractInlineEditResult,
  InlineEditContentProvider,
  InlineEditController
};
