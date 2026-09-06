'use strict';
const http = require('node:http');

function serverURL(value) {
  const url = new URL(value);
  if (url.protocol !== 'http:' || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname) ||
      url.username || url.password || url.search || url.hash || url.pathname !== '/') throw new Error('invalidEndpoint');
  // Avoid DNS/proxy routing for the localhost alias.
  if (url.hostname === 'localhost') url.hostname = '127.0.0.1';
  return url.origin;
}

class Client {
  constructor(base, log = () => {}) { this.base = serverURL(base); this.log = log; this.requests = new Set(); this.closed = false; }
  request(route, body, timeout = 15000) {
    if (this.closed) return Promise.reject(new Error('disconnected'));
    if (!/^\/api\/[a-z-]+(?:\?thread_id=[\w%-]+)?$/.test(route)) return Promise.reject(new Error('invalidRequest'));
    const data = body === undefined ? undefined : JSON.stringify(body);
    if (data && Buffer.byteLength(data) > 512 * 1024) return Promise.reject(new Error('tooLarge'));
    const method = data === undefined ? 'GET' : 'POST';
    return new Promise((resolve, reject) => {
      const req = http.request(this.base + route, { method, agent: false, headers: { Accept: 'application/json', ...(data ? { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(data) } : {}) } }, res => {
        const chunks = []; let size = 0;
        res.on('data', chunk => { size += chunk.length; if (size > 8 * 1024 * 1024) req.destroy(new Error('tooLarge')); else chunks.push(chunk); });
        res.on('error', reject);
        res.on('end', () => {
          this.log(`${method} ${route.split('?')[0]} ${res.statusCode}`);
          const text = Buffer.concat(chunks).toString('utf8');
          if (res.statusCode < 200 || res.statusCode >= 300) return reject(new Error(`HTTP ${res.statusCode}: ${text.slice(0, 1000)}`));
          try { resolve(JSON.parse(text)); } catch { reject(new Error('invalidResponse')); }
        });
      });
      const timer = setTimeout(() => req.destroy(new Error('timeout')), timeout);
      this.requests.add(req);
      req.on('close', () => { clearTimeout(timer); this.requests.delete(req); });
      req.on('error', reject);
      req.end(data);
    });
  }
  events(onEvent, onError) {
    if (this.closed) return () => {};
    const req = http.get(this.base + '/api/events', { agent: false, headers: { Accept: 'text/event-stream' } }, res => {
      if (res.statusCode !== 200 || !String(res.headers['content-type']).startsWith('text/event-stream')) { req.destroy(new Error('invalidResponse')); return; }
      const parser = new SSEParser(onEvent);
      res.setEncoding('utf8');
      res.on('data', chunk => { try { parser.push(chunk); } catch (err) { req.destroy(err); } });
      res.on('error', () => {});
      res.on('end', () => req.destroy(new Error('disconnected')));
    });
    this.requests.add(req);
    req.setTimeout(45000, () => req.destroy(new Error('timeout')));
    req.on('error', err => { if (!this.closed) onError(err); });
    req.on('close', () => this.requests.delete(req));
    return () => { req.removeAllListeners('error'); req.on('error', () => {}); req.destroy(); };
  }
  dispose() { this.closed = true; for (const req of this.requests) req.destroy(new Error('disconnected')); this.requests.clear(); }
}

class SSEParser {
  constructor(onEvent) { this.onEvent = onEvent; this.buffer = ''; this.lines = []; this.size = 0; }
  push(chunk) {
    this.buffer += chunk;
    if (this.buffer.length > 1024 * 1024) throw new Error('tooLarge');
    let index;
    while ((index = this.buffer.indexOf('\n')) >= 0) {
      const line = this.buffer.slice(0, index).replace(/\r$/, ''); this.buffer = this.buffer.slice(index + 1);
      if (line === '') {
        if (this.lines.length) { const event = JSON.parse(this.lines.join('\n')); this.onEvent(event); }
        this.lines = []; this.size = 0;
      } else if (line.startsWith('data:')) {
        const data = line.slice(5).replace(/^ /, ''); this.size += data.length;
        if (this.size > 1024 * 1024) throw new Error('tooLarge');
        this.lines.push(data);
      }
    }
  }
}
module.exports = { Client, SSEParser, serverURL };
