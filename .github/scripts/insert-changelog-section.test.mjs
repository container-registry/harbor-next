// Run with: node --test .github/scripts/insert-changelog-section.test.mjs
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const script = new URL('./insert-changelog-section.mjs', import.meta.url).pathname;

function insert(changelog, version) {
  const dir = mkdtempSync(join(tmpdir(), 'insert-changelog-'));
  const changelogPath = join(dir, 'CHANGELOG.md');
  const sectionPath = join(dir, 'section.md');
  writeFileSync(changelogPath, changelog);
  writeFileSync(sectionPath, `## [${version}](https://example.test) (2026-11-10)\n\n* new entry\n`);
  execFileSync('node', [script, changelogPath, sectionPath, version]);
  return readFileSync(changelogPath, 'utf8');
}

function headings(changelog) {
  return changelog.split('\n').filter((line) => line.startsWith('## '));
}

test('patch lands below a newer minor already on main', () => {
  const out = insert('# Changelog\n\n## [2.16.0](x)\n\n* a\n\n## [2.15.13](x)\n\n* b\n', '2.15.14');
  assert.deepEqual(headings(out), ['## [2.16.0](x)', '## [2.15.14](https://example.test) (2026-11-10)', '## [2.15.13](x)']);
});

test('newest release goes to the top', () => {
  const out = insert('# Changelog\n\n## [2.15.13](x)\n\n* b\n', '2.15.14');
  assert.equal(headings(out)[0], '## [2.15.14](https://example.test) (2026-11-10)');
});

test('legacy unbracketed blocks count as older', () => {
  const out = insert('# Changelog\n\n## v2.14.x\n\n### v2.14.3\n\n* old\n', '2.15.1');
  assert.deepEqual(headings(out), ['## [2.15.1](https://example.test) (2026-11-10)', '## v2.14.x']);
});

test('a version already present is left alone', () => {
  for (const changelog of [
    '# Changelog\n\n## [2.15.14](x) (2026-10-23)\n\n* b\n',
    '# Changelog\n\n## v2.15.x\n\n### v2.15.14\n\n* b\n',
  ]) {
    assert.equal(insert(changelog, '2.15.14'), changelog);
  }
});

test('a prefix of another version is not mistaken for it', () => {
  const out = insert('# Changelog\n\n## [2.15.10](x)\n\n* b\n', '2.15.1');
  assert.deepEqual(headings(out), ['## [2.15.10](x)', '## [2.15.1](https://example.test) (2026-11-10)']);
});

test('appends when no older release exists', () => {
  const out = insert('# Changelog\n\n## [2.16.0](x)\n\n* a\n', '2.15.0');
  assert.deepEqual(headings(out), ['## [2.16.0](x)', '## [2.15.0](https://example.test) (2026-11-10)']);
});
