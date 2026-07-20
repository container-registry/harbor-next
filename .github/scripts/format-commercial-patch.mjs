import { readFileSync } from 'node:fs';

const [patchPath] = process.argv.slice(2);

if (!patchPath) {
  throw new Error('usage: format-commercial-patch.mjs <patch-file>');
}

const lines = readFileSync(patchPath, 'utf8').split(/\r?\n/);
let subjectIndex = lines.findIndex(line => line.startsWith('Subject: '));
let includeBody = true;

if (subjectIndex === -1) {
  subjectIndex = lines.findIndex(line => line.trim() !== '');
  includeBody = false;

  if (subjectIndex === -1 || lines[subjectIndex].startsWith('From ')) {
    process.exit(0);
  }
}

const subject = lines[subjectIndex]
  .replace(/^Subject: /, '')
  .replace(/^\[PATCH[^\]]*\]\s*/, '')
  .trim();

let bodyStart = subjectIndex;
if (includeBody) {
  bodyStart = lines.findIndex((line, index) => index > subjectIndex && line === '');
  if (bodyStart === -1) {
    bodyStart = subjectIndex;
  }
}

const bodyEnd = includeBody ? lines.findIndex((line, index) => index > bodyStart && line === '---') : -1;
const bodyLines = includeBody ? lines.slice(bodyStart + 1, bodyEnd === -1 ? lines.length : bodyEnd) : [];

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
