import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';

const [formattedNotesPath, repository, outputPath] = process.argv.slice(2);

if (!formattedNotesPath || !repository || !outputPath) {
  throw new Error('usage: extract-pr-summary.mjs <formatted-notes> <owner/repo> <output>');
}

const formattedNotes = readFileSync(formattedNotesPath, 'utf8');
const escapedRepository = repository.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
// Only the canonical "in [#N](.../pull/N)" reference names the entry's own PR;
// a bare pull URL inside an entry title may point at an unrelated PR.
const canonicalPrPattern = new RegExp(
  `\\bin \\[#(\\d+)\\]\\(https://github\\.com/${escapedRepository}/pull/\\1\\)`,
  'g',
);
// The entry's own reference is the last canonical match on the line; an
// earlier one could sit inside the entry title.
const prNumbers = [...new Set(
  formattedNotes
    .split(/\r?\n/)
    .map(line => [...line.matchAll(canonicalPrPattern)].at(-1)?.[1])
    .filter(Boolean),
)];

function sleep(milliseconds) {
  Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, milliseconds);
}

function fetchPullRequest(number) {
  for (let attempt = 1; attempt <= 5; attempt++) {
    try {
      return JSON.parse(execFileSync('gh', ['api', `repos/${repository}/pulls/${number}`], {
        encoding: 'utf8',
        stdio: ['ignore', 'pipe', 'pipe'],
      }));
    } catch (error) {
      // A changelog reference that is not a pull request has no description to read.
      if (`${error.stderr ?? ''}`.includes('HTTP 404')) {
        return undefined;
      }

      if (attempt === 5) {
        throw error;
      }

      console.error(`GitHub request for PR #${number} failed (attempt ${attempt}/5); retrying...`);
      sleep(attempt * 2000);
    }
  }
}

function stripHtmlComments(text) {
  let previous;
  do {
    previous = text;
    text = text.replace(/<!--[\s\S]*?-->/g, '');
  } while (text !== previous);
  return text;
}

function releaseNotesSection(body) {
  const lines = (body ?? '').split(/\r?\n/);
  const start = lines.findIndex(line => /^##\s+release notes\s*$/i.test(line.trim()));
  if (start === -1) {
    return undefined;
  }

  let end = lines.findIndex((line, index) => index > start && /^\s{0,3}#{1,2}(?:[ \t]|$)/.test(line));
  if (end === -1) {
    end = lines.length;
  }

  const section = stripHtmlComments(lines.slice(start + 1, end).join('\n')).trim();

  if (!section || /^_?(?:none|n\/a|tbd)_?[.!]?$/i.test(section)) {
    return undefined;
  }

  return section;
}

const entries = [];

// Each section is emitted verbatim: no headings, no PR titles, no PR number.
for (const number of prNumbers) {
  const section = releaseNotesSection(fetchPullRequest(number)?.body);
  if (section) {
    entries.push(section);
  }
}

writeFileSync(
  outputPath,
  entries.length > 0 ? `## Summary\n\n${entries.join('\n\n')}\n` : '',
);
