import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';

const [releaseBodyPath, generatedNotesPath, outputPath] = process.argv.slice(2);

if (!releaseBodyPath || !generatedNotesPath || !outputPath) {
  throw new Error('usage: format-release-notes.mjs <release-body> <generated-notes> <output>');
}

const releaseBody = readFileSync(releaseBodyPath, 'utf8');
const generatedNotes = readFileSync(generatedNotesPath, 'utf8');
const authorsByPr = new Map();
const upstreamMetadataByCommit = new Map();

for (const match of generatedNotes.matchAll(/by (@[^\s]+) in https:\/\/github\.com\/[^/]+\/[^/]+\/pull\/(\d+)/g)) {
  authorsByPr.set(match[2], match[1]);
}

const sectionNames = new Map([
  ['Bug Fixes', 'Fixes'],
  ['Performance Improvements', 'Updates'],
  ['Code Refactoring', 'Updates'],
  ['Documentation', 'Updates'],
]);
const sectionOrder = ['Features', 'Fixes', 'Updates', 'Upstream', 'Reverts'];
const sections = new Map(sectionOrder.map(section => [section, []]));
const trailing = [];
let currentSection = '';
let sawSection = false;

function formatEntry(line) {
  let entry = line.replace(/^\* /, '- ');

  entry = entry.replace(
    /\(\[#(\d+)\]\(https:\/\/github\.com\/([^/]+)\/([^/]+)\/issues\/\1\)\)\s+\((\[[^)]+\]\([^)]+\))\)/g,
    (_match, number, owner, repo, commit) => {
      const author = authorsByPr.get(number);
      const prLink = `[#${number}](https://github.com/${owner}/${repo}/pull/${number})`;

      if (!author) {
        return `in ${prLink} (${commit})`;
      }

      return `by ${author} in ${prLink} (${commit})`;
    },
  );

  return entry;
}

function commitShaFromEntry(entry) {
  return entry.match(/https:\/\/github\.com\/[^/]+\/[^/]+\/commit\/([0-9a-f]{7,40})/i)?.[1];
}

function commitMessage(sha) {
  try {
    return execFileSync('git', ['show', '-s', '--format=%B', sha], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    });
  } catch {
    return '';
  }
}

function normalizeUpstreamPr(value) {
  if (!value) {
    return undefined;
  }

  return value.match(/(?:goharbor\/harbor#|github\.com\/goharbor\/harbor\/pull\/|#)(\d+)/)?.[1]
    ?? value.match(/^(\d+)$/)?.[1];
}

function normalizeAuthor(value) {
  return value?.trim().match(/^@[A-Za-z0-9-]+(?:\[bot\])?/)?.[0];
}

function parseUpstreamMetadata(message) {
  const pr = normalizeUpstreamPr(message.match(/^Upstream-PR:\s*(.+)$/im)?.[1]);
  const author = normalizeAuthor(message.match(/^Upstream-Author:\s*(.+)$/im)?.[1]);

  if (!pr && !author) {
    return undefined;
  }

  return {pr, author};
}

function metadataForCommit(sha) {
  if (!sha) {
    return undefined;
  }

  if (!upstreamMetadataByCommit.has(sha)) {
    upstreamMetadataByCommit.set(sha, parseUpstreamMetadata(commitMessage(sha)) ?? null);
  }

  return upstreamMetadataByCommit.get(sha) ?? undefined;
}

function parseInlineUpstreamMetadata(entry) {
  const formatted = entry.match(/\sby\s+(@[A-Za-z0-9-]+(?:\[bot\])?)\s+in\s+(?:\[goharbor\/harbor#(\d+)\]\(https:\/\/github\.com\/goharbor\/harbor\/pull\/\2\)|https:\/\/github\.com\/goharbor\/harbor\/pull\/(\d+))/i);
  if (formatted) {
    return {
      pr: formatted[2] ?? formatted[3],
      author: normalizeAuthor(formatted[1]),
    };
  }

  const inline = entry.match(/\((?:upstream\s+)?(?:PR\s+)?((?:goharbor\/harbor#|https:\/\/github\.com\/goharbor\/harbor\/pull\/)\d+)\s+by\s+(@[A-Za-z0-9-]+(?:\[bot\])?)\)/i);
  if (!inline) {
    return undefined;
  }

  return {
    pr: normalizeUpstreamPr(inline[1]),
    author: normalizeAuthor(inline[2]),
  };
}

function upstreamPrLink(pr) {
  return `[goharbor/harbor#${pr}](https://github.com/goharbor/harbor/pull/${pr})`;
}

function formatUpstreamEntry(entry, sha) {
  const commitMetadata = metadataForCommit(sha);
  const inlineMetadata = parseInlineUpstreamMetadata(entry);
  const metadata = {
    pr: commitMetadata?.pr ?? inlineMetadata?.pr,
    author: commitMetadata?.author ?? inlineMetadata?.author,
  };
  let formatted = entry
    .replace(/\s*\[upstream\]\s*/i, ' ')
    .replace(/\sby\s+@[A-Za-z0-9-]+(?:\[bot\])?\s+in\s+(?:\[goharbor\/harbor#\d+\]\(https:\/\/github\.com\/goharbor\/harbor\/pull\/\d+\)|https:\/\/github\.com\/goharbor\/harbor\/pull\/\d+)/i, '')
    .replace(/\s+\((?:upstream\s+)?(?:PR\s+)?(?:goharbor\/harbor#|https:\/\/github\.com\/goharbor\/harbor\/pull\/)\d+\s+by\s+@[A-Za-z0-9-]+(?:\[bot\])?\)/i, '')
    .replace(/\s{2,}/g, ' ');

  if (!metadata?.pr && !metadata?.author) {
    return formatted;
  }

  const details = [
    metadata.author ? `by ${metadata.author}` : undefined,
    metadata.pr ? `in ${upstreamPrLink(metadata.pr)}` : undefined,
  ].filter(Boolean).join(' ');

  const commitSuffix = formatted.match(/\s+\(\[[0-9a-f]+\]\(https:\/\/github\.com\/[^)]+\/commit\/[0-9a-f]+\)\)$/i);
  if (!commitSuffix) {
    return `${formatted} ${details}`;
  }

  return `${formatted.slice(0, commitSuffix.index)} ${details}${commitSuffix[0]}`;
}

function releaseNotesLines(body) {
  const lines = body.split(/\r?\n/);
  const start = lines.findIndex(line => line.trim() === '## What\'s Changed');
  if (start === -1) {
    return lines;
  }

  const block = [];
  for (const line of lines.slice(start + 1)) {
    if (line.startsWith('## ')) {
      break;
    }

    block.push(line);
  }

  return block;
}

for (const line of releaseNotesLines(releaseBody)) {
  if (line.startsWith('## ')) {
    continue;
  }

  if (line.startsWith('### ')) {
    sawSection = true;
    currentSection = sectionNames.get(line.slice(4)) ?? line.slice(4);
    if (!sections.has(currentSection)) {
      sections.set(currentSection, []);
    }
    continue;
  }

  if (line.startsWith('* ') || line.startsWith('- ')) {
    let entry = formatEntry(line);
    const targetSection = entry.toLowerCase().includes('[upstream]') || currentSection === 'Upstream' ? 'Upstream' : currentSection;

    if (targetSection === 'Upstream') {
      entry = formatUpstreamEntry(entry, commitShaFromEntry(entry));
    }

    if (targetSection) {
      sections.get(targetSection)?.push(entry);
      continue;
    }
  }

  if (sawSection && line.trim()) {
    trailing.push(line);
  }
}

const output = ['## What\'s Changed'];
const emittedSections = new Set();

for (const section of sectionOrder) {
  const entries = sections.get(section) ?? [];
  if (entries.length === 0) {
    continue;
  }

  output.push('', `### ${section}`, '', ...entries);
  emittedSections.add(section);
}

for (const [section, entries] of sections) {
  if (emittedSections.has(section) || entries.length === 0) {
    continue;
  }

  output.push('', `### ${section}`, '', ...entries);
}

if (trailing.length > 0) {
  output.push('', ...trailing);
}

const newContributors = generatedNotes.match(/## New Contributors[\s\S]*?(?=\n\n\*\*Full Changelog\*\*|$)/)?.[0];
if (newContributors) {
  output.push('', newContributors.replace('## New Contributors', '### New Contributors').replace(/^\*/gm, '-'));
}

writeFileSync(outputPath, `${output.join('\n').trim()}\n`);
