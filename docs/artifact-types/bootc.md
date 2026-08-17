# bootc image scanning

[bootc](https://containers.github.io/bootc/) images are bootable operating
system images distributed as OCI container images — a full Fedora/CentOS
Stream/RHEL-derived root filesystem with an RPM package database inside,
rather than an application layered on a base image. Regular container
vulnerability scanning only sees what's in the OCI layers as generic
filesystem content; it doesn't understand that there's an RPM database in
there to interrogate the way `dnf`/`rpm` would. This guide is about that gap
— **pushing and pulling bootc images is unchanged**, this is purely about
getting real CVE coverage for what's inside them.

## What's different

A bootc-aware scanner:

1. Recognizes the image as bootc via the `containers.bootc: "1"` label on
   the image config (the same label `bootc` itself uses).
2. Locates the RPM package database inside the image content — including
   hardlinked database files, which trip up naive extraction.
3. Extracts the installed-package inventory (via Syft's RPM/dpkg/apk/pacman
   catalogers) and identifies the OS release (e.g. "CentOS Stream 9") from
   bootc-specific metadata, not just `/etc/os-release` heuristics.
4. Matches that inventory against CVE data **for that specific OS release**
   — version-specific matching, not just "some RPM named foo exists
   somewhere."
5. Reports results as a standard Harbor SBOM + vulnerability report, so they
   show up in the Portal exactly like any other scan.

## Enabling it

Two scanner adapters ship with this capability; either can be your default
or project-level scanner, same as Trivy:

- **Grype-based** (`tools/grype-scanner`) — via Docker Compose:
  ```bash
  WITH_GRYPE=true DEFAULT_SCANNER=grype task dev:up
  ```
  or in production, enable the `grype` profile and set
  `DEFAULT_SCANNER=grype` in your deployment config. Leave `DEFAULT_SCANNER`
  unset to keep Trivy as default and just make Grype available as an
  alternative scanner per-project.
- **Snyk-based** (`tools/snyk-scanner`) — opt-in the same way via the `snyk`
  compose profile (`WITH_SNYK=true`); requires a Snyk API token.

Once enabled, register/select the scanner the same way as any other:
**Administration → Interrogation Services** (system default) or per-project
under **Project → Configuration → Vulnerability Scanner**.

The underlying vulnerability data comes from a purpose-built
`harbor-scanner-trivy` image (tag `8gcr-bootc` in `versions.env`'s
`HARBOR_SCANNER_TRIVY_VERSION`) with bootc/RPM package-DB support baked in —
if you're building your own images rather than using the published ones,
make sure that pin is what gets deployed, not a stock upstream
`harbor-scanner-trivy` tag.

## Using it

Nothing changes about pushing or pulling bootc images — `podman push` /
`docker push` a bootc image to a Harbor project exactly as any other
container image. Once a bootc-aware scanner is your project's (or system's)
scanner, pushes trigger auto-scan as normal (if enabled), or trigger it
manually from the artifact's **Vulnerabilities** tab. The report shown is a
regular Harbor vulnerability report — CVE IDs, severity, fixed-in version —
just computed from the RPM inventory instead of generic layer content.

## Troubleshooting

| Symptom | Cause |
|---|---|
| Scan completes but finds nothing / treats it like an empty image | The `containers.bootc` label is missing or not `"1"` — confirm the image was actually built with `bootc`/`bootc-image-builder`, or that a custom build process preserved the label. |
| Scan fails outright on a bootc image that scans fine as a generic image with Trivy | RPM database extraction failed — check the scanner adapter logs; hardlinked RPM DBs and unusual layer layouts are the known edge cases here (`tools/grype-scanner/rpmdb.go`, `package_db.go`). |
| CVE results look like they're for the wrong OS release | The OS-release detection reads bootc-specific metadata, not just `/etc/os-release` — confirm the image is a genuine CentOS Stream/Fedora/RHEL-derived bootc build, not a relabeled generic image. |
| `WITH_GRYPE=true` but the scanner never shows up as selectable | Confirm the `grype` compose profile actually started (`docker compose ps`) and that Administration → Interrogation Services shows it registered — registration happens at Core startup. |

## Source pointers

`tools/grype-scanner/{main,rpmdb,package_db,buildstream}.go`,
`tools/snyk-scanner/main.go`, `versions.env`
(`GRYPE_VERSION`, `SYFT_VERSION`, `HARBOR_SCANNER_TRIVY_VERSION`),
`deploy/compose/docker-compose.yaml` (`WITH_GRYPE`, `WITH_SNYK`,
`DEFAULT_SCANNER`), `taskfile/dev.yml` (`PORT_GRYPE`, `PORT_SNYK` dev ports).
