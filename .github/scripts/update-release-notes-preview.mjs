import { readFileSync, writeFileSync } from 'node:fs';

const [notesPath, outputPath] = process.argv.slice(2);

if (!notesPath || !outputPath) {
  throw new Error('usage: update-release-notes-preview.mjs <notes> <output>');
}

const notes = readFileSync(notesPath, 'utf8').trim();
const heading = '## Release Notes Preview';

writeFileSync(outputPath, `${heading}\n\n${notes}\n`);
