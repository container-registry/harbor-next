import { readFileSync } from 'node:fs';

const [patchPath] = process.argv.slice(2);

if (!patchPath) {
  throw new Error('usage: format-commercial-patch.mjs <patch-file>');
}

const lines = readFileSync(patchPath, 'utf8').split(/\r?\n/);
const subjectIndex = lines.findIndex(line => line.startsWith('Subject: '));

function cleanSubject(value) {
  return value
    .replace(/^Subject: /, '')
    .replace(/^\[PATCH[^\]]*\]\s*/, '')
    .trim();
}

function trimBody(bodyLines) {
  while (bodyLines.length > 0 && bodyLines[0].trim() === '') {
    bodyLines.shift();
  }

  while (bodyLines.length > 0 && bodyLines.at(-1).trim() === '') {
    bodyLines.pop();
  }

  return bodyLines;
}

let subject;
let bodyLines;

if (subjectIndex !== -1) {
  subject = cleanSubject(lines[subjectIndex]);

  let bodyStart = lines.findIndex((line, index) => index > subjectIndex && line === '');
  if (bodyStart === -1) {
    bodyStart = subjectIndex;
  }

  const bodyEnd = lines.findIndex((line, index) => index > bodyStart && line === '---');
  bodyLines = lines.slice(bodyStart + 1, bodyEnd === -1 ? lines.length : bodyEnd);
} else {
  const titleIndex = lines.findIndex(line => line.trim() !== '');
  if (titleIndex === -1) {
    process.exit(0);
  }

  subject = cleanSubject(lines[titleIndex]);
  const bodyEnd = lines.findIndex((line, index) => index > titleIndex && line === '---');
  bodyLines = lines
    .slice(titleIndex + 1, bodyEnd === -1 ? lines.length : bodyEnd)
    .filter(line => !line.startsWith('From: ') && !line.startsWith('Signed-off-by: '));
}

if (!subject) {
  process.exit(0);
}

bodyLines = trimBody(bodyLines);

console.log(`- **${subject}**`);

if (bodyLines.length > 0) {
  console.log('');
  for (const line of bodyLines) {
    console.log(line.trim() ? `  ${line}` : '');
  }
  console.log('');
}
