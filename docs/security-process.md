# Security Response Process (Maintainer Runbook)

This document describes how Harbor Next maintainers handle private vulnerability reports, from intake to public disclosure. The reporter-facing policy is [SECURITY.md](../SECURITY.md).

The whole process happens inside **one GitHub artifact**: the private report becomes the draft security advisory, the fix is developed in the advisory's temporary private fork, the CVE is requested inside the advisory, and publishing the advisory is the disclosure. Nothing is re-transcribed between systems.

This process follows the workflow proposed for upstream Harbor in [goharbor/community#292](https://github.com/goharbor/community/pull/292). Harbor Next acts as the reference implementation.

## Roles

- **Security responders**: maintainers with admin access to the repository, or members of a GitHub team with the **security manager** role on the org. They see incoming private reports and receive notifications for them.
- **Advisory lead**: the responder who accepts a report. They own the advisory through publication: triage, severity, fix coordination, CVE request, and disclosure.

Every incoming report needs exactly one advisory lead. If you accept a report, you are the lead until you explicitly hand it over.

## 1. Receiving a report

Reports arrive at **Security → Advisories** as `Proposed` advisories: <https://github.com/container-registry/harbor-next/security/advisories?state=triage>

Notifications for new private reports go to repo admins and org security managers only. **Check the queue at least weekly** even if you saw no notification.

Within **3 business days** of a report arriving:

1. Reply in the advisory thread to acknowledge receipt. The reporter can see comments on the advisory; nothing else is needed to keep them informed.
2. Do a first sanity check:
   - **Spam or clearly invalid** → close the report with a short explanation. Anyone with a GitHub account can file, so expect noise; closing politely and quickly is part of the job.
   - **Already public** (existing CVE, public scanner finding, already-disclosed dependency issue) → ask the reporter to re-file as a public issue and close the report.
   - **Dependency vulnerability, not exploitable in Harbor Next** → treat as a normal dependency update (Dependabot / `zero-cve` workflow), not a Harbor Next advisory. Explain and close.
   - **Wrong repo** (e.g. an upstream `goharbor/harbor` issue that does not affect Harbor Next code) → point the reporter to the right project's security policy and close.
   - **Plausible product vulnerability** → continue to triage.

## 2. Triage

Within **7 calendar days** of the report:

1. Reproduce the issue, or establish why reproduction isn't needed (e.g. obvious by code inspection). Ask the reporter for missing details in the advisory thread.
2. If confirmed, click **Accept and open as draft**. The report converts into a draft security advisory — same object, no copying.
3. Fill in the draft advisory fields:
   - **Affected products/versions**: ecosystem, package, affected version ranges, and patched versions (fill patched versions in once known).
   - **CWE** classification.
   - **Severity**: use the built-in **CVSS calculator** (v3.1 or v4.0). Score it yourself; do not just accept the reporter's score, but do share and discuss your assessment with them.
4. If declined, explain why in the thread and close the advisory.

Severity drives urgency:

| CVSS | Severity | Handling |
| ---- | -------- | -------- |
| 9.0–10.0 | Critical | Drop other work. Fix + advisory target: **14 business days**. Patch release for the supported release line. |
| 7.0–8.9 | High | Fix in the next patch release; target within 30 days. |
| 4.0–6.9 | Medium | Fix in a regular upcoming release. |
| 0.1–3.9 | Low | Fix opportunistically; may be handled as a public issue after agreeing with the reporter. |

While a fix is in progress, post a status update in the advisory thread at least every **14 days** — the reporter has no other visibility into progress.

## 3. Embargoed fix (temporary private fork)

Do **not** develop the fix in the public repository. Public commits, branches, or PRs leak the vulnerability before disclosure.

1. In the draft advisory, click **Start a temporary private fork**. GitHub creates `harbor-next-ghsa-xxxx-xxxx-xxxx`, visible only to advisory collaborators.
2. Add collaborators as needed via the advisory's collaborator list. The reporter is already a collaborator; add only the maintainers actually working the fix. Access is scoped to this one advisory and cleaned up automatically.
3. Develop and review the fix as PRs inside the private fork. Keep the commit message neutral — it becomes public on disclosure, but should not read as a public spoiler if seen early.
4. **CI does not run on private forks.** Run the test suite locally (`task test:quick`, `task test:ci`) and record in the advisory thread what was run and the results. Do not merge an untested fix.
5. Do not merge the private fork's PR into the public repo until publication time (step 6).

## 4. CVE request

For confirmed vulnerabilities in Harbor Next code:

1. In the draft advisory, choose **Request CVE**. GitHub (acting as CNA) reviews the request, usually within **72 hours**, and reserves a CVE ID bound to the advisory. The CVE is published to MITRE only when the advisory is published.
2. If the affected component is covered by another CNA (e.g. the vulnerability is really in a vendored/upstream component), GitHub may decline; coordinate with that component's security team instead and reference their CVE in our advisory.
3. Vulnerabilities inherited from upstream `goharbor/harbor` normally get their CVE through upstream's process; our advisory then references the upstream CVE rather than requesting a new one.

## 5. Pre-disclosure coordination

1. Agree on a disclosure date with the reporter in the advisory thread. Default: as soon as the fixed release is available; at most **90 days** from the report.
2. Prepare the patch release: the fix will need to land on `main` and be backported to the supported `release-X.Y` branch (see [SECURITY.md](../SECURITY.md) supported versions).
3. If the issue affects known downstream distributors or operators, notify them under embargo before publication (currently ad hoc — see "Open items" below).
4. Prefer publishing on a **weekday, not Friday**, so operators can respond during working hours.

## 6. Publish and release

Order matters — the fix must be available when the advisory goes public:

1. Merge the fix from the temporary private fork (the advisory's **Merge pull request** flow pushes the commits to the public repo).
2. Cut the patch release(s) through the normal release process. Release notes must mention the advisory: `Fixes GHSA-xxxx-xxxx-xxxx (CVE-YYYY-NNNNN)` with severity and a one-line description — no exploit details beyond the advisory.
3. Fill in the advisory's **patched versions** field.
4. Add **credits**: the reporter (with the appropriate role, e.g. *reporter* / *analyst*), who must accept the credit for it to display. Ask first if they want anonymity.
5. Click **Publish advisory**. This makes the advisory public, publishes the CVE to MITRE, and submits it to the GitHub Advisory Database (review can take up to 72h; Dependabot alerts then reach downstream Go-module consumers).
6. The temporary private fork is deleted automatically after publication.

Note: Dependabot alerts only reach consumers of Harbor Next **Go modules**, not users running container images. Image users learn about the fix from the advisory, the release notes, and the registry tags — which is why step 2's release-note convention is mandatory.

## 7. After disclosure

- Verify the advisory appears under [published advisories](https://github.com/container-registry/harbor-next/security/advisories) and the CVE record is live.
- Post-mortem (optional but encouraged for High/Critical): what allowed the bug, whether a test/linter/scanner could catch the class, and file follow-up issues.
- If the issue also affects upstream `goharbor/harbor`, make sure upstream's security team was informed **before** our disclosure, not after.

## Checklist (copy into the advisory thread)

```text
- [ ] Acknowledged reporter (≤3 business days)
- [ ] Triaged: reproduced / declined (≤7 days)
- [ ] Accepted as draft advisory; CWE + CVSS filled in
- [ ] Temporary private fork created; collaborators scoped
- [ ] Fix developed + reviewed in private fork
- [ ] Tests run locally, results recorded (no CI on private forks!)
- [ ] CVE requested via GitHub
- [ ] Disclosure date agreed with reporter
- [ ] Backport(s) prepared for supported release line(s)
- [ ] Release notes reference GHSA/CVE
- [ ] Credits added and accepted
- [ ] Advisory published; patched versions filled in
- [ ] Upstream goharbor/harbor informed (if affected)
```

## Open items (need org-level decisions)

- **Security responder roster**: create a GitHub team with the org **security manager** role instead of relying on repo admins, so the roster is explicit and auditable.
- **Distributor/operator embargo list**: upstream Harbor uses `cncf-harbor-distributors-announce`; Harbor Next has no equivalent yet. Decide if/when one is needed.
- **Fallback intake for reporters without a GitHub account**: currently "contact a maintainer privately"; a dedicated alias would be cleaner.
- **security.txt**: publish `/.well-known/security.txt` on the project website pointing at the GitHub report form (requires a website/domain decision, plus a yearly `Expires` refresh).
