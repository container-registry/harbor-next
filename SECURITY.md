# Security Policy

Harbor Next takes security vulnerabilities seriously. Please report suspected vulnerabilities privately so project maintainers can investigate, coordinate fixes, and prepare disclosure before details are made public.

Harbor Next is maintained independently from the upstream `goharbor/harbor` project. Do not use the Harbor security mailing lists for Harbor Next vulnerability reports.

## Supported Versions

| Version | Supported |
| ------- | --------- |
| Latest stable release line (currently `2.15.x`) | Yes — security fixes and patch releases |
| `main` (next release) | Yes — fixes land here first |
| Older release lines | No — best effort only, for Critical issues |

If you are unsure whether a version is supported, report the issue anyway and include the affected version, commit, image tag, and deployment details. Maintainers will determine affected versions and whether backports are feasible based on severity and release status.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately through GitHub private vulnerability reporting:

- Open a private vulnerability report directly at [Report a vulnerability](https://github.com/container-registry/harbor-next/security/advisories/new).
- Or go to the repository **Security** tab and click **Report a vulnerability**.
- Or go to [New Issue](https://github.com/container-registry/harbor-next/issues/new/choose) and select **Security Vulnerability**, which routes to the same private form.

Reporting requires a GitHub account. If you cannot use GitHub, contact a maintainer listed in [OWNERS.md](OWNERS.md) through any private channel and ask them to open the report on your behalf.

Private vulnerability reports are visible only to project maintainers, who will coordinate with the reporter during investigation and remediation. The maintainer-side handling process is documented in [docs/security-process.md](docs/security-process.md).

**If the vulnerability already has a public CVE or is already publicly disclosed** (for example, a scanner finding in one of our dependencies), open a normal [public issue](https://github.com/container-registry/harbor-next/issues/new/choose) instead — there is nothing left to keep private.

## What to Include

Please include as much of the following information as possible:

- Your name and affiliation, if you are comfortable sharing it.
- The affected Harbor Next version, commit, image tag, or branch.
- Deployment details such as Docker Compose, Kubernetes, cloud provider, configuration, and enabled components.
- Detailed steps to reproduce the issue, including proof-of-concept code, screenshots, logs, or packet captures when helpful.
- The expected and actual impact on confidentiality, integrity, availability, privilege boundaries, authentication, authorization, or tenant isolation.
- Attack prerequisites, affected roles, required permissions, and exposed attack surface.
- Any related upstream Harbor behavior, dependencies, or third-party projects involved.
- Whether the vulnerability is already public or being actively exploited, if known.

Avoid including production secrets, private keys, credentials, or sensitive customer data in the report.

## When to Report

Report privately when:

- You believe Harbor Next has a potential security vulnerability.
- You suspect a vulnerability but are unsure whether it impacts Harbor Next.
- You know of a not-yet-public vulnerability in a dependency that may affect Harbor Next.

For non-security bugs, feature requests, and proposals, use the public [GitHub issue templates](https://github.com/container-registry/harbor-next/issues/new/choose).

## Response Targets

| Step | Target |
| ---- | ------ |
| Acknowledge your report | Within **3 business days** |
| Triage decision (accepted / declined) and initial severity | Within **7 calendar days** |
| Status updates while a fix is in progress | At least every **14 days** |
| Fix and advisory for **Critical** issues | Within **14 business days** of triage |
| Coordinated public disclosure | Within **90 days** of the report, or earlier once a fix is released |

These are targets, not guarantees. Complex issues can take longer; if a vulnerability is already public or actively exploited, maintainers may disclose and ship a fix faster.

## What Reporters Can Expect

- Your report is handled through a private GitHub security advisory. You are added as a collaborator on the advisory and can follow and participate in the discussion and the fix.
- Maintainers score severity with CVSS (using the advisory's built-in calculator) and share the assessment with you.
- For confirmed vulnerabilities, maintainers request a **CVE** through GitHub inside the advisory.
- You receive **credit** in the published advisory unless you prefer to stay anonymous.
- If the report is declined, maintainers will explain the reason and may ask that it be re-filed as a public issue if appropriate.

## Disclosure Policy

Maintainers coordinate public disclosure with the reporter after a mitigation or patch is available. Public disclosure happens through [GitHub Security Advisories](https://github.com/container-registry/harbor-next/security/advisories), the release notes of the fixed release, and the CVE record. Fixes for embargoed issues are developed in a temporary private fork and are not visible in the public repository until the advisory is published.

## Security Priorities

The highest priority reports are issues that compromise confidentiality, integrity, availability, authentication, authorization, privilege boundaries, or tenant isolation. Denial-of-service and resource-exhaustion issues are also security concerns when they can materially affect Harbor Next operators or users.
