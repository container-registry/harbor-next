import { test, expect } from '@playwright/test';
import { execFileSync } from 'child_process';

// variables
const LOCAL_REGISTRY: string = process.env.LOCAL_REGISTRY || 'registry.goharbor.io';
const LOCAL_REGISTRY_NAMESPACE: string = process.env.LOCAL_REGISTRY_NAMESPACE || 'harbor-ci';
const ip: string = requiredEnv('IP');
const user: string = process.env.HARBOR_ADMIN || 'admin';
const pwd: string = process.env.HARBOR_PASSWORD || 'Harbor12345';

test('login and scan the things', async ({ page }) => {
  test.setTimeout(60 * 60 * 1000); // 60 minutes
const tag = 'v2.2.0';
const digest = 'sha256:7c3f03db32f9a89b47faedb69cb6ea10741cec203ec76eb45add65e58baa2a82';
const d = '1';
const index_repo = `index${d}`;

// now we need to tag and push these images to harbor.
  const images = [
  'goharbor/harbor-log-base',
  'goharbor/harbor-prepare-base',
  'goharbor/harbor-redis-base',
  'goharbor/harbor-nginx-base',
  'goharbor/harbor-registry-base'
];

const project: string = 'aproject-'+ Date.now();
    // login
    await page.goto('/');
    await page.getByRole('textbox', { name: 'Username' }).click();
    await page.getByRole('textbox', { name: 'Username' }).fill(user);
    await page.getByRole('textbox', { name: 'Password' }).click();
    await page.getByRole('textbox', { name: 'Password' }).fill(pwd);
    await page.getByRole('button', { name: 'LOG IN' }).click();

  // create project
  await page.getByRole('button', { name: 'New Project' }).click();
  await page.locator('#create_project_name').click();
  await page.locator('#create_project_name').fill(project);
  await page.getByRole('button', { name: 'OK' }).click();

for (const image of images) {

  pushImageWithTag(ip, user, pwd, project, image, tag, tag);

  // go into project repo page to verify the image is there.
  await page.getByRole('link', { name: project }).click();
  await page.getByRole('link', { name: project + '/' + image }).click();


  // scan the repo
//   const tagname = process.env.TAG_NAME || 'v2.2.0';
// const row = page.getByRole('row', { name: new RegExp(tagname) });
await page.waitForTimeout(1000);
await page.getByRole('gridcell', { name: 'Select Select' }).locator('label').click();
await page.waitForTimeout(1000);
await page.getByRole('checkbox', { name: 'Select', exact: true }).check();
await page.waitForTimeout(5000);
await page.getByRole('button', { name: 'Scan vulnerability' }).click();
await page.getByRole('gridcell', { name: /Total/ }).waitFor();


await page.goto('/');
}

  // // get back to the project page
  // await page.getByText(project).click();


  const output = runCommand(
    './e2e/scripts/docker_push_manifest_list.sh',
    [
      ip,
      user,
      `${ip}/${project}/${index_repo}:${tag}`,
      `${ip}/${project}/${images[0]}:${tag}`,
      `${ip}/${project}/${images[1]}:${tag}`,
    ],
    { input: `${pwd}\n` }
  );

  expect(output).not.toContain('Error');

// delete first two repos
for (let i = 0; i < 2; i++) {
  const image = images[i];
    await page.goto('/');
    await page.getByRole('link', { name: project }).click();
    await page.locator('.refresh-btn > clr-icon').click();

const rowRegex = new RegExp(`Select\\s+Select\\s+${project}/${image}`, 'i');

// Wait for row to appear and click its checkbox (label)
const row = page.getByRole('row', { name: rowRegex });
await row.waitFor({ state: 'visible', timeout: 10000 }); // ensure it's visible
await row.locator('label').click();
await page.waitForTimeout(5000);
await page.getByRole('button', { name: 'Delete' }).click();
await page.getByRole('button', { name: 'DELETE', exact: true }).click();
}

// go into the index repo and scan the manifest list
await page.getByRole('link', { name: project + '/' + index_repo }).click();
//scan the repo
await page.waitForTimeout(1000);
await page.getByRole('gridcell', { name: 'Select Select' }).locator('label').click();
await page.waitForTimeout(1000);
await page.getByRole('checkbox', { name: 'Select', exact: true }).check();
await page.waitForTimeout(5000);
await page.getByRole('button', { name: 'Scan vulnerability' }).click();
await page.getByRole('gridcell', { name: /Total/ }).waitFor();

// go to security hub
await page.getByRole('link', { name: 'Interrogation Services' }).click();
await page.getByRole('link', { name: 'Security Hub' }).click();
// await page.getByRole('button', { name: 'SEARCH' }).click();

// get vuln summary from api and compare it with the ui display
const summary = await getVulnerabilitySummaryFromAPI(ip, user, pwd);
console.log('Vulnerability Summary from API:', summary);
  // Map expected counts
  const expectedCounts = [
    summary.critical_cnt, // 1st div
    summary.high_cnt,     // 2nd div
    summary.medium_cnt,   // 3rd div
    summary.low_cnt,      // 4th div
    summary.unknown_cnt,                    // 5th div
    0,                    // 6th div
  ];

  // Loop through and verify UI elements
  for (let i = 0; i < expectedCounts.length - 2; i++) {
    await page.waitForTimeout(1000);
    console.log(`Verifying count for severity index ${i}: Expected ${expectedCounts[i]}`);
    await expect(page.locator('app-vulnerability-summary')).toContainText(`${expectedCounts[i]}`);
  }

  // check the top 5 dangerous artifacts
  const dangerousArtifacts = summary.dangerous_artifacts;

await page.waitForTimeout(2000);
for (const artifact of dangerousArtifacts) {
  // repository name
  await expect(page.locator('app-vulnerability-summary')).toContainText(artifact.repository_name);

  // shortened digest (first few chars for parity with UI)
  const shortDigest = artifact.digest.slice(0, 14); // e.g. sha256:f4215ab2
  await expect(page.locator('app-vulnerability-summary')).toContainText(shortDigest);

  // log for debug visibility
  console.log(`Verified artifact: ${artifact.repository_name} (${shortDigest})`);
}
await page.waitForTimeout(2000);

// // check the top 5 dangerous CVEs
const dangerousCVEs = summary.dangerous_cves;
console.log('Dangerous CVEs from API:', dangerousCVEs);

const filterCVE = dangerousCVEs.find(cve =>
  cve.cve_id &&
  cve.package &&
  cve.cvss_score_v3 !== undefined &&
  cve.cvss_score_v3 !== null &&
  cve.severity &&
  cve.description
);
if (!filterCVE) {
  throw new Error('Expected at least one dangerous CVE with complete filter fields');
}

const cveID = filterCVE.cve_id;
const packageName = filterCVE.package;
const cvssScore = String(filterCVE.cvss_score_v3);
const severity = filterCVE.severity;
const cveDescription = filterCVE.description;

for (const cve of dangerousCVEs) {
  const pkgVersion = `${cve.package}@${cve.version}`;

  // Check dynamically for each CVE's values.
  await expect(page.locator('app-vulnerability-summary')).toContainText(cve.cve_id);
  await expect(page.locator('app-vulnerability-summary')).toContainText(cve.severity);
  await expect(page.locator('app-vulnerability-summary')).toContainText(String(cve.cvss_score_v3));
  await expect(page.locator('app-vulnerability-summary')).toContainText(pkgVersion);

  console.log(`Verified CVE ${cve.cve_id} (${cve.severity}) - ${pkgVersion}`);
}

// check the quick search
  // select the first repo name on the right side - dangerous artifacts
  const firstDangerousArtifact = dangerousArtifacts[0];
  if (!firstDangerousArtifact) {
    throw new Error('Expected at least one dangerous artifact in the vulnerability summary');
  }
  await page.locator('app-vulnerability-summary').getByRole('link', { name: firstDangerousArtifact.repository_name }).first().click();
  // check if the below element got the right repo and digest
  console.log('Checking quick search values for repo:', dangerousCVEs[0].repository_name);
  console.log('Checking quick search values for repo:', summary.dangerous_cves[0].repository_name);
  console.log('checking value of dangerous arts:', firstDangerousArtifact.repository_name);

  await expect(page.locator('app-vulnerability-filter form div').filter({ hasText: 'Filter by All Repository Name' }).getByRole('textbox')).toHaveValue(firstDangerousArtifact.repository_name);
  await expect(page.getByRole('textbox').nth(2)).toHaveValue(firstDangerousArtifact.digest);
  // check if the table shows the right info
  await page.locator('.datagrid-inner-wrapper').click();
  await page.waitForTimeout(3000);
  // await expect(page.locator('#clr-dg-row33')).toContainText('CVE-2021-37600');
  await expect(page.getByText(firstDangerousArtifact.repository_name).nth(2)).toBeVisible(); // this works no need for fuzzy i guess
  // const repo = summary.dangerous_cves[0].cve_id;
  // // const repo = summary.dangerous_artifacts[0].repository_name;:w

  // // create fuzzy version (partial match)
  // const fuzzyRepo = new RegExp(repo.replace(/[.*+?^${}()|[\]\\]/g, ''), 'i');

  // // check visibility using fuzzy matching
  // await expect(page.getByText(fuzzyRepo).first()).toBeVisible();

  // await expect(page.locator('#clr-dg-row33')).toContainText('CVE-2021-37600');
  // await expect(page.locator('#clr-dg-row33')).toContainText(summary.dangerous_artifacts[0].repository_name);
  await page.waitForTimeout(3000);

  await expect(page.getByRole('gridcell', { name: firstDangerousArtifact.repository_name }).first()).toBeVisible();
  await expect(page.getByRole('gridcell', { name: firstDangerousArtifact.digest.substring(0, 12) }).first()).toBeVisible();
  // await expect(page.getByRole('gridcell', { name: summary.dangerous_cves[0].version }).first()).toBeVisible();
  // await expect(page.getByRole('gridcell', { name: summary.dangerous_cves[0].cvss_score_v3.toString() }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row33')).toContainText(summary.dangerous_artifacts[0].digest.substring(0, 12));
  // previously the below 2 lines were using cves
  // await expect(page.locator('#clr-dg-row33')).toContainText(summary.dangerous_cves[0].version);
  // await expect(page.locator('#clr-dg-row33')).toContainText(summary.dangerous_cves[0].cvss_score_v3.toString());
 await page.waitForTimeout(2000);

 // check for the cve id
//  // ---  remove this once test passes -- start
//   await page.goto('/');
//   // login
//   await page.goto('/');
//   await page.getByRole('textbox', { name: 'Username' }).click();
//   await page.getByRole('textbox', { name: 'Username' }).fill('admin');
//   await page.getByRole('textbox', { name: 'Password' }).click();
//   await page.getByRole('textbox', { name: 'Password' }).fill('Harbor12345');
//   await page.getByRole('button', { name: 'LOG IN' }).click();
//   await page.getByRole('link', { name: 'Interrogation Services' }).click();
//   // --- remove this once test passes -- end

  // search for dangerous cves
  await page.getByRole('link', { name: cveID }).first().click();
  await page.waitForTimeout(2000);
  // await page.getByText('Top 5 Most Dangerous CVEs CVE').click();
  await page.getByText('Top 5 Most Dangerous CVEs').click();
  const value = await page.locator('div:nth-child(3) > .card-block > div > div').first();
  // console.log("what the hell", value.textContent());
  // console.log("what is this", value);

  // TODO: this should be dynamic
  await page.getByRole('link', { name: cveID }).first().click();
  await expect(page.locator('app-vulnerability-filter').getByRole('textbox')).toHaveValue(cveID);
  // await page.locator('.datagrid-inner-wrapper').click();
  await expect(page.locator('app-vulnerability-filter').getByRole('combobox')).toHaveValue('cve_id');
  // await page.locator('.datagrid').click();
  await expect(page.getByRole('gridcell', { name: cveID }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row31')).toContainText(dangerousCVEs[0].cve_id);
  await page.waitForTimeout(2000);
  // quick search done ---

  // check the search by one condition
  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('project_id');
  await page.locator('app-vulnerability-filter').getByRole('textbox').dblclick();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(project);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  await expect(page.getByRole('gridcell', { name: project }).first()).toBeVisible();

  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('repository_name');
  await page.locator('app-vulnerability-filter').getByRole('textbox').click();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(`${project}/${images[2]}`);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  // await expect(page.locator('#clr-dg-row58')).toContainText(`${project}/${images[2]}`);
  await expect(page.getByRole('gridcell', { name: `${project}/${images[2]}` }).first()).toBeVisible();

  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('digest');
  await page.locator('app-vulnerability-filter').getByRole('textbox').click();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(digest);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  const shortDigest = digest.slice(0, 14);
  await expect(page.getByRole('gridcell', { name: `${shortDigest}` }).first()).toBeVisible();

  // search by cve id
  await page.waitForTimeout(2000);
  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('cve_id');
  await page.locator('app-vulnerability-filter').getByRole('textbox').click();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(cveID);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  // await expect(page.locator('#clr-dg-row118')).toContainText('CVE-2022-29155');
  await expect(page.getByRole('gridcell', { name: cveID }).first()).toBeVisible();

  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('package');
  await page.locator('app-vulnerability-filter').getByRole('textbox').click();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(packageName);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  await expect(page.getByRole('gridcell', { name: packageName }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row129')).toContainText('curl');

  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('tag');
  await page.locator('app-vulnerability-filter').getByRole('textbox').click();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(tag);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  await expect(page.getByRole('gridcell', { name: tag }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row144')).toContainText('v2.2.0');
  // await page.getByText('CVE-2022-32207aproject-1764110949942/goharbor/harbor-redis-basesha256:').click();
  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('cvss_score_v3');
  await page.getByRole('textbox').nth(1).click();
  await page.getByRole('textbox').nth(1).fill(cvssScore);
  await page.getByRole('textbox').nth(2).click();
  await page.getByRole('textbox').nth(2).fill(cvssScore);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  // await expect(page.locator('#clr-dg-row159')).toContainText('7.5');
  await expect(page.getByRole('gridcell', { name: cvssScore }).first()).toBeVisible();

  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('severity');
  await page.getByRole('combobox').nth(1).selectOption('Critical');
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  if (summary.critical_cnt > 1000) {
    await expect(page.locator('clr-dg-footer')).toContainText('1000+ CVEs');
  } else {
    await expect(page.locator('clr-dg-footer')).toContainText(summary.critical_cnt + ' CVEs');
  }
  await expect(page.getByRole('gridcell', { name: 'Critical' }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row174')).toContainText('Critical');
  await page.getByRole('combobox').nth(1).selectOption('High');
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  if (summary.high_cnt > 1000) {
    await expect(page.locator('clr-dg-footer')).toContainText('1000+ CVEs');
  } else {
    await expect(page.locator('clr-dg-footer')).toContainText(summary.high_cnt + ' CVEs');
  }
  // await expect(page.locator('clr-dg-footer')).toContainText(summary.high_cnt + ' CVEs');
  await expect(page.getByRole('gridcell', { name: 'High' }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row189')).toContainText('High');
  await page.getByRole('combobox').nth(1).selectOption('Medium');
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  if (summary.medium_cnt > 1000) {
    await expect(page.locator('clr-dg-footer')).toContainText('1000+ CVEs');
  } else {
    await expect(page.locator('clr-dg-footer')).toContainText(summary.medium_cnt + ' CVEs');
  }
  await expect(page.getByRole('gridcell', { name: 'Medium' }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row204')).toContainText('Medium');
  await page.getByRole('combobox').nth(1).selectOption('Low');
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  if (summary.low_cnt > 1000) {
    await expect(page.locator('clr-dg-footer')).toContainText('1000+ CVEs');
  } else {
    await expect(page.locator('clr-dg-footer')).toContainText(summary.low_cnt + ' CVEs');
  }
  await expect(page.getByRole('gridcell', { name: 'Low' }).first()).toBeVisible();
  // await expect(page.locator('#clr-dg-row219')).toContainText('Low');
  await page.getByRole('combobox').nth(1).selectOption('Unknown');
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  await expect(page.locator('clr-dg-placeholder')).toContainText('We could not find any vulnerability');
  await page.getByRole('combobox').nth(1).selectOption('None');
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.waitForTimeout(2000);
  await expect(page.locator('clr-dg-placeholder')).toContainText('We could not find any vulnerability');

  // search by multiple conditions
  await page.getByRole('combobox').first().selectOption('project_id');
  await page.locator('app-vulnerability-filter').getByRole('textbox').dblclick();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(project);
  await expect(page.locator('app-vulnerability-filter clr-icon').nth(2)).toBeVisible();
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.getByRole('combobox').nth(1).selectOption('repository_name');
  await page.getByRole('textbox').nth(2).dblclick();
  await page.getByRole('textbox').nth(2).fill(`${project}/${images[2]}`);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.getByRole('combobox').nth(2).selectOption('digest');
  await page.getByRole('textbox').nth(3).fill(digest);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.locator('div:nth-child(4) > .clr-control-container > div > .clr-select').first().selectOption('cve_id');
  await page.locator('app-vulnerability-filter').getByRole('textbox').nth(3).dblclick();
  await page.locator('app-vulnerability-filter').getByRole('textbox').nth(3).fill(cveID);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.locator('div:nth-child(5) > .clr-control-container > div > .clr-select').first().selectOption('package');
  await page.locator('app-vulnerability-filter').getByRole('textbox').nth(4).dblclick();
  await page.locator('app-vulnerability-filter').getByRole('textbox').nth(4).fill(packageName);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.locator('div:nth-child(6) > .clr-control-container > div > .clr-select').first().selectOption('tag');
  await page.locator('.clr-input.ng-untouched').dblclick();
  await page.locator('.clr-input.ng-untouched').fill(tag);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.locator('div:nth-child(7) > .clr-control-container > div > .clr-select').first().selectOption('cvss_score_v3');
  await page.locator('div').filter({ hasText: /^FromTo$/ }).getByRole('textbox').first().dblclick();
  await page.locator('div').filter({ hasText: /^FromTo$/ }).getByRole('textbox').first().fill(cvssScore);
  await page.locator('div').filter({ hasText: /^FromTo$/ }).getByRole('textbox').nth(1).dblclick();
  await page.locator('div').filter({ hasText: /^FromTo$/ }).getByRole('textbox').nth(1).fill(cvssScore);
  await page.locator('app-vulnerability-filter clr-icon').nth(1).click();
  await page.locator('.clr-select-wrapper.ml-1 > .clr-select').selectOption(severity);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  // await expect(page.locator('#clr-dg-row281')).toContainText('High');
  await expect(page.getByRole('gridcell', { name: severity }).first()).toBeVisible();

  await page.getByRole('button', { name: 'Open' }).first().click();
  // await expect(page.locator('#clr-dg-expandable-row-281')).toContainText('Description: libcurl-using applications can ask for a specific client certificate to be used in a transfer');
  // await page.getByRole('gridcell', { name: 'Close' }).click();
  await expect(page.locator('clr-datagrid')).toContainText(cveDescription);

// do page jump test
  // await page.getByRole('link', { name: 'Projects' }).click();
  // await page.getByRole('link', { name: 'Interrogation Services' }).click();
  // await page.getByText('CVE-2022-22823').click();
  await page.getByRole('link', { name: cveID }).first().click();
  await page.waitForTimeout(2000);

  // do repo jump test
  await page.goto('/');
  await page.getByRole('link', { name: 'Interrogation Services' }).click();
  // await page.getByRole('link', { name: 'Security Hub' }).click();
  // await page.locator('#clr-dg-row317').getByRole('link', { name: 'aproject-1764149771585/' }).click();
  // await expect(page.locator('h2')).toContainText('goharbor/harbor-registry-base');
  const indexArtifact = dangerousArtifacts.find(artifact => artifact.repository_name === `${project}/${index_repo}`);
  if (!indexArtifact) {
    throw new Error(`Expected dangerous artifact for ${project}/${index_repo}`);
  }
  await expect(page.getByRole('link', { name: indexArtifact.repository_name }).last()).toBeVisible();
  await page.getByRole('link', { name: indexArtifact.repository_name }).last().click();
  await page.waitForTimeout(1000);
  await page.getByRole('link', { name: indexArtifact.repository_name }).last().click();
  // await page.getByRole('link', { name: indexArtifact.repository_name }).last().click();
  await expect(page.locator('h2')).toContainText(index_repo);

  // do digest jump test
  await page.goto('/');
  await page.getByRole('link', { name: 'Interrogation Services' }).click();
  // await page.getByRole('link', { name: 'Security Hub' }).click();
  await expect(page.getByRole('link', { name: indexArtifact.digest.substring(0, 12) }).last()).toBeVisible();
  await page.getByRole('link', { name: indexArtifact.digest.substring(0, 12) }).last().click();
  // await page.getByRole('gridcell', { name: 'sha256:3f42abf2' }).click();
  await page.waitForTimeout(1000);
  // await page.getByRole('link', { name: summary.dangerous_artifacts[0].digest.substring(0, 12) }).last().click();
  // await expect(page.getByRole('link', { name: summary.dangerous_artifacts[0].digest.substring(0, 12) })).toBeVisible();
  await expect(page.locator('h2')).toContainText(indexArtifact.digest.substring(0, 12));

  // top 5 dangerous artifacts jump test
  await page.goto('/');
  await page.getByRole('link', { name: 'Interrogation Services' }).click();
  await expect(page.locator('app-vulnerability-summary')).toContainText(summary.dangerous_artifacts[0].digest.substring(0, 12));
  await page.getByRole('link', { name: summary.dangerous_artifacts[0].digest.substring(0, 12) }).last().click();
  await page.waitForTimeout(1000);
  await expect(page.locator('h2')).toContainText(summary.dangerous_artifacts[0].digest.substring(0, 12));

  // top 5 dangerous artifacts jump test 2
  await page.goto('/');
  await page.getByRole('link', { name: 'Interrogation Services' }).click();
  await expect(page.locator('app-vulnerability-summary')).toContainText(summary.dangerous_artifacts[1].digest.substring(0, 12));
  await page.getByRole('link', { name: summary.dangerous_artifacts[1].digest.substring(0, 12) }).last().click();
  await page.waitForTimeout(1000);
  await expect(page.locator('h2')).toContainText(summary.dangerous_artifacts[1].digest.substring(0, 12));
  // -- jump tests done --
  await page.goto('/');
  await page.getByRole('link', { name: 'Interrogation Services' }).click();

  // Check that there is no such artifact in the security hub after deleting the artifact
  // delete index repo and then check security hub

  await page.getByRole('link', { name: 'Projects' }).click();
  await page.getByRole('link', { name: project }).click();
  // await expect(page.locator('#clr-dg-row381')).toContainText(project + '/' + index_repo);
  // delete index repo
  const rowRegex = new RegExp(`Select\\s+Select\\s+${project}/${index_repo}`, 'i');

// Wait for row to appear and click its checkbox (label)
const row = page.getByRole('row', { name: rowRegex });
await row.waitFor({ state: 'visible', timeout: 10000 }); // ensure it's visible
await row.locator('label').click();
await page.waitForTimeout(5000);
await page.getByRole('button', { name: 'Delete' }).click();
await page.getByRole('button', { name: 'DELETE', exact: true }).click();

  // await page.getByRole('row', { name: 'Select Select aproject-1764149771585/index1 1 4 11/26/25, 3:07 PM' }).locator('label').click();
  // await page.getByRole('button', { name: 'Delete' }).click();
  // await page.getByRole('button', { name: 'DELETE', exact: true }).click();

  await page.getByRole('link', { name: 'Interrogation Services' }).click();
  await expect(page.locator('app-vulnerability-summary')).not.toContainText(project + '/' + index_repo);

  await page.locator('app-vulnerability-filter').getByRole('combobox').selectOption('repository_name');
  await page.locator('app-vulnerability-filter').getByRole('textbox').click();
  await page.locator('app-vulnerability-filter').getByRole('textbox').fill(project + '/' + index_repo);
  await page.getByRole('button', { name: 'SEARCH' }).click();
  await expect(page.locator('clr-dg-placeholder')).toContainText('We could not find any vulnerability');

  // await page.getByRole('row', { name: /Select Select ${project} + '/' + ${image}/ }).locator('label').click();
    // logout
    await page.goto('/');
    await page.getByRole('button', { name: user, exact: true }).waitFor();
    await page.getByRole('button', { name: user, exact: true }).click();
    await page.getByRole('menuitem', { name: 'Log Out' }).click();
});
export async function getVulnerabilitySummaryFromAPI(ip: string, user: string, password: string) {
  // Encode credentials for Basic Auth
  const credentials = Buffer.from(`${user}:${password}`).toString('base64');

  // API endpoint (mirrors your Robot curl command)
  const url = `https://${ip}/api/v2.0/security/summary?with_dangerous_cve=true&with_dangerous_artifact=true`;

  const previousTlsSetting = process.env.NODE_TLS_REJECT_UNAUTHORIZED;
  process.env.NODE_TLS_REJECT_UNAUTHORIZED = '0';

  let response: Response;
  try {
    response = await fetch(url, {
      method: 'GET',
      headers: {
        'Authorization': `Basic ${credentials}`,
        'Content-Type': 'application/json',
      },
    });
  } finally {
    if (previousTlsSetting === undefined) {
      delete process.env.NODE_TLS_REJECT_UNAUTHORIZED;
    } else {
      process.env.NODE_TLS_REJECT_UNAUTHORIZED = previousTlsSetting;
    }
  }

  if (!response.ok) {
    throw new Error(`Failed to fetch vulnerability summary: ${response.status} ${response.statusText}`);
  }

  const json = await response.json();
  return json;
}

type CommandOptions = {
  input?: string;
  redactedArgs?: string[];
};

export function runCommand(command: string, args: string[] = [], options: CommandOptions = {}): string {
  const redactedArgs = new Set(options.redactedArgs || []);
  const displayArgs = args.map(arg => redactedArgs.has(arg) ? '<redacted>' : arg);
  const displayCommand = [command, ...displayArgs].join(' ');
  console.log(`\n$ ${displayCommand}`);

  try {
    const output = execFileSync(command, args, {
      encoding: 'utf-8',  // ensures string output
      stdio: ['pipe', 'pipe', 'pipe'], // capture all streams
      input: options.input,
    });

    console.log('Command output:\n', output.trim()); // print captured output
    return output.trim(); // return for further processing
  } catch (error: any) {
    console.error(`Command failed: ${displayCommand}`);
    console.error('--- STDOUT ---\n', error.stdout?.toString()?.trim() || '');
    console.error('--- STDERR ---\n', error.stderr?.toString()?.trim() || '');
    throw error;
  }
}

function requiredEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    throw new Error(`Missing required env var: ${name}`);
  }
  return value;
}

/**
 * Tags and pushes a single image to Harbor.
 */
function pushImageWithTag(
  ip: string,
  user: string,
  pwd: string,
  project: string,
  image: string,
  tag: string,
  tag1: string = 'latest'
): void {
  console.log(`\nRunning docker push for ${image}...`);

  const sourceImage = `${LOCAL_REGISTRY}/${LOCAL_REGISTRY_NAMESPACE}/${image}:${tag1}`;
  const targetImage = `${ip}/${project}/${image}:${tag}`;

  // Pull image from local registry
  runCommand('docker', ['pull', sourceImage]);

  // Login to Harbor
  runCommand('docker', ['login', '-u', user, '--password-stdin', ip], {
    input: `${pwd}\n`,
  });

  // Tag image for Harbor project
  runCommand('docker', ['tag', sourceImage, targetImage]);

  // Push image to Harbor
  runCommand('docker', ['push', targetImage]);

  // Logout after push
  runCommand('docker', ['logout', ip]);
}
