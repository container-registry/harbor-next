import { readFileSync, writeFileSync } from 'node:fs';

const [mode, inputPath, ...paths] = process.argv.slice(2);
const [notesPath, outputPath] = mode === 'upsert' ? paths : [undefined, paths[0]];
const startMarker = '<!-- harbor-release-notes:start -->';
const endMarker = '<!-- harbor-release-notes:end -->';

if (!mode || !inputPath || !outputPath || (mode === 'upsert' && !notesPath)) {
  throw new Error('usage: release-notes-preview.mjs <upsert|extract> <input> [notes] <output>');
}

let input = readFileSync(inputPath, 'utf8');

if (mode === 'upsert') {
  const notes = readFileSync(notesPath, 'utf8').trim();
  const withoutPreview = input.replace(
    new RegExp(`\\n?${startMarker}[\\s\\S]*?${endMarker}\\n?`),
    '\n',
  ).trimEnd();
  const preview = `${startMarker}\n${notes}\n${endMarker}`;

  writeFileSync(outputPath, `${withoutPreview}\n\n${preview}\n`);
} else if (mode === 'extract') {
  try {
    const pullRequests = JSON.parse(input).flat();
    input = pullRequests.find(pullRequest => pullRequest.body?.includes(startMarker))?.body ?? '';
  } catch {
    // The input is a pull request body rather than an API response.
  }

  const start = input.indexOf(startMarker);
  const end = input.indexOf(endMarker, start + startMarker.length);

  if (start === -1 || end === -1) {
    process.exit(1);
  }

  writeFileSync(outputPath, `${input.slice(start + startMarker.length, end).trim()}\n`);
} else {
  throw new Error(`unknown mode: ${mode}`);
}
