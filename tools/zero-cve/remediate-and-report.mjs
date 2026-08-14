#!/usr/bin/env node

import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { spawnSync } from 'node:child_process';

const repoRoot = process.cwd();
const reportsDir = process.env.ZERO_CVE_REPORT_DIR || 'zero-cve-reports';
const moduleDir = process.env.ZERO_CVE_MODULE_DIR || 'src';
const imageRefs = parseList(process.env.ZERO_CVE_IMAGES || process.env.ZERO_CVE_IMAGE || '8gears.container-registry.com/hcr:latest');
const imageRef = imageRefs.join(', ');
const applyRemediation = process.env.ZERO_CVE_APPLY !== 'false';
const maxTableRows = Number.parseInt(process.env.ZERO_CVE_MAX_TABLE_ROWS || '100', 10);
const generatedAt = new Date().toISOString();

mkdirSync(path.join(repoRoot, reportsDir), { recursive: true });

const severityRank = new Map([
  ['CRITICAL', 5],
  ['HIGH', 4],
  ['MEDIUM', 3],
  ['LOW', 2],
  ['UNKNOWN', 1],
]);

function readText(relPath) {
  const absPath = path.join(repoRoot, relPath);
  if (!existsSync(absPath)) {
    return '';
  }
  return readFileSync(absPath, 'utf8');
}

function writeReport(name, content) {
  writeFileSync(path.join(repoRoot, reportsDir, name), content);
}

function parseList(value) {
  return String(value || '')
    .split(/[\s,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseJSONFile(relPath) {
  const text = readText(relPath).trim();
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text);
  } catch (error) {
    return { __parseError: error.message };
  }
}

function parseJSONStream(text) {
  const values = [];
  let start = -1;
  let depth = 0;
  let inString = false;
  let escaped = false;

  for (let i = 0; i < text.length; i += 1) {
    const ch = text[i];
    if (start === -1) {
      if (ch === '{' || ch === '[') {
        start = i;
        depth = 1;
      }
      continue;
    }

    if (inString) {
      if (escaped) {
        escaped = false;
      } else if (ch === '\\') {
        escaped = true;
      } else if (ch === '"') {
        inString = false;
      }
      continue;
    }

    if (ch === '"') {
      inString = true;
    } else if (ch === '{' || ch === '[') {
      depth += 1;
    } else if (ch === '}' || ch === ']') {
      depth -= 1;
      if (depth === 0) {
        const chunk = text.slice(start, i + 1);
        try {
          const parsed = JSON.parse(chunk);
          if (Array.isArray(parsed)) {
            values.push(...parsed);
          } else {
            values.push(parsed);
          }
        } catch {
          // Scanner output may contain status lines; ignore unparseable chunks.
        }
        start = -1;
      }
    }
  }

  return values;
}

function readExit(name) {
  const raw = readText(path.join(reportsDir, `${name}.exit`)).trim();
  if (!raw) {
    return null;
  }
  const parsed = Number.parseInt(raw, 10);
  return Number.isNaN(parsed) ? null : parsed;
}

function listReportFiles(pattern) {
  const absDir = path.join(repoRoot, reportsDir);
  if (!existsSync(absDir)) {
    return [];
  }
  return readdirSync(absDir)
    .filter((entry) => pattern.test(entry))
    .sort();
}

function shortText(value, max = 240) {
  if (!value) {
    return '';
  }
  const compact = String(value).replace(/\s+/g, ' ').trim();
  return compact.length > max ? `${compact.slice(0, max - 3)}...` : compact;
}

function run(command, args, options = {}) {
  return spawnSync(command, args, {
    cwd: options.cwd || repoRoot,
    encoding: 'utf8',
    maxBuffer: 128 * 1024 * 1024,
    stdio: options.stdio || ['ignore', 'pipe', 'pipe'],
  });
}

function listModules() {
  const result = run('go', ['list', '-m', '-json', 'all'], {
    cwd: path.join(repoRoot, moduleDir),
  });
  const modules = new Map();
  if (result.status !== 0) {
    return { modules, error: result.stderr || result.stdout || 'go list failed' };
  }

  for (const mod of parseJSONStream(result.stdout)) {
    if (mod?.Path) {
      modules.set(mod.Path, {
        path: mod.Path,
        version: mod.Version || '',
        replacement: mod.Replace?.Version || mod.Replace?.Path || '',
      });
    }
  }
  return { modules, error: '' };
}

function normalizeGoVersion(version) {
  if (!version) {
    return '';
  }
  let value = String(version).trim();
  if (!value || value === '<nil>') {
    return '';
  }
  if (value.includes('@')) {
    value = value.slice(value.lastIndexOf('@') + 1);
  }
  if (/^\d+\.\d+\.\d+/.test(value)) {
    return `v${value}`;
  }
  return value;
}

function splitFixedVersions(value) {
  if (!value) {
    return [];
  }
  return String(value)
    .split(/[\s,]+/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function parseSemver(version) {
  if (!version) {
    return null;
  }
  const match = String(version)
    .replace(/\+incompatible$/, '')
    .match(/^v?(\d+)\.(\d+)\.(\d+)(?:[-+](.+))?$/);
  if (!match) {
    return null;
  }
  return {
    major: Number.parseInt(match[1], 10),
    minor: Number.parseInt(match[2], 10),
    patch: Number.parseInt(match[3], 10),
    suffix: match[4] || '',
  };
}

function compareVersions(left, right) {
  if (left === right) {
    return 0;
  }
  const a = parseSemver(left);
  const b = parseSemver(right);
  if (!a || !b) {
    return String(left).localeCompare(String(right));
  }
  for (const key of ['major', 'minor', 'patch']) {
    if (a[key] !== b[key]) {
      return a[key] - b[key];
    }
  }
  if (a.suffix === b.suffix) {
    return 0;
  }
  if (!a.suffix) {
    return 1;
  }
  if (!b.suffix) {
    return -1;
  }
  return a.suffix.localeCompare(b.suffix);
}

function maxVersion(versions) {
  return versions.reduce((best, candidate) => {
    if (!best) {
      return candidate;
    }
    return compareVersions(candidate, best) > 0 ? candidate : best;
  }, '');
}

function versionAtLeast(current, fixed) {
  if (!current || !fixed) {
    return false;
  }
  if (current === fixed) {
    return true;
  }
  return compareVersions(current, fixed) >= 0;
}

function deriveFixedVersionFromOSV(osv, modulePath) {
  if (!osv?.affected) {
    return '';
  }
  const fixed = [];
  for (const affected of osv.affected) {
    const packageName = affected.package?.name || affected.package?.Name || '';
    if (modulePath && packageName && packageName !== modulePath) {
      continue;
    }
    for (const range of affected.ranges || []) {
      for (const event of range.events || []) {
        if (event.fixed) {
          fixed.push(normalizeGoVersion(event.fixed));
        }
      }
    }
  }
  return maxVersion(fixed);
}

function deriveModuleFromOSV(osv) {
  const affected = osv?.affected || [];
  if (affected.length !== 1) {
    return '';
  }
  return affected[0].package?.name || affected[0].package?.Name || '';
}

function addFinding(findings, finding) {
  if (!finding.vulnerabilityID || !finding.packageName) {
    return;
  }
  const key = [
    finding.vulnerabilityID,
    finding.packageName,
    finding.installedVersion || '',
    finding.fixedVersion || '',
  ].join('|');
  const existing = findings.get(key);
  if (existing) {
    existing.sources.add(finding.source);
    if (finding.target) existing.targets.add(finding.target);
    if (finding.type) existing.types.add(finding.type);
    if (finding.severity && severityRank.get(finding.severity) > severityRank.get(existing.severity || 'UNKNOWN')) {
      existing.severity = finding.severity;
    }
    if (!existing.title && finding.title) {
      existing.title = finding.title;
    }
    return;
  }

  findings.set(key, {
    ...finding,
    sources: new Set([finding.source]),
    targets: new Set(finding.target ? [finding.target] : []),
    types: new Set(finding.type ? [finding.type] : []),
    fixed: false,
  });
}

function parseGovulncheck(findings, modulesBefore) {
  const messages = parseJSONStream(readText(path.join(reportsDir, 'govulncheck.json')));
  const osvByID = new Map();
  for (const message of messages) {
    if (message?.osv) {
      const id = message.osv.id || message.osv.ID;
      if (id) {
        osvByID.set(id, message.osv);
      }
    }
  }

  for (const message of messages) {
    const finding = message?.finding;
    if (!finding) {
      continue;
    }
    const vulnerabilityID = finding.osv || finding.osv_id || finding.id || finding.ID;
    if (!vulnerabilityID) {
      continue;
    }
    const osv = osvByID.get(vulnerabilityID);
    const trace = Array.isArray(finding.trace) ? finding.trace : [];
    const moduleFrame = trace.find((frame) => frame.module && frame.module !== 'stdlib')
      || trace.find((frame) => frame.module)
      || null;
    const packageName = moduleFrame?.module || deriveModuleFromOSV(osv) || 'unknown';
    const installedVersion = moduleFrame?.version || modulesBefore.get(packageName)?.version || '';
    const fixedVersion = normalizeGoVersion(
      finding.fixed_version || finding.fixedVersion || deriveFixedVersionFromOSV(osv, packageName),
    );

    addFinding(findings, {
      source: 'govulncheck',
      target: 'src/go.mod',
      type: packageName === 'stdlib' ? 'stdlib' : 'gomod',
      vulnerabilityID,
      packageName,
      installedVersion,
      fixedVersion,
      severity: 'UNKNOWN',
      title: shortText(osv?.summary || osv?.details || ''),
    });
  }
}

function parseTrivy(reportName, source, findings) {
  const report = parseJSONFile(path.join(reportsDir, reportName));
  if (!report || report.__parseError) {
    return;
  }

  for (const result of report.Results || []) {
    const vulnerabilities = result.Vulnerabilities || [];
    for (const vulnerability of vulnerabilities) {
      const fixedVersion = maxVersion(
        splitFixedVersions(vulnerability.FixedVersion).map((version) => (
          result.Type === 'gomod' ? normalizeGoVersion(version) : version
        )),
      );
      addFinding(findings, {
        source,
        target: result.Target || report.ArtifactName || '',
        type: result.Type || '',
        vulnerabilityID: vulnerability.VulnerabilityID || vulnerability.ID || '',
        packageName: vulnerability.PkgName || vulnerability.PkgID || 'unknown',
        installedVersion: vulnerability.InstalledVersion || '',
        fixedVersion,
        severity: vulnerability.Severity || 'UNKNOWN',
        title: shortText(vulnerability.Title || vulnerability.Description || ''),
      });
    }
  }
}

function isActionableGoFinding(finding, modulesBefore) {
  if (!finding.fixedVersion || !modulesBefore.has(finding.packageName)) {
    return false;
  }
  if (finding.packageName === 'stdlib' || finding.packageName === 'toolchain') {
    return false;
  }
  return finding.sources.has('govulncheck') || finding.types.has('gomod');
}

function buildUpdatePlan(findings, modulesBefore) {
  const plan = new Map();
  for (const finding of findings.values()) {
    if (!isActionableGoFinding(finding, modulesBefore)) {
      continue;
    }
    const fixedVersions = splitFixedVersions(finding.fixedVersion).map(normalizeGoVersion).filter(Boolean);
    const targetVersion = maxVersion(fixedVersions);
    if (!targetVersion) {
      continue;
    }
    const currentVersion = modulesBefore.get(finding.packageName)?.version || '';
    if (currentVersion && versionAtLeast(currentVersion, targetVersion)) {
      continue;
    }
    const existing = plan.get(finding.packageName);
    if (!existing) {
      plan.set(finding.packageName, {
        module: finding.packageName,
        fromVersion: currentVersion,
        targetVersion,
        vulnerabilities: new Set([finding.vulnerabilityID]),
      });
      continue;
    }
    existing.targetVersion = maxVersion([existing.targetVersion, targetVersion]);
    existing.vulnerabilities.add(finding.vulnerabilityID);
  }
  return [...plan.values()].sort((a, b) => a.module.localeCompare(b.module));
}

function applyUpdates(plan) {
  const actions = [];
  let attempted = false;
  for (const item of plan) {
    if (!applyRemediation) {
      actions.push({ ...item, vulnerabilities: [...item.vulnerabilities], status: 'planned', toVersion: '' });
      continue;
    }
    attempted = true;
    const result = run('go', ['get', `${item.module}@${item.targetVersion}`], {
      cwd: path.join(repoRoot, moduleDir),
      stdio: 'inherit',
    });
    actions.push({
      ...item,
      vulnerabilities: [...item.vulnerabilities],
      status: result.status === 0 ? 'updated' : 'failed',
      toVersion: '',
    });
  }

  if (attempted) {
    const tidy = run('go', ['mod', 'tidy'], {
      cwd: path.join(repoRoot, moduleDir),
      stdio: 'inherit',
    });
    if (tidy.status !== 0) {
      actions.push({
        module: 'go mod tidy',
        fromVersion: '',
        targetVersion: '',
        toVersion: '',
        vulnerabilities: [],
        status: 'failed',
      });
    }
  }
  return actions;
}

function finalizeFindings(findings, modulesAfter, actions) {
  const actionByModule = new Map(actions.map((action) => [action.module, action]));
  for (const finding of findings.values()) {
    const moduleAfter = modulesAfter.get(finding.packageName)?.version || '';
    const action = actionByModule.get(finding.packageName);
    if (moduleAfter && finding.fixedVersion && versionAtLeast(moduleAfter, normalizeGoVersion(finding.fixedVersion))) {
      finding.fixed = true;
    } else if (action?.status === 'updated' && moduleAfter) {
      finding.fixed = versionAtLeast(moduleAfter, action.targetVersion);
    }
    finding.afterVersion = moduleAfter;
    finding.sources = [...finding.sources].sort();
    finding.targets = [...finding.targets].sort();
    finding.types = [...finding.types].sort();
  }

  for (const action of actions) {
    action.toVersion = modulesAfter.get(action.module)?.version || action.toVersion || '';
  }
}

function scannerStatuses() {
  const statuses = [
    { name: 'govulncheck', exitCode: readExit('govulncheck'), stderr: readText(path.join(reportsDir, 'govulncheck.stderr')) },
    { name: 'trivy fs', exitCode: readExit('trivy-fs'), stderr: readText(path.join(reportsDir, 'trivy-fs.stderr')) },
  ];

  const imageExitFiles = listReportFiles(/^trivy-image(?:-\d+)?\.exit$/);
  for (const file of imageExitFiles) {
    const suffix = file.replace(/^trivy-image/, '').replace(/\.exit$/, '');
    const index = Number.parseInt(suffix.replace('-', ''), 10);
    const label = Number.isNaN(index) ? imageRef : imageRefs[index - 1] || imageRef;
    statuses.push({
      name: `trivy image: ${label}`,
      exitCode: readExit(file.replace(/\.exit$/, '')),
      stderr: readText(path.join(reportsDir, file.replace(/\.exit$/, '.stderr'))),
    });
  }

  return statuses;
}

function md(value) {
  const text = value === undefined || value === null || value === '' ? '-' : String(value);
  return text.replace(/\|/g, '\\|').replace(/\r?\n/g, '<br>');
}

function code(value) {
  if (!value) {
    return '-';
  }
  return `\`${md(value).replace(/`/g, '')}\``;
}

function sortedFindings(findings) {
  return [...findings].sort((a, b) => {
    const severityDiff = (severityRank.get(b.severity || 'UNKNOWN') || 0) - (severityRank.get(a.severity || 'UNKNOWN') || 0);
    if (severityDiff !== 0) {
      return severityDiff;
    }
    return `${a.packageName}${a.vulnerabilityID}`.localeCompare(`${b.packageName}${b.vulnerabilityID}`);
  });
}

function renderVulnerabilityTable(findings) {
  const rows = sortedFindings(findings);
  if (rows.length === 0) {
    return 'No vulnerabilities were reported by the completed scanners.\n';
  }
  const visible = rows.slice(0, maxTableRows);
  const lines = [
    '| Vulnerability | Package | Present version | Fixed version | Fixed |',
    '| --- | --- | --- | --- | --- |',
  ];
  for (const finding of visible) {
    const label = finding.title ? `${finding.vulnerabilityID}<br>${md(finding.title)}` : finding.vulnerabilityID;
    lines.push([
      md(label),
      code(finding.packageName),
      code(finding.installedVersion),
      code(finding.fixedVersion),
      finding.fixed ? ':white_check_mark:' : ':x:',
    ].join(' | ').replace(/^/, '| ').replace(/$/, ' |'));
  }
  if (rows.length > visible.length) {
    lines.push('');
    lines.push(`_Showing ${visible.length} of ${rows.length} findings. See workflow artifacts for full JSON reports._`);
  }
  return `${lines.join('\n')}\n`;
}

function renderActions(actions) {
  if (actions.length === 0) {
    return 'No Go module updates were needed or possible from the scanner fixed-version data.\n';
  }
  const lines = [
    '| Module | From | Target | After | Status |',
    '| --- | --- | --- | --- | --- |',
  ];
  for (const action of actions) {
    lines.push(`| ${code(action.module)} | ${code(action.fromVersion)} | ${code(action.targetVersion)} | ${code(action.toVersion)} | ${md(action.status)} |`);
  }
  return `${lines.join('\n')}\n`;
}

function renderScannerStatus(statuses) {
  const lines = [
    '| Scanner | Exit code | Notes |',
    '| --- | --- | --- |',
  ];
  for (const status of statuses) {
    const failed = status.exitCode !== null && status.exitCode !== 0;
    lines.push(`| ${md(status.name)} | ${status.exitCode ?? '-'} | ${failed ? md(shortText(status.stderr, 500)) : '-'} |`);
  }
  return `${lines.join('\n')}\n`;
}

function renderPRBody(summary, findings, actions, statuses) {
  return `## Summary

Automated zero-CVE dependency remediation from the daily scanner workflow.

## Vulnerabilities

${renderVulnerabilityTable(findings)}
## Go Module Updates

${renderActions(actions)}
## Scan Scope

- Repository: \`trivy fs .\`
- Go dependencies: \`govulncheck ./...\`
- Images: ${imageRefs.map((ref) => `\`${md(ref)}\``).join(', ')}

## Scanner Status

${renderScannerStatus(statuses)}
## Type of Change

- [x] Bug fix (\`fix:\`)
- [x] Dependencies update (\`chore:\`)

## Testing

- [x] \`trivy fs\` report generated
- [x] \`govulncheck\` report generated
- [x] \`trivy image\` report generated for configured image refs
- [x] \`go mod tidy\` run when Go dependency updates were applied

<!-- zero-cve-summary: fixed=${summary.fixedFindingCount}; remaining=${summary.remainingFindingCount}; scanners=${summary.scannerErrorCount} -->
`;
}

function renderIssueBody(summary, findings, actions, statuses) {
  return `# Zero CVE Daily Report

Generated: \`${generatedAt}\`

## Status

- Total findings: ${summary.findingCount}
- Fixed by Go dependency remediation: ${summary.fixedFindingCount}
- Remaining findings: ${summary.remainingFindingCount}
- Scanner errors: ${summary.scannerErrorCount}
- Rolling PR branch: \`${summary.branch}\`
- Scanned images: ${imageRefs.map((ref) => `\`${md(ref)}\``).join(', ')}

## Vulnerabilities

${renderVulnerabilityTable(findings)}
## Go Module Updates

${renderActions(actions)}
## Scanner Status

${renderScannerStatus(statuses)}
`;
}

const modulesBeforeResult = listModules();
const findingsByKey = new Map();
parseGovulncheck(findingsByKey, modulesBeforeResult.modules);
parseTrivy('trivy-fs.json', 'trivy-fs', findingsByKey);
for (const imageReport of listReportFiles(/^trivy-image(?:-\d+)?\.json$/)) {
  parseTrivy(imageReport, 'trivy-image', findingsByKey);
}

const updatePlan = buildUpdatePlan(findingsByKey, modulesBeforeResult.modules);
const actions = applyUpdates(updatePlan);
const modulesAfterResult = listModules();
finalizeFindings(findingsByKey, modulesAfterResult.modules, actions);

const findings = sortedFindings([...findingsByKey.values()]);
const statuses = scannerStatuses();
const scannerErrorCount = statuses.filter((status) => status.exitCode !== null && status.exitCode !== 0).length
  + (modulesBeforeResult.error ? 1 : 0)
  + (modulesAfterResult.error ? 1 : 0);
const fixedFindingCount = findings.filter((finding) => finding.fixed).length;
const remainingFindingCount = findings.length - fixedFindingCount;

const summary = {
  generatedAt,
  imageRef,
  imageRefs,
  branch: process.env.ZERO_CVE_BRANCH || 'automation/zero-cve',
  findingCount: findings.length,
  fixedFindingCount,
  remainingFindingCount,
  scannerErrorCount,
  moduleListErrors: [modulesBeforeResult.error, modulesAfterResult.error].filter(Boolean),
  actions,
  findings,
  statuses: statuses.map((status) => ({
    name: status.name,
    exitCode: status.exitCode,
    stderr: status.exitCode ? shortText(status.stderr, 1000) : '',
  })),
};

writeReport('summary.json', `${JSON.stringify(summary, null, 2)}\n`);
writeReport('pr-body.md', renderPRBody(summary, findings, actions, statuses));
writeReport('issue-body.md', renderIssueBody(summary, findings, actions, statuses));

console.log(`Zero CVE report: ${findings.length} findings, ${fixedFindingCount} fixed, ${remainingFindingCount} remaining, ${scannerErrorCount} scanner errors.`);
