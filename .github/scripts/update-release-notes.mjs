import { execFileSync, spawnSync } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { setTimeout as sleep } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';

// Renders a release's notes from its tagged changelog and publishes them.
// The release-notes engine drives every mode through the environment:
// TAG_NAME (vX.Y.Z[-prerelease] for the app, chart-vX.Y.Z for the chart),
// RELEASE_NOTES_PREVIEW_PR_NUMBER (rewrite a release PR's preview instead),
// RELEASE_NOTES_COMPONENT, RELEASE_NOTES_DRY_RUN and RELEASE_NOTES_OUTPUT.

const IMAGES = ['core', 'jobservice', 'registryctl', 'exporter', 'portal', 'registry', 'trivy-adapter'];
const SERIES = 'taskfile/commercial-patches';
const PATCHES_REMOTE = 'https://x-access-token@github.com/container-registry/8gcr';
const FENCE = '```';
const OIDC_ISSUER = '  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \\';
// The token stays in the environment: never in a URL, argument or Git config.
const ASKPASS = `#!/bin/sh
case "$1" in
  *Username*) printf '%s\\n' 'x-access-token' ;;
  *Password*) printf '%s\\n' "\${PATCHES_TOKEN}" ;;
  *) exit 1 ;;
esac
`;
const MAX_BUFFER = 64 * 1024 * 1024;

class Failure extends Error {}

const env = (name) => process.env[name] ?? '';
const read = (path) => readFileSync(path, 'utf8');
const lines = (...items) => items.map((item) => `${item}\n`).join('');
const trimNewlines = (text) => text.replace(/\n+$/, '');

function run(cmd, args, { quiet = false, extraEnv = {} } = {}) {
  return execFileSync(cmd, args, {
    encoding: 'utf8',
    maxBuffer: MAX_BUFFER,
    stdio: ['ignore', 'pipe', quiet ? 'ignore' : 'inherit'],
    env: { ...process.env, ...extraEnv },
  });
}

const out = (cmd, ...args) => trimNewlines(run(cmd, args));

function tryOut(cmd, ...args) {
  try {
    return trimNewlines(run(cmd, args, { quiet: true }));
  } catch {
    return '';
  }
}

const succeeds = (cmd, ...args) => spawnSync(cmd, args, { stdio: 'ignore' }).status === 0;

function node(script, ...args) {
  execFileSync(process.execPath, [`.github/scripts/${script}`, ...args], { stdio: 'inherit' });
}

async function github(cmd, ...args) {
  for (let attempt = 1; attempt <= 5; attempt++) {
    const result = spawnSync(cmd, args, { encoding: 'utf8', maxBuffer: MAX_BUFFER, stdio: ['ignore', 'pipe', 'inherit'] });
    if (result.status === 0) {
      return result.stdout;
    }
    if (attempt < 5) {
      console.error(`GitHub request failed (attempt ${attempt}/5); retrying...`);
      await sleep(attempt * 2000);
    }
  }
  throw new Failure(`${[cmd, ...args].slice(0, 3).join(' ')} failed after 5 attempts`);
}

function versionKey(value) {
  return value.split(/([0-9]+)/).map((part, index) => (index % 2 ? Number(part) : part));
}

export function compareVersions(a, b) {
  const x = versionKey(a);
  const y = versionKey(b);
  for (let i = 0; i < Math.min(x.length, y.length); i++) {
    if (x[i] !== y[i]) {
      return x[i] < y[i] ? -1 : 1;
    }
  }
  return x.length - y.length;
}

function manifestVersion(path, key, label) {
  const version = JSON.parse(read(path))[key];
  if (!version) {
    throw new Failure(`missing ${label} release version`);
  }
  return version;
}

function resolveTag(previewPr) {
  let tag = env('TAG_NAME');
  if (previewPr && env('RELEASE_NOTES_COMPONENT') === 'chart') {
    // A chart release PR bumps the chart manifest on its branch to the new version.
    tag = `chart-v${manifestVersion('.release-please-manifest-chart.json', 'deploy/chart', 'chart')}`;
  } else if (previewPr && !/^v[0-9]+\.[0-9]+\.[0-9]+$/.test(tag)) {
    tag = `v${manifestVersion('.release-please-manifest.json', '.', 'root')}`;
  }
  if (!tag) {
    throw new Failure('TAG_NAME is required');
  }
  return redirectAnchor(tag);
}

// A dated anchor is not a release: its notes live on the rolling pre-release.
// Redirect only while the pointer still points at the anchor. Once it has
// moved on, the beta's notes describe a different build. nightly- is the
// pre-rename spelling; anchors published under it still exist.
function redirectAnchor(tag) {
  const match = /^v([0-9]+\.[0-9]+\.[0-9]+)-(beta\.|nightly-)[0-9]{8}$/.exec(tag);
  if (!match) {
    return tag;
  }
  const betaTag = `v${match[1]}-beta`;
  const anchorSha = tryOut('git', 'rev-parse', '-q', '--verify', `${tag}^{commit}`);
  if (anchorSha && anchorSha === tryOut('git', 'rev-parse', '-q', '--verify', `${betaTag}^{commit}`)) {
    console.error(`::notice::${tag} carries no release; rendering ${betaTag}`);
    return betaTag;
  }
  throw new Failure(`${tag} has no release, and ${betaTag} has moved on. Render ${betaTag} directly if that is what you want.`);
}

function repository() {
  if (env('GITHUB_REPOSITORY')) {
    return env('GITHUB_REPOSITORY');
  }
  const url = tryOut('git', 'remote', 'get-url', 'next');
  const repo = url.replace(/^(git@github\.com:|https:\/\/github\.com\/)/, '').replace(/\.git$/, '');
  return repo || out('gh', 'repo', 'view', '--json', 'nameWithOwner', '--jq', '.nameWithOwner');
}

function parseTag(tag) {
  let match = /^chart-v([0-9]+\.[0-9]+\.[0-9]+)$/.exec(tag);
  if (match) {
    return [true, match[1]];
  }
  // The prerelease suffix is the preview channel: v2.16.0-beta.20260918
  // renders the upcoming 2.16.0's notes as they stand tonight.
  match = /^v([0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?)$/.exec(tag);
  if (match) {
    return [false, match[1]];
  }
  throw new Failure('TAG_NAME must be vX.Y.Z[-prerelease] (app release) or chart-vX.Y.Z (chart release)');
}

function chartField(chartYaml, key) {
  for (const line of chartYaml.split('\n')) {
    const fields = line.split(/[:\s]+/);
    if (fields[0] === key) {
      return (fields[1] ?? '').replaceAll('"', '');
    }
  }
  return '';
}

// The greatest chart tag below the target by version, tagged yet or not.
// Chart lines on different release branches advance independently, so a
// 2.1.x preview must not diff against a chart-v2.2.0 that already exists.
function previousChartTag(tag) {
  const tags = [...new Set([...out('git', 'tag', '--list', 'chart-v*').split('\n').filter(Boolean), tag])].sort(compareVersions);
  const index = tags.indexOf(tag);
  return index ? tags[index - 1] : '';
}

async function generatedNotes(tag, version, chartMode, previewPr, repo) {
  // Without an explicit previous tag GitHub infers one: for the chart it
  // picks the latest app release, for the app it counts prereleases.
  const args = ['-f', `tag_name=${tag}`];
  let previous;
  if (chartMode) {
    previous = previousChartTag(tag);
    if (!previous) {
      // First chart release: any range would attribute unrelated app PRs.
      return '';
    }
  } else {
    // -o interleaved: the root Taskfile prefixes output per task.
    previous = out('task', '-o', 'interleaved', 'release-notes:previous-stable-tag', `PREDECESSOR_FOR=${version}`);
  }
  if (previous) {
    args.push('-f', `previous_tag_name=${previous}`);
  }
  if (previewPr) {
    args.push('-f', `target_commitish=${out('git', 'rev-parse', 'HEAD')}`);
  }
  return github('gh', 'api', `repos/${repo}/releases/generate-notes`, ...args, '--jq', '.body');
}

async function releaseBranch(tag, chartMode, previewPr, repo) {
  const refName = env('GITHUB_REF_NAME');
  if (chartMode) {
    // A chart release's target_commitish is a pinned SHA and a manual
    // recreate can run from any branch, so only a preview trusts the ref.
    // chart-v2.1.1 and older were published from main, later ones from the
    // release-X.Y branch that tagged them.
    if (previewPr && refName) {
      return refName;
    }
    if (succeeds('git', 'merge-base', '--is-ancestor', tag, 'origin/main')) {
      return 'main';
    }
    for (const ref of out('git', 'branch', '-r', '--contains', tag, '--format=%(refname:short)').split('\n')) {
      const match = /^origin\/(release-[0-9]*\.[0-9]*)$/.exec(ref);
      if (match) {
        return match[1];
      }
    }
    return '';
  }
  if (previewPr) {
    if (!refName) {
      throw new Failure('GITHUB_REF_NAME is required for a release PR preview');
    }
    return refName;
  }
  // Push-triggered runs only: a manual recreate on a tag ref has the tag as GITHUB_REF_NAME.
  if (env('GITHUB_REF_TYPE') === 'branch' && refName) {
    return refName;
  }
  return trimNewlines(await github('gh', 'release', 'view', tag, '--repo', repo, '--json', 'targetCommitish', '--jq', '.targetCommitish'));
}

function awkNumber(value) {
  const match = /^\s*([-+]?[0-9]+)/.exec(value);
  return match ? Number.parseInt(match[1], 10) : 0;
}

function newer(a, b) {
  const x = [...a.replace(/^v/, '').split('.'), '', '', ''];
  const y = [...b.replace(/^v/, '').split('.'), '', '', ''];
  for (let i = 0; i < 3; i++) {
    if (awkNumber(x[i]) !== awkNumber(y[i])) {
      return awkNumber(x[i]) > awkNumber(y[i]);
    }
  }
  return false;
}

// This release's entries from a patch branch's changelogs/<branch>.md.
// The release cut stamps "--- release vX.Y.Z (target <sha12>) ---" markers,
// newest first. Notes render before the marker is stamped, so the block
// above the first marker is the current release's delta; an older tag reads
// its own section. A marker newer than the tag with no section of its own
// means a re-render of a pre-changelog tag: nothing.
export function releaseEntries(changelog, tag) {
  let seenMarker = false;
  let inTag = false;
  let found = false;
  let sawNewer = false;
  let seenTitle = false;
  const tagged = [];
  const unreleased = [];
  for (const line of changelog.split('\n')) {
    if (line.startsWith('--- release ')) {
      const marker = line.split(/\s+/)[2] ?? '';
      seenMarker = true;
      inTag = marker === tag;
      found ||= inTag;
      sawNewer ||= newer(marker, tag);
    } else if (!seenTitle) {
      seenTitle = line.startsWith('# ');
    } else if (line.startsWith('- ') || line.startsWith('  ')) {
      if (inTag) {
        tagged.push(line);
      } else if (!seenMarker) {
        unreleased.push(line);
      }
    }
  }
  return (found ? tagged : sawNewer ? [] : unreleased).join('\n');
}

// Notes per commercial patch branch listed in the series. The Harbor branch
// owns the ordered list; 8gcr only stores the branch commits. Nothing here
// reads a commit subject: under merge-sync every patch branch tip is a
// "sync: merge ..." commit, never a feature name.
function commercialNotes(tag, tmp, token) {
  if (!existsSync(SERIES)) {
    return [0, ''];
  }
  const askpass = join(tmp, 'git-askpass.sh');
  writeFileSync(askpass, ASKPASS);
  chmodSync(askpass, 0o700);
  const gitEnv = { GIT_ASKPASS: askpass, GIT_TERMINAL_PROMPT: '0', PATCHES_TOKEN: token };
  const patches = join(tmp, 'patches-repo');
  run('git', ['init', '-q', '--bare', patches]);
  let declared = 0;
  let notes = '';
  for (const line of read(SERIES).split('\n')) {
    const branch = line.split('#')[0].trim();
    if (!branch) {
      continue;
    }
    if (!succeeds('git', 'check-ref-format', '--branch', branch)) {
      throw new Failure(`Invalid commercial branch name in series: ${branch}`);
    }
    run('git', ['-C', patches, 'fetch', '--depth=1', PATCHES_REMOTE, `${branch}:refs/remotes/origin/${branch}`], { extraEnv: gitEnv });
    declared++;
    const changelog = tryOut('git', '-C', patches, 'cat-file', '-p', `refs/remotes/origin/${branch}:changelogs/${branch}.md`);
    let title = changelog ? (changelog.split('\n').find((entry) => entry.startsWith('# ')) ?? '').slice(2) : '';
    const entries = changelog ? releaseEntries(changelog, tag) : '';
    if (!title) {
      console.error(`::warning::${branch} has no '# Title' in changelogs/${branch}.md`);
      title = branch;
    }
    if (entries) {
      notes += `### ${title}\n\n${entries}\n\n`;
    }
  }
  return [declared, notes];
}

function render(ctx) {
  const { tag, chartMode, registry, repo, branch } = ctx;
  const parts = [];
  if (ctx.summary) {
    parts.push(`${ctx.summary}\n`);
  }
  if (ctx.declared) {
    // The branch inventory once listed here carried commit subjects, which
    // is how sync merges reached the notes.
    parts.push(lines('## Commercial Features', ''), ctx.commercial || lines('_No changes this release._', ''));
  }
  parts.push(ctx.formatted, lines('', '---', ''));
  const chartRef = `${registry}/charts/${ctx.chartName}`;
  if (ctx.chartVersion) {
    parts.push(lines(
      '## Helm Chart',
      '',
      `${FENCE}sh`,
      `helm install harbor oci://${chartRef} \\`,
      `  --version ${ctx.chartVersion} -n harbor --create-namespace -f my-values.yaml`,
      FENCE,
      '',
    ));
    if (chartMode) {
      parts.push(lines(
        'Signed with [cosign](https://github.com/sigstore/cosign). **Verify the chart signature:**',
        `${FENCE}sh`,
        'cosign verify \\',
        `  --certificate-identity "https://github.com/${repo}/.github/workflows/publish-chart.yml@refs/heads/${branch}" \\`,
        OIDC_ISSUER,
        `  ${chartRef}:${ctx.chartVersion}`,
        FENCE,
        '',
      ));
    }
    parts.push(lines('---', ''));
  }
  if (chartMode) {
    const appVersion = chartField(ctx.chartYaml, 'appVersion');
    if (appVersion) {
      parts.push(lines(`Default Harbor image tags follow the chart appVersion: \`${appVersion}\`.`));
    }
    return parts.join('');
  }
  parts.push(lines(
    '## Container Images',
    '',
    'Multi-arch images (`linux/amd64`, `linux/arm64`) signed with [cosign](https://github.com/sigstore/cosign).',
    '',
    '| Image | Reference |',
    '|-------|-----------|',
  ));
  for (const image of IMAGES) {
    const name = image === 'trivy-adapter' ? image : `harbor-${image}`;
    parts.push(lines(`| \`${name}\` | \`${registry}/${name}:${tag}\` |`));
  }
  parts.push(lines(
    '',
    '**Verify an image signature:**',
    `${FENCE}sh`,
    'cosign verify \\',
    `  --certificate-identity "https://github.com/${repo}/.github/workflows/publish-images.yml@refs/heads/${branch}" \\`,
    OIDC_ISSUER,
    `  ${registry}/harbor-core:${tag}`,
    FENCE,
  ));
  // New Contributors spans every PR between the tags, which for the chart
  // line would credit unrelated app work: app releases only.
  if (ctx.contributors) {
    parts.push(lines('', '---', ''), ctx.contributors);
  }
  return parts.join('');
}

async function main() {
  if (!env('GH_TOKEN')) {
    throw new Failure('GH_TOKEN is required');
  }
  const previewPr = env('RELEASE_NOTES_PREVIEW_PR_NUMBER');
  const tag = resolveTag(previewPr);
  const repo = repository();
  const [chartMode, version] = parseTag(tag);
  const registry = `${env('REGISTRY_ADDRESS') || '8gears.container-registry.com'}/${env('REGISTRY_PROJECT') || '8gcr'}`;

  // The chart releases on its own line, so an app release shows the chart
  // committed at its ref: the newest chart published at that time.
  const chartPath = 'deploy/chart/Chart.yaml';
  const chartYaml = previewPr
    ? (existsSync(chartPath) ? read(chartPath) : '')
    : tryOut('git', 'show', `${tag}:${chartPath}`);

  const tmp = mkdtempSync(join(tmpdir(), 'release-notes-'));
  try {
    const file = (name) => join(tmp, name);
    const optional = (name) => (existsSync(file(name)) ? read(file(name)) : '');
    const changelog = chartMode ? 'deploy/chart/CHANGELOG.md' : 'CHANGELOG.md';
    writeFileSync(file('CHANGELOG.md'), previewPr ? read(changelog) : run('git', ['show', `${tag}:${changelog}`]));
    node('extract-changelog-release.mjs', file('CHANGELOG.md'), version, file('release-source.md'));
    writeFileSync(file('generated-notes.md'), await generatedNotes(tag, version, chartMode, previewPr, repo));
    node('format-release-notes.mjs', file('release-source.md'), file('generated-notes.md'), file('formatted-notes.md'), file('contributors.md'));
    node('extract-pr-summary.mjs', file('formatted-notes.md'), repo, file('summary.md'));

    const branch = await releaseBranch(tag, chartMode, previewPr, repo);
    if (!branch) {
      throw new Failure(`Release ${tag} has no target branch`);
    }
    const [declared, commercial] = chartMode ? [0, ''] : commercialNotes(tag, tmp, env('PATCHES_TOKEN') || env('GH_TOKEN'));

    const notes = render({
      tag,
      chartMode,
      registry,
      repo,
      branch,
      declared,
      commercial,
      chartYaml,
      summary: optional('summary.md'),
      formatted: read(file('formatted-notes.md')),
      contributors: optional('contributors.md'),
      chartName: chartField(chartYaml, 'name') || 'harbor-next',
      chartVersion: chartMode ? version : chartField(chartYaml, 'version'),
    });
    const notesPath = file('release-notes.md');
    writeFileSync(notesPath, notes);

    const dryRun = env('RELEASE_NOTES_DRY_RUN') === 'true';
    const output = env('RELEASE_NOTES_OUTPUT');
    if (previewPr && !dryRun) {
      node('update-release-notes-preview.mjs', notesPath, file('release-pr-body-with-preview.md'));
      await github('gh', 'pr', 'edit', previewPr, '--repo', repo, '--body-file', file('release-pr-body-with-preview.md'));
    } else if (dryRun && output) {
      writeFileSync(output, notes);
      console.log(`Wrote release notes to ${output}`);
    } else if (dryRun) {
      process.stdout.write(notes);
    } else {
      await github('gh', 'release', 'edit', tag, '--repo', repo, '--notes-file', notesPath);
    }
  } finally {
    rmSync(tmp, { recursive: true, force: true });
  }
}

if (process.argv[1] === fileURLToPath(import.meta.url)) {
  main().catch((error) => {
    if (error instanceof Failure) {
      console.error(error.message);
      process.exit(1);
    }
    if (error.status !== undefined) {
      process.exit(error.status || 1);
    }
    throw error;
  });
}
