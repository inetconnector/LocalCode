'use strict';

const http = require('http');
const { serverURL } = require('./client');

const MAX_REVIEW_BYTES = 128 * 1024;
const DEFAULT_AGENTOS_URL = 'http://127.0.0.1:8000';

/**
 * Send a code/diff snippet to the local loopback AgentOS Cross-Vendor Adversarial Review
 * endpoint (`POST /v1/routing/adversarial-review`). Strictly rejects non-loopback hosts.
 *
 * @param {{
 *   agentosUrl?: string,
 *   authorProvider?: string,
 *   codeOrDiff: string,
 *   filePath?: string,
 *   isClinical?: boolean,
 *   lang?: 'de' | 'en',
 *   timeoutMs?: number
 * }} opts
 * @returns {Promise<any>}
 */
function runLoopbackAdversarialReview(opts) {
  let baseUrl;
  try {
    baseUrl = serverURL((opts?.agentosUrl || DEFAULT_AGENTOS_URL).trim());
  } catch (err) {
    return Promise.reject(err);
  }
  const code = String(opts?.codeOrDiff || '');
  if (!code.trim()) {
    return Promise.reject(new Error('adversarialReviewEmpty'));
  }
  if (Buffer.byteLength(code, 'utf8') > MAX_REVIEW_BYTES) {
    return Promise.reject(new Error('adversarialReviewLimit'));
  }

  const lang = String(opts?.lang || 'en').toLowerCase().startsWith('de') ? 'de' : 'en';
  const payload = JSON.stringify({
    task_id: `vscode-adv-${Date.now().toString(36)}`,
    author_provider: String(opts?.authorProvider || 'anthropic').trim() || 'anthropic',
    code_or_diff: code,
    file_path: String(opts?.filePath || ''),
    is_clinical: Boolean(opts?.isClinical),
    contains_pii: Boolean(opts?.isClinical),
    lang
  });

  const target = new URL('/v1/routing/adversarial-review', baseUrl);
  const timeoutMs = Math.max(500, Math.min(Number(opts?.timeoutMs) || 6000, 30000));

  return new Promise((resolve, reject) => {
    const req = http.request(
      target,
      {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Content-Length': Buffer.byteLength(payload, 'utf8'),
          Accept: 'application/json'
        },
        timeout: timeoutMs
      },
      res => {
        const chunks = [];
        res.on('data', chunk => chunks.push(chunk));
        res.on('end', () => {
          const text = Buffer.concat(chunks).toString('utf8');
          let parsed = null;
          try {
            parsed = text ? JSON.parse(text) : null;
          } catch {
            // handled below
          }
          if (res.statusCode && res.statusCode >= 200 && res.statusCode < 300 && parsed) {
            resolve(parsed);
          } else {
            reject(new Error(parsed?.detail || parsed?.error || `HTTP ${res.statusCode || 0}`));
          }
        });
      }
    );
    req.on('timeout', () => {
      req.destroy(new Error('adversarialReviewTimeout'));
    });
    req.on('error', reject);
    req.write(payload);
    req.end();
  });
}

/**
 * Format a compact bilingual DE/EN summary line for VS Code notifications.
 * @param {any} report
 * @param {'de' | 'en'} lang
 */
function formatAdversarialSummary(report, lang) {
  const isDe = String(lang || 'en').toLowerCase().startsWith('de');
  const verdict = String(report?.verdict || 'UNKNOWN');
  const rev = report?.reviewer_provider
    ? `${report.reviewer_provider}/${report.reviewer_model || ''}`.replace(/\/$/, '')
    : '—';
  const total = Number(report?.finding_counts?.total ?? 0);
  if (isDe) {
    return `Adversarial Review [${verdict}] · Prüfer: ${rev} · Befunde: ${total}`;
  }
  return `Adversarial Review [${verdict}] · Reviewer: ${rev} · Findings: ${total}`;
}

module.exports = {
  DEFAULT_AGENTOS_URL,
  MAX_REVIEW_BYTES,
  runLoopbackAdversarialReview,
  formatAdversarialSummary
};
