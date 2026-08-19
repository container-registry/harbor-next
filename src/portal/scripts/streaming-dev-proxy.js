const http = require('node:http');
const https = require('node:https');

const HARBOR_PATH_PREFIXES = [
  '/api',
  '/service',
  '/v2',
  '/npm',
  '/maven',
  '/pypi',
  '/cargo',
  '/go',
  '/go-sumdb',
  '/homebrew',
  '/chartrepo',
  '/c',
  '/LICENSE',
];

function isHarborPath(requestUrl) {
  const pathname = new URL(requestUrl, 'http://localhost').pathname;
  return HARBOR_PATH_PREFIXES.some(
    prefix => pathname === prefix || pathname.startsWith(`${prefix}/`)
  );
}

function transportFor(target) {
  return target.protocol === 'https:' ? https : http;
}

function proxyHttpRequest(request, response, target) {
  const upstream = transportFor(target).request(
    {
      protocol: target.protocol,
      hostname: target.hostname,
      port: target.port,
      method: request.method,
      path: request.url,
      headers: request.headers,
    },
    upstreamResponse => {
      upstreamResponse.on('error', error => {
        if (!response.headersSent) {
          response.writeHead(502, { 'Content-Type': 'text/plain' });
        }
        response.end(`Development proxy error: ${error.message}`);
      });
      response.writeHead(
        upstreamResponse.statusCode,
        upstreamResponse.statusMessage,
        upstreamResponse.headers
      );
      upstreamResponse.pipe(response);
    }
  );

  upstream.on('error', error => {
    if (!response.headersSent) {
      response.writeHead(502, { 'Content-Type': 'text/plain' });
    }
    response.end(`Development proxy error: ${error.message}`);
  });
  request.on('error', () => upstream.destroy());
  request.on('aborted', () => upstream.destroy());
  request.pipe(upstream);
}

function writeRawResponse(socket, upstreamResponse, decodedBody = false) {
  socket.write(
    `HTTP/1.1 ${upstreamResponse.statusCode} ${upstreamResponse.statusMessage}\r\n`
  );
  for (let index = 0; index < upstreamResponse.rawHeaders.length; index += 2) {
    const name = upstreamResponse.rawHeaders[index];
    if (
      decodedBody &&
      ['connection', 'transfer-encoding'].includes(name.toLowerCase())
    ) {
      continue;
    }
    socket.write(
      `${name}: ${upstreamResponse.rawHeaders[index + 1]}\r\n`
    );
  }
  if (decodedBody) {
    socket.write('Connection: close\r\n');
  }
  socket.write('\r\n');
}

function proxyH2CRequest(request, socket, head, target) {
  const headers = { ...request.headers };
  delete headers.connection;
  delete headers.upgrade;
  delete headers['http2-settings'];
  headers.connection = 'close';

  const upstream = transportFor(target).request(
    {
      protocol: target.protocol,
      hostname: target.hostname,
      port: target.port,
      method: request.method,
      path: request.url,
      headers,
    },
    upstreamResponse => {
      writeRawResponse(socket, upstreamResponse, true);
      upstreamResponse.on('error', () => socket.destroy());
      upstreamResponse.pipe(socket);
    }
  );

  socket.on('error', () => upstream.destroy());
  upstream.on('error', () => socket.destroy());

  const contentLength = Number.parseInt(headers['content-length'] || '0', 10);
  if (!Number.isFinite(contentLength) || contentLength <= 0) {
    upstream.end();
    return;
  }

  let remaining = contentLength;
  const forward = chunk => {
    const body = chunk.subarray(0, remaining);
    remaining -= body.length;
    if (body.length > 0) {
      upstream.write(body);
    }
    if (remaining === 0) {
      socket.off('data', forward);
      upstream.end();
    }
  };
  forward(head);
  if (remaining > 0) {
    socket.on('data', forward);
    socket.on('end', () => upstream.destroy());
    socket.resume();
  }
}

function proxyUpgrade(request, socket, head, target) {
  const upstream = transportFor(target).request({
    protocol: target.protocol,
    hostname: target.hostname,
    port: target.port,
    method: request.method,
    path: request.url,
    headers: request.headers,
  });

  socket.on('error', () => upstream.destroy());

  upstream.on('upgrade', (upstreamResponse, upstreamSocket, upstreamHead) => {
    upstreamSocket.on('error', () => socket.destroy());
    writeRawResponse(socket, upstreamResponse, true);
    if (upstreamHead.length > 0) {
      socket.write(upstreamHead);
    }
    if (head.length > 0) {
      upstreamSocket.write(head);
    }
    upstreamSocket.pipe(socket).pipe(upstreamSocket);
  });

  upstream.on('response', upstreamResponse => {
    writeRawResponse(socket, upstreamResponse);
    upstreamResponse.on('error', () => socket.destroy());
    upstreamResponse.pipe(socket);
  });
  upstream.on('error', () => socket.destroy());
  upstream.end();
}

function createStreamingDevProxy({ harborTarget, angularTarget }) {
  const harbor = new URL(harborTarget);
  const angular = new URL(angularTarget);
  const server = http.createServer((request, response) => {
    proxyHttpRequest(
      request,
      response,
      isHarborPath(request.url) ? harbor : angular
    );
  });
  server.on('upgrade', (request, socket, head) => {
    if (
      isHarborPath(request.url) &&
      request.headers.upgrade?.toLowerCase() === 'h2c'
    ) {
      proxyH2CRequest(request, socket, head, harbor);
      return;
    }
    proxyUpgrade(
      request,
      socket,
      head,
      isHarborPath(request.url) ? harbor : angular
    );
  });
  return server;
}

module.exports = {
  createStreamingDevProxy,
  isHarborPath,
};
