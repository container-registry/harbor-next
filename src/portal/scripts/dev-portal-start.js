const { rmSync, mkdirSync } = require('node:fs');
const { spawn, spawnSync } = require('node:child_process');

const openApiOutputDir = '/app/src/openapi-ui';
const angularCli = '/app/node_modules/@angular/cli/bin/ng';

function cleanOpenApiOutput() {
  rmSync(`${openApiOutputDir}/devcenter-api-2.0`, {
    force: true,
    recursive: true,
  });
  rmSync(`${openApiOutputDir}/swagger-ui.bundle.js`, { force: true });
  mkdirSync(openApiOutputDir, { recursive: true });
}

function buildOpenApiUi() {
  console.log('OPENAPI_UI=true: building Swagger UI assets into portal');
  const build = spawnSync('bun', ['run', 'build'], {
    cwd: '/swagger-ui',
    env: {
      ...process.env,
      OPENAPI_UI_HTML_FILENAME: 'devcenter-api-2.0/index.html',
      OPENAPI_UI_OUTPUT_DIR: openApiOutputDir,
      OPENAPI_UI_SKIP_MINIFY: 'true',
    },
    stdio: 'inherit',
  });

  if (build.status !== 0) {
    process.exit(build.status ?? 1);
  }
}

function startPortal() {
  const child = spawn(
    'bun',
    [angularCli, 'serve', '--host', '0.0.0.0', '--hmr'],
    {
      env: process.env,
      stdio: 'inherit',
    }
  );

  const forwardSignal = signal => {
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

if (process.env.OPENAPI_UI === 'true') {
  buildOpenApiUi();
}

startPortal();
