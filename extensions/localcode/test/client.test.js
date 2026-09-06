'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { Client, SSEParser, serverURL } = require('../src/client');
test('endpoint rejects remote hosts, credentials, paths and protocol changes', () => {
  for (const value of ['https://127.0.0.1', 'http://example.com', 'http://127.0.0.2', 'http://localhost.evil', 'http://user:secret@localhost', 'http://localhost/api', 'http://localhost?x=1', 'http://localhost#x', 'file:///etc/passwd']) assert.throws(() => serverURL(value));
  assert.equal(serverURL('http://localhost:32145'), 'http://127.0.0.1:32145');
  assert.equal(serverURL('http://[::1]:32145'), 'http://[::1]:32145');
});
test('SSE handles CRLF/chunks/comments/multiline data and bounded malformed frames', () => {
  const events = [], parser = new SSEParser(e => events.push(e));
  for (const chunk of [': ping\r\n\r\ndata: {"id":', '"1",\r\n', 'data: "message":"Grüße"}\r\n\r', '\n']) parser.push(chunk);
  assert.deepEqual(events, [{ id: '1', message: 'Grüße' }]);
  assert.throws(() => parser.push('data: invalid\n\n'));
  assert.throws(() => new SSEParser(() => {}).push('x'.repeat(1024 * 1024 + 1)), /tooLarge/);
});
test('HTTP uses explicit JSON bodies, refuses redirects, times out and disposes', async t => {
  const received = [];
  const server = http.createServer((req, res) => {
    if (req.url === '/api/hang') return;
    if (req.url === '/api/redirect') { res.writeHead(302, { Location: 'http://example.com' }); res.end(); return; }
    if (req.url === '/api/bad') { res.end('not JSON'); return; }
    if (req.url === '/api/large') { res.end('x'.repeat(8 * 1024 * 1024 + 1)); return; }
    let body = ''; req.on('data', chunk => { body += chunk; }); req.on('end', () => { received.push([req.method, JSON.parse(body)]); res.setHeader('content-type', 'application/json'); res.end('{"ok":true}'); });
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const client = new Client('http://127.0.0.1:' + server.address().port);
  t.after(() => { client.dispose(); server.closeAllConnections(); server.close(); });
  assert.deepEqual(await client.request('/api/chat', { message: '`not a shell`' }), { ok: true });
  assert.deepEqual(received, [['POST', { message: '`not a shell`' }]]);
  await assert.rejects(client.request('/api/redirect'), /302/);
  await assert.rejects(client.request('/api/bad'), /invalidResponse/);
  await assert.rejects(client.request('/api/large'), /tooLarge/);
  await assert.rejects(client.request('/api/hang', undefined, 30), /timeout/);
  await assert.rejects(client.request('//example.com'), /invalidRequest/);
  await assert.rejects(client.request('/api/chat', { message: 'x'.repeat(512 * 1024) }), /tooLarge/);
  const pending = client.request('/api/hang'); client.dispose(); await assert.rejects(pending, /disconnected/);
});
