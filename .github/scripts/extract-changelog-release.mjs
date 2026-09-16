import { readFileSync, writeFileSync } from 'node:fs';

const [changelogPath, version, outputPath] = process.argv.slice(2);

if (!changelogPath || !version || !outputPath) {
  throw new Error('usage: extract-changelog-release.mjs <changelog> <version> <output>');
}

const lines = readFileSync(changelogPath, 'utf8').split(/\r?\n/);
// release-please writes "## [x.y.z](compare-url) (date)" when a previous tag
// exists and "## x.y.z (date)" for a line's first release (e.g. the chart).
const headings = [`## [${version}]`, `## [v${version}]`, `## ${version} `, `## v${version} `];
const start = lines.findIndex(line => headings.some(heading => line.startsWith(heading)));

if (start === -1) {
  throw new Error(`CHANGELOG.md has no release section for ${version}`);
}

let end = lines.findIndex((line, index) => index > start && line.startsWith('## '));
if (end === -1) {
  end = lines.length;
}

writeFileSync(outputPath, `${lines.slice(start, end).join('\n').trim()}\n`);
