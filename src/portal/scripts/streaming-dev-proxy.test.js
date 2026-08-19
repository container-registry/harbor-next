const assert = require('node:assert/strict');
const http = require('node:http');
const { after, before, test } = require('node:test');

const { createStreamingDevProxy, isHarborPath } = require('./streaming-dev-proxy');

let angularServer;
let harborServer;
let proxyServer;
let proxyPort;

function listen(server) {
  return new Promise(resolve => {
    server.listen(0, '127.0.0.1', () => resolve(server.address().port));
  });
}

function close(server) {
  return new Promise((resolve, reject) => {
    server.close(error => (error ? reject(error) : resolve()));
  });
}

before(async () => {
  angularServer = http.createServer((_request, response) => {
    response.end('angular');
  });
  angularServer.on('upgrade', (_request, socket) => {
    socket.end(
      'HTTP/1.1 101 Switching Protocols\r\n' +
        'Connection: Upgrade\r\n' +
        'Upgrade: h2c\r\n' +
        'X-Upgrade-Upstream: angular\r\n\r\n'
    );
  });
  harborServer = http.createServer((request, response) => {
    if (request.url === '/cargo/library/config.json') {
      response.writeHead(200, { 'Content-Type': 'application/json' });
      response.end('{"auth-required":false}');
      return;
    }
    const chunks = [];
    request.on('data', chunk => chunks.push(chunk));
    request.on('end', () => {
      response.writeHead(202, {
        Location: `http://${request.headers.host}${request.url}`,
        'X-Received-Body': Buffer.concat(chunks).toString(),
      });
      response.end();
    });
  });
  harborServer.on('upgrade', (request, socket) => {
    if (request.url === '/cargo/library/config.json') {
      const body = '{"auth-required":false}';
      socket.end(
        'HTTP/1.1 200 OK\r\n' +
          'Content-Type: application/json\r\n' +
          `Content-Length: ${Buffer.byteLength(body)}\r\n` +
          'Connection: close\r\n\r\n' +
          body
      );
      return;
    }
    socket.end(
      'HTTP/1.1 101 Switching Protocols\r\n' +
        'Connection: Upgrade\r\n' +
        'Upgrade: h2c\r\n' +
        'X-Upgrade-Upstream: harbor\r\n\r\n'
    );
  });

  const angularPort = await listen(angularServer);
  const harborPort = await listen(harborServer);
  proxyServer = createStreamingDevProxy({
    angularTarget: `http://127.0.0.1:${angularPort}`,
    harborTarget: `http://127.0.0.1:${harborPort}`,
  });
  proxyPort = await listen(proxyServer);
});

after(async () => {
  await Promise.all([
    close(proxyServer),
    close(harborServer),
    close(angularServer),
  ]);
});

test('routes Harbor and package API paths to Core', () => {
  for (const path of ['/v2/', '/npm/library/', '/go/library/']) {
    assert.equal(isHarborPath(path), true);
  }
  assert.equal(isHarborPath('/projects'), false);
});

test('streams chunked registry PATCH uploads and preserves Host', async () => {
  const result = await new Promise((resolve, reject) => {
    const request = http.request(
      {
        host: '127.0.0.1',
        port: proxyPort,
        path: '/v2/library/test/blobs/uploads/uuid',
        method: 'PATCH',
        headers: {
          Host: 'localhost:4700',
          'Content-Type': 'application/octet-stream',
          'Transfer-Encoding': 'chunked',
        },
      },
      response => {
        resolve({ statusCode: response.statusCode, headers: response.headers });
        response.resume();
      }
    );
    request.on('error', reject);
    request.write('chunk-one');
    request.end('-chunk-two');
  });

  assert.equal(result.statusCode, 202);
  assert.equal(result.headers['x-received-body'], 'chunk-one-chunk-two');
  assert.equal(
    result.headers.location,
    'http://localhost:4700/v2/library/test/blobs/uploads/uuid'
  );
});

test('streams Cargo h2c request bodies to Core over HTTP/1.1', async () => {
  const body = 'cargo-publish-body';
  const result = await new Promise((resolve, reject) => {
    const request = http.request({
      host: '127.0.0.1',
      port: proxyPort,
      path: '/cargo/library/api/v1/crates/new',
      method: 'PUT',
      headers: {
        Connection: 'Upgrade, HTTP2-Settings',
        Upgrade: 'h2c',
        'HTTP2-Settings': 'AAMAAABkAAQAAQAAAAIAAAAA',
        'Content-Length': Buffer.byteLength(body),
      },
    });
    request.on('response', response => {
      resolve({ statusCode: response.statusCode, headers: response.headers });
      response.resume();
    });
    request.on('error', reject);
    request.end(body);
  });

  assert.equal(result.statusCode, 202);
  assert.equal(result.headers['x-received-body'], body);
});

test('streams HTTP/1.1 fallback when Core declines h2c upgrade', async () => {
  const result = await new Promise((resolve, reject) => {
    const request = http.request({
      host: '127.0.0.1',
      port: proxyPort,
      path: '/cargo/library/config.json',
      headers: {
        Connection: 'Upgrade, HTTP2-Settings',
        Upgrade: 'h2c',
        'HTTP2-Settings': 'AAMAAABkAAQAAQAAAAIAAAAA',
      },
    });
    request.on('response', response => {
      const chunks = [];
      response.on('data', chunk => chunks.push(chunk));
      response.on('end', () => {
        resolve({
          statusCode: response.statusCode,
          contentType: response.headers['content-type'],
          body: Buffer.concat(chunks).toString(),
        });
      });
    });
    request.on('error', reject);
    request.end();
  });

  assert.deepEqual(result, {
    statusCode: 200,
    contentType: 'application/json',
    body: '{"auth-required":false}',
  });
});

test('routes UI requests to Angular', async () => {
  const body = await new Promise((resolve, reject) => {
    http
      .get(`http://127.0.0.1:${proxyPort}/projects`, response => {
        const chunks = [];
        response.on('data', chunk => chunks.push(chunk));
        response.on('end', () => resolve(Buffer.concat(chunks).toString()));
      })
      .on('error', reject);
  });

  assert.equal(body, 'angular');
});

test('survives a reset WebSocket client', async () => {
  await new Promise((resolve, reject) => {
    const request = http.request({
      host: '127.0.0.1',
      port: proxyPort,
      path: '/ws',
      headers: {
        Connection: 'Upgrade',
        Upgrade: 'websocket',
      },
    });
    request.on('error', error => {
      if (error.code === 'ECONNRESET') {
        resolve();
        return;
      }
      reject(error);
    });
    request.on('socket', socket => {
      socket.on('connect', () => socket.destroy());
    });
    request.end();
    setTimeout(resolve, 100);
  });

  const body = await new Promise((resolve, reject) => {
    http
      .get(`http://127.0.0.1:${proxyPort}/projects`, response => {
        const chunks = [];
        response.on('data', chunk => chunks.push(chunk));
        response.on('end', () => resolve(Buffer.concat(chunks).toString()));
      })
      .on('error', reject);
  });
  assert.equal(body, 'angular');
});
