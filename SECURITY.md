# Security Policy

Harbor Next takes security vulnerabilities seriously. Please report suspected vulnerabilities privately so project maintainers can investigate, coordinate fixes, and prepare disclosure before details are made public.

Harbor Next is maintained independently from the upstream `goharbor/harbor` project. Do not use the Harbor security mailing lists for Harbor Next vulnerability reports.

## Supported Versions

Security fixes are provided for actively maintained Harbor Next releases. If you are unsure whether a version is supported, report the issue anyway and include the affected version, commit, image tag, and deployment details. Maintainers will determine affected versions and whether backports are feasible based on severity and release status.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report vulnerabilities privately through either of these GitHub flows:

- Open a private vulnerability report directly at [New GitHub Security Advisory](https://github.com/container-registry/harbor-next/security/advisories/new).
- Go to [New Issue](https://github.com/container-registry/harbor-next/issues/new/choose) and select **Security Vulnerability**. The option is labeled **Report Security Vulnerabilities privately** and routes to the private GitHub Security Advisory flow.

Private vulnerability reports are visible to project maintainers, who will coordinate with the reporter during investigation and remediation.

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
- You know of a publicly disclosed vulnerability that may affect Harbor Next or one of its dependencies.

For non-security bugs, feature requests, and proposals, use the public [GitHub issue templates](https://github.com/container-registry/harbor-next/issues/new/choose).

## Response and Disclosure

Maintainers aim to acknowledge private vulnerability reports within 3 business days. The project will investigate the report, determine impact and severity, identify mitigations or workarounds when available, and coordinate a fix.

If the issue is confirmed, maintainers will coordinate public disclosure with the reporter after a mitigation or patch is available, unless the vulnerability is already public or active exploitation requires faster disclosure. Public disclosure is normally published through [GitHub Security Advisories](https://github.com/container-registry/harbor-next/security/advisories).

If the report is not considered a security vulnerability, maintainers will explain the reason and may ask that it be re-filed as a public issue if appropriate.

## Security Priorities

The highest priority reports are issues that compromise confidentiality, integrity, availability, authentication, authorization, privilege boundaries, or tenant isolation. Denial-of-service and resource-exhaustion issues are also security concerns when they can materially affect Harbor Next operators or users.
