const { rmSync, mkdirSync } = require('node:fs');
const { spawn } = require('node:child_process');

const { createStreamingDevProxy } = require('./streaming-dev-proxy');

const openApiOutputDir = '/app/src/openapi-ui';
const angularCli = '/app/node_modules/@angular/cli/bin/ng';
const publicPort = Number(process.env.HARBOR_PORTAL_PORT || 4200);
const angularPort = Number(process.env.HARBOR_ANGULAR_PORT || 4201);
const harborTarget = process.env.HARBOR_PROXY_TARGET || 'http://localhost:8080';

function cleanOpenApiOutput() {
  rmSync(`${openApiOutputDir}/devcenter-api-2.0`, {
    force: true,
    recursive: true,
  });
  rmSync(`${openApiOutputDir}/swagger-ui.bundle.js`, { force: true });
  mkdirSync(openApiOutputDir, { recursive: true });
}

function buildOpenApiUi() {
  console.log('Building Swagger UI assets in background...');
  const child = spawn('bun', ['run', 'build'], {
    cwd: '/swagger-ui',
    env: {
      ...process.env,
      OPENAPI_UI_HTML_FILENAME: 'devcenter-api-2.0/index.html',
      OPENAPI_UI_OUTPUT_DIR: openApiOutputDir,
      OPENAPI_UI_SKIP_MINIFY: 'true',
    },
    stdio: 'inherit',
  });

  child.on('exit', code => {
    if (code !== 0) {
      console.error(`Swagger UI build failed with exit code ${code}`);
    } else {
      console.log('Swagger UI build complete');
    }
  });
}

function startPortal() {
  const child = spawn(
    'node',
    [
      angularCli,
      'serve',
      '--host',
      '127.0.0.1',
      '--port',
      String(angularPort),
      '--hmr',
    ],
    {
      env: process.env,
      stdio: 'inherit',
    }
  );

  const proxy = createStreamingDevProxy({
    angularTarget: `http://127.0.0.1:${angularPort}`,
    harborTarget,
  });
  proxy.listen(publicPort, '0.0.0.0', () => {
    console.log(
      `Portal streaming proxy listening on ${publicPort}; Angular HMR on ${angularPort}`
    );
  });

  const forwardSignal = signal => {
    proxy.close();
    if (!child.killed) {
      child.kill(signal);
    }
  };

  process.on('SIGINT', () => forwardSignal('SIGINT'));
  process.on('SIGTERM', () => forwardSignal('SIGTERM'));

  child.on('exit', code => {
    process.exit(code ?? 0);
  });
}

cleanOpenApiOutput();
buildOpenApiUi();
startPortal();
