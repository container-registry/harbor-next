# Security Policy

Harbor Next takes security vulnerabilities seriously. Please report suspected vulnerabilities privately so project maintainers can investigate, coordinate fixes, and prepare disclosure before details are made public.

Harbor Next is maintained independently from the upstream `goharbor/harbor` project. Do not use the Harbor security mailing lists for Harbor Next vulnerability reports.

## Supported Versions

<<<<<<< HEAD
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
=======
## Reporting a Vulnerability - Private Disclosure Process
Security is of the highest importance and all security vulnerabilities or suspected security vulnerabilities should be reported to Harbor privately, to minimize attacks against current users of Harbor before they are fixed. Vulnerabilities will be investigated and patched on the next patch (or minor) release as soon as possible. This information could be kept entirely internal to the project.

If you know of a publicly disclosed security vulnerability for Harbor, please **IMMEDIATELY** report it via [GitHub private vulnerability reporting](https://github.com/goharbor/harbor/security/advisories/new) to inform the Harbor Security Team.

> [!IMPORTANT]
> **Do not file public issues on GitHub for security vulnerabilities**

The report is visible only to you and the Harbor maintainers. Reports will be acknowledged within 5 business days. The Harbor Team will then investigate and follow up with a plan to address the issue and any potential workarounds to perform in the meantime. Filing a report requires a GitHub account; this is the only channel for vulnerability reports.

> [!NOTE]
> Do not report non-security-impacting bugs through this channel. Use [GitHub issues](https://github.com/goharbor/harbor/issues/new/choose) instead.


### Proposed Report Content
Provide a descriptive title and in the description of the report include the following information:
* Basic identity information, such as your name and your affiliation or company.
* Detailed steps to reproduce the vulnerability (POC scripts and screenshots are helpful to us; larger artifacts such as packet captures can be shared in the temporary private fork once the report is accepted).
* Description of the effects of the vulnerability on Harbor and the related hardware and software configurations, so that the Harbor Security Team can reproduce it.
* How the vulnerability affects Harbor usage and an estimation of the attack surface, if there is one.
* List other projects or dependencies that were used in conjunction with Harbor to produce the vulnerability.

## When to report a vulnerability
* When you think Harbor has a potential security vulnerability.
* When you suspect a potential vulnerability, but you are unsure that it impacts Harbor.
* When you know of or suspect a potential vulnerability on another project that is used by Harbor. For example Harbor has a dependency on Docker, PostgreSQL, Redis, Trivy, etc.

## Patch, Release, and Disclosure
The Harbor Security Team will respond to vulnerability reports as follows:

1.  The Security Team will investigate the vulnerability and determine its effects and criticality.
2.  If the issue is not deemed to be a vulnerability, the Security Team will close the report with a detailed reason for rejection.
3.  The Security Team will acknowledge the report and initiate a conversation with the reporter within 5 business days.
4.  If the report is confirmed as a vulnerability, the Security Team will accept it as a draft security advisory, and the reporter is added as a collaborator on it. The Security Team will then work on a plan to communicate with the appropriate community, including identifying mitigating steps that affected users can take to protect themselves until the fix is rolled out.
5.  The Security Team will also assess the severity of the vulnerability with a [CVSS](https://www.first.org/cvss/specification-document) score, using the calculator built into the draft advisory. The Security Team makes the final call on the calculated CVSS; it is better to move quickly than making the CVSS perfect. Where GitHub is eligible to act as CNA, a CVE will be requested through the draft advisory and remains private until the advisory is published. If another CNA already covers the affected component, the Security Team coordinates with that CNA instead.
6.  The Security Team will work on fixing the vulnerability in a temporary private fork associated with the advisory, keeping the patch embargoed, and perform internal testing before preparing to roll out the fix.
7.  The Security Team will provide early disclosure of the vulnerability by emailing the cncf-harbor-distributors-announce@lists.cncf.io mailing list. Distributors can initially plan for the vulnerability patch ahead of the fix, and later can test the fix and provide feedback to the Harbor team. See the section **Early Disclosure to Harbor Distributors List** for details about how to join this mailing list.
8. A public disclosure date is negotiated by the Harbor Security Team, the bug submitter, and the distributors list. We prefer to fully disclose the bug as soon as possible once a user mitigation or patch is available. It is reasonable to delay disclosure when the bug or the fix is not yet fully understood, the solution is not well-tested, or for distributor coordination. The timeframe for disclosure is from immediate (especially if it’s already publicly known) to a few weeks. For a critical vulnerability with a straightforward mitigation, we expect report date to public disclosure date to be on the order of 14 business days. The Harbor Security Team holds the final say when setting a public disclosure date.
9.  Once the fix is confirmed, the Security Team will patch the vulnerability in the next patch or minor release, and backport a patch release into all earlier supported releases. Upon release of the patched version of Harbor, we will follow the **Public Disclosure Process**.

### Public Disclosure Process
The Security Team publishes the security [advisory](https://github.com/goharbor/harbor/security/advisories) to the Harbor community via GitHub. Where GitHub issued the CVE, publishing the advisory also publishes the CVE. In most cases, additional communication via Slack, Twitter, CNCF lists, blog and other channels will assist in educating Harbor users and rolling out the patched release to affected users.

The Security Team will also publish any mitigating steps users can take until the fix can be applied to their Harbor instances. Harbor distributors will handle creating and publishing their own security advisories.

## Mailing lists
- Use cncf-harbor-security@lists.cncf.io to reach the Harbor Security Team. Do not use it to report vulnerabilities; use [GitHub private vulnerability reporting](https://github.com/goharbor/harbor/security/advisories/new) instead.
>>>>>>> cce1901e9 (docs: switch security reporting to GitHub private vulnerability reporting (#23559))

Reporting requires a GitHub account. If you cannot use GitHub, contact a maintainer listed in [OWNERS.md](OWNERS.md) through any private channel and ask them to open the report on your behalf.

<<<<<<< HEAD
Private vulnerability reports are visible only to project maintainers, who will coordinate with the reporter during investigation and remediation. The maintainer-side handling process is documented in [docs/security-process.md](docs/security-process.md).
=======
### Membership Criteria
To be eligible to join the cncf-harbor-distributors-announce@lists.cncf.io mailing list, you should:
1. Be an active distributor of Harbor.
2. Have a user base that is not limited to your own organization.
3. Have a publicly verifiable track record up to the present day of fixing security issues.
4. Not be a downstream or rebuild of another distributor.
5. Be a participant and active contributor in the Harbor community.
6. Accept the Embargo Policy that is outlined below.
7. Has someone who is already on the list vouch for the person requesting membership on behalf of your distribution.
>>>>>>> cce1901e9 (docs: switch security reporting to GitHub private vulnerability reporting (#23559))

**If the vulnerability already has a public CVE or is already publicly disclosed** (for example, a scanner finding in one of our dependencies), open a normal [public issue](https://github.com/container-registry/harbor-next/issues/new/choose) instead — there is nothing left to keep private.

## What to Include

<<<<<<< HEAD
Please include as much of the following information as possible:
=======
In the unfortunate event that you share information beyond what is permitted by this policy, you must urgently inform the Harbor Security Team via cncf-harbor-security@lists.cncf.io of exactly what information was leaked and to whom. If you continue to leak information and break the policy outlined here, you will be permanently removed from the list.

### Requesting to Join
Send new membership requests to cncf-harbor-security@lists.cncf.io.
In the body of your request please specify how you qualify for membership and fulfill each criterion listed in the Membership Criteria section above.
>>>>>>> cce1901e9 (docs: switch security reporting to GitHub private vulnerability reporting (#23559))

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
