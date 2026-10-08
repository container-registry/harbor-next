import { readFileSync, writeFileSync } from 'node:fs';

const [changelogPath, sectionPath, version] = process.argv.slice(2);

if (!changelogPath || !sectionPath || !version) {
  throw new Error('usage: insert-changelog-section.mjs <changelog> <section> <version>');
}

if (!/^\d+\.\d+\.\d+$/.test(version)) {
  throw new Error(`version must be X.Y.Z, got: ${version}`);
}

const section = `${readFileSync(sectionPath, 'utf8').trim()}\n`;
const changelog = readFileSync(changelogPath, 'utf8');
const escapedVersion = version.replaceAll('.', '\\.');

// Idempotent: a heading for this version is already present, in either the
// release-please form (## [X.Y.Z]) or a legacy nested form (### vX.Y.Z).
if (new RegExp(`^#{2,3} \\[?v?${escapedVersion}\\]?\\b`, 'm').test(changelog)) {
  process.exit(0);
}

function compareVersions(a, b) {
  const x = a.split('.').map(Number);
  const y = b.split('.').map(Number);
  for (let i = 0; i < 3; i++) {
    if (x[i] !== y[i]) {
      return x[i] - y[i];
    }
  }
  return 0;
}

const lines = changelog.split('\n');
// Insert before the first release section older than this version, so a
// patch release lands below any newer minor already on main. Legacy
// unbracketed blocks ("## v2.14.x") are always older than new releases.
let insertLine = -1;
for (let i = 0; i < lines.length; i++) {
  const release = lines[i].match(/^## \[v?(\d+\.\d+\.\d+)\]/)?.[1];
  if (release && compareVersions(release, version) < 0) {
    insertLine = i;
    break;
  }
  const legacy = lines[i].match(/^## v(\d+)\.(\d+)\.x/);
  if (legacy && compareVersions(`${legacy[1]}.${legacy[2]}.0`, version) < 0) {
    insertLine = i;
    break;
  }
}

const output = insertLine === -1
  ? `${changelog.trimEnd()}\n\n${section}`
  : [...lines.slice(0, insertLine), section, ...lines.slice(insertLine)].join('\n');

writeFileSync(changelogPath, output);
