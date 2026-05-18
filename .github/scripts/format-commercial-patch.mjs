import { readFileSync } from 'node:fs';

const [patchPath] = process.argv.slice(2);

if (!patchPath) {
  throw new Error('usage: format-commercial-patch.mjs <patch-file>');
}

const lines = readFileSync(patchPath, 'utf8').split(/\r?\n/);
const subjectIndex = lines.findIndex(line => line.startsWith('Subject: '));

if (subjectIndex === -1 && !lines[0]?.trim()) {
  process.exit(0);
}

const subjectLine = subjectIndex === -1 ? lines[0] : lines[subjectIndex];
const subject = subjectLine
  .replace(/^Subject: /, '')
  .replace(/^\[PATCH[^\]]*\]\s*/, '')
  .trim();

const headerIndex = subjectIndex === -1 ? 0 : subjectIndex;
let bodyStart = lines.findIndex((line, index) => index > headerIndex && line === '');
if (bodyStart === -1) {
  bodyStart = headerIndex;
}

const bodyEnd = lines.findIndex((line, index) => index > bodyStart && line === '---');
const bodyLines = lines
  .slice(bodyStart + 1, bodyEnd === -1 ? lines.length : bodyEnd)
  .filter(line => !line.startsWith('From: ') && !/^[A-Za-z-]+-by: /i.test(line));

while (bodyLines.length > 0 && bodyLines[0].trim() === '') {
  bodyLines.shift();
}

while (bodyLines.length > 0 && bodyLines.at(-1).trim() === '') {
  bodyLines.pop();
}

console.log(`- **${subject}**`);

if (bodyLines.length > 0) {
  console.log('');
  for (const line of bodyLines) {
    console.log(line.trim() ? `  ${line}` : '');
  }
  console.log('');
}
