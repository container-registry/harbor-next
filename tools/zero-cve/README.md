# Zero CVE automation

The `Zero CVE` workflow runs once per day and can also be started manually.

It scans:

- the repository with `trivy fs`
- Go dependencies with `govulncheck ./...`
- the branch-appropriate published images with `trivy image`

Go dependency findings with fixed module versions are remediated with `go get module@fixed-version`, followed by `go mod tidy`. The workflow pushes those changes to one rolling branch, `automation/zero-cve`, and creates or updates a single PR.

Each run updates the `Zero CVE Daily Report` issue and uploads the raw scanner JSON, rendered PR body, rendered issue body, and summary JSON as the `zero-cve-reports` workflow artifact.

Image selection:

- `main` scans `8gears.container-registry.com/8gcr/harbor-core:latest`, `harbor-jobservice:latest`, `harbor-registryctl:latest`, `harbor-exporter:latest`, `harbor-portal:latest`, `harbor-registry:latest`, and `trivy-adapter:latest`.
- `release-X.Y` scans the newest matching release tag, for example `release-2.15` scans `v2.15.3` when that is the latest `v2.15.x` tag.
- Manual runs default to the selected workflow ref; use the `base_branch` input to scan another branch.
- The manual `image` input, `ZERO_CVE_IMAGES`, or `ZERO_CVE_IMAGE` can override the derived image list.

Configuration:

- `ZERO_CVE_IMAGES` supports a comma, space, or newline-separated list. `ZERO_CVE_IMAGE` is accepted as a single-image fallback.
- `REGISTRY_USERNAME` as a repository variable or secret and `REGISTRY_PASSWORD` as a secret are used by Trivy for private registry access. `HARBOR_USERNAME` and `HARBOR_PASSWORD` are accepted as fallbacks.
- The workflow updates one open issue titled `Zero CVE Daily Report` with the latest scan summary.
