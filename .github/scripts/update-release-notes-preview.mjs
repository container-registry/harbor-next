import { readFileSync, writeFileSync } from 'node:fs';

const [bodyPath, notesPath, outputPath] = process.argv.slice(2);

if (!bodyPath || !notesPath || !outputPath) {
  throw new Error('usage: update-release-notes-preview.mjs <pr-body> <notes> <output>');
}

const body = readFileSync(bodyPath, 'utf8').trimEnd();
const notes = readFileSync(notesPath, 'utf8').trim();
const heading = '## Release Notes Preview';
const start = body.indexOf(heading);
const prefix = start === -1 ? body : body.slice(0, start).trimEnd();

writeFileSync(outputPath, `${prefix}\n\n${heading}\n\n${notes}\n`);
