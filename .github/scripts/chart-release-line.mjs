import { execFileSync, spawnSync } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

// Maintains the Helm chart release line of a release-X.Y branch.
//
// seed:            cut release-X.Y at the vX.Y.0 release commit and seed its chart line
//                  (TAG_NAME, VERSION, MAJOR, MINOR, GITHUB_REPOSITORY, GH_TOKEN).
// drop-release-as: remove the one-shot chart release-as once its version is tagged
//                  (CHART_VERSION, GITHUB_REF_NAME, GH_TOKEN).

const CONFIG = 'release-please-config-chart.json';
const MANIFEST = '.release-please-manifest-chart.json';
const CHART = 'deploy/chart/Chart.yaml';
const BOT = ['-c', 'user.name=github-actions[bot]', '-c', 'user.email=41898282+github-actions[bot]@users.noreply.github.com'];
// The token stays in the environment: never in a URL, argument or Git config.
const ASKPASS = `#!/bin/sh
case "$1" in
  *Username*) printf '%s\\n' 'x-access-token' ;;
  *Password*) printf '%s\\n' "\${GH_TOKEN}" ;;
  *) exit 1 ;;
esac
`;

const git = (...args) => execFileSync('git', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] }).trim();
const succeeds = (cmd, ...args) => spawnSync(cmd, args, { stdio: 'ignore' }).status === 0;
const load = (path) => JSON.parse(readFileSync(path, 'utf8'));
const dump = (path, data) => writeFileSync(path, `${JSON.stringify(data, null, 2)}\n`);

function compareVersions(a, b) {
  const key = (value) => value.split(/([0-9]+)/).map((part, index) => (index % 2 ? Number(part) : part));
  const x = key(a);
  const y = key(b);
  for (let i = 0; i < Math.min(x.length, y.length); i++) {
    if (x[i] !== y[i]) {
      return x[i] < y[i] ? -1 : 1;
    }
  }
  return x.length - y.length;
}

function requestedVersion(chartYaml) {
  const match = /^version: *(.*)$/m.exec(chartYaml);
  return match ? match[1].replaceAll('"', '') : '';
}

// The next chart minor, or the version set in Chart.yaml on main when that is
// higher: that is how a chart major is requested.
function nextChartVersion(latest, requested) {
  const [major, minor] = (latest || '0.0.0').split('.');
  return [`${major}.${Number(minor) + 1}.0`, requested].filter(Boolean).sort(compareVersions).at(-1);
}

function dispatch(branch) {
  // A push made with GITHUB_TOKEN starts no workflow.
  execFileSync('gh', ['workflow', 'run', 'release-please.yml', '--repo', process.env.GITHUB_REPOSITORY, '--ref', branch], { stdio: 'inherit' });
}

function seed() {
  const { TAG_NAME: tag, VERSION: version, MAJOR: major, MINOR: minor } = process.env;
  const branch = `release-${major}.${minor}`;
  if (succeeds('git', 'ls-remote', '--exit-code', '--heads', 'origin', branch)) {
    // A rerun after a failed dispatch lands here; an extra release-please run
    // on the branch is harmless.
    console.log(`${branch} already exists`);
    dispatch(branch);
    return;
  }

  // Not shallow: a depth=1 fetch would cut the history the anchor below needs.
  git('fetch', 'origin', `refs/tags/${tag}:refs/tags/${tag}`);
  const sha = git('rev-list', '-n', '1', tag);
  git('switch', '--detach', sha);
  writeFileSync('VERSION', `${version}\n`);

  // The branch owns the chart line for this app minor. The app release commit
  // already stamped appVersion into Chart.yaml, so it opens the first chart
  // release PR. last-release-sha is the newest chart tag or app minor release
  // behind it, not its parent: chart changes merged on main since the previous
  // line was cut ship in this release and belong in its notes.
  const tags = git('tag', '--list', 'chart-v[0-9]*', '--sort=-v:refname').split('\n').filter(Boolean);
  const latest = tags.length ? tags[0].replace(/^chart-v/, '') : '';
  const chartVersion = nextChartVersion(latest, requestedVersion(existsSync(CHART) ? readFileSync(CHART, 'utf8') : ''));
  let anchor;
  try {
    anchor = git('describe', '--tags', '--abbrev=0', '--match', 'chart-v*', '--match', 'v*.*.0', `${sha}^`);
  } catch {
    anchor = `${sha}^`;
  }

  const config = load(CONFIG);
  config['last-release-sha'] = git('rev-parse', `${anchor}^{commit}`);
  config.packages['deploy/chart']['release-as'] = chartVersion;
  dump(CONFIG, config);
  if (latest) {
    const manifest = load(MANIFEST);
    manifest['deploy/chart'] = latest;
    dump(MANIFEST, manifest);
  }

  git('add', 'VERSION', CONFIG, MANIFEST);
  git(...BOT, 'commit', '-s', '-m', `chore: Initialize ${branch} version and chart ${chartVersion} release line`);
  git('push', 'origin', `HEAD:refs/heads/${branch}`);
  dispatch(branch);
}

function dropReleaseAs() {
  // Left in place, the one-shot release-as makes validate-release-ref fail
  // every later run on this branch.
  const chartVersion = process.env.CHART_VERSION;
  const config = load(CONFIG);
  if (!chartVersion || config.packages['deploy/chart']['release-as'] !== chartVersion) {
    return;
  }
  delete config.packages['deploy/chart']['release-as'];
  dump(CONFIG, config);
  git('add', CONFIG);
  git(...BOT, 'commit', '-s', '-m', `ci: Drop the fulfilled chart release-as ${chartVersion}`);
  git('push', 'origin', `HEAD:refs/heads/${process.env.GITHUB_REF_NAME}`);
}

const commands = { seed, 'drop-release-as': dropReleaseAs };
const command = process.argv[2];
if (process.argv.length !== 3 || !Object.hasOwn(commands, command)) {
  console.error(`usage: chart-release-line.mjs {${Object.keys(commands).join(',')}}`);
  process.exit(1);
}

const tmp = mkdtempSync(join(tmpdir(), 'chart-release-line-'));
try {
  const askpass = join(tmp, 'git-askpass');
  writeFileSync(askpass, ASKPASS);
  chmodSync(askpass, 0o700);
  Object.assign(process.env, { GIT_ASKPASS: askpass, GIT_TERMINAL_PROMPT: '0' });
  commands[command]();
} catch (error) {
  if (error.status === undefined) {
    throw error;
  }
  process.exitCode = error.status || 1;
} finally {
  rmSync(tmp, { recursive: true, force: true });
}
