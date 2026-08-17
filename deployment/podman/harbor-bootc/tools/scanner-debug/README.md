# Bootc Scanner Debug Tools

These scripts are the working scanner harnesses copied from the bootc
investigation scratch area and cleaned up for the `8gcr-bootc` branch.

They are diagnostic tools, not the final production scanner integration. They
write only under `./temp` by default.

## Scripts

- `scan-loop.sh` triggers Harbor vulnerability scans for Bluefin, Bazzite, and
  Dakota until each artifact has passed twice with a non-empty vulnerability
  report.
- `trivy-sbom-force-os.sh` fetches Harbor's generated SPDX SBOM, extracts RPM
  package purls, rewrites them into a CycloneDX SBOM with explicit OS metadata,
  and runs `trivy sbom --pkg-types os`.
- `grype-sbom-scan.sh` fetches Harbor's generated SPDX SBOM and runs Grype
  directly against that SBOM.
- `clair-scan.sh` starts a local Clair stack under `./temp/clair-bluefin` and
  records direct-image and best-effort SBOM scan behavior.

## Common Environment

```bash
export HARBOR_URL=http://100.100.156.26:18085
export HARBOR_AUTH='admin:Harbor12345'
```

For scripts that use separate user/password variables:

```bash
export HARBOR_USER=admin
export HARBOR_PASSWORD=Harbor12345
```

## Acceptance Loop

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/scan-loop.sh
```

The loop succeeds only when these three artifacts pass twice:

```text
bluefin/bluefin-bootc:latest-20260708
bazzite/bazzite-bootc:stable
dakota/dakota-bootc:stable
```

Output goes to:

```text
temp/scan-check/results/
```

## SBOM-Based Trivy Probe

Generate SBOM in Harbor first:

```bash
curl -fsS -u "$HARBOR_AUTH" \
  -H 'Content-Type: application/json' \
  -X POST --data '{"scan_type":"sbom"}' \
  "$HARBOR_URL/api/v2.0/projects/bluefin/repositories/bluefin-bootc/artifacts/latest-20260708/scan"
```

Then run:

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/trivy-sbom-force-os.sh
```

Override the forced OS identity if the image is not Fedora 42:

```bash
OS_NAME=fedora OS_VERSION=44 \
  deployment/podman/harbor-bootc/tools/scanner-debug/trivy-sbom-force-os.sh
```

The key output is:

```text
temp/trivy-sbom-force-os/trivy-from-forced-cyclonedx.json
```

## Grype SBOM Probe

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/grype-sbom-scan.sh
```

The key output is:

```text
temp/grype-sbom-scan/grype-from-sbom.json
```

## Clair Probe

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/clair-scan.sh
```

Clair can be expensive to initialize because it downloads updater data. Use:

```bash
CLAIR_UPDATE_WAIT_SECONDS=0 \
  deployment/podman/harbor-bootc/tools/scanner-debug/clair-scan.sh
```

to capture startup behavior without waiting for updater data.

## Important Findings From The Scratch Run

- Harbor SBOM generation succeeded for Bluefin, Bazzite, and Dakota.
- Direct Harbor vulnerability scans completed with `Success` but zero
  vulnerabilities.
- Direct `trivy image --pkg-types os` against the Harbor Bluefin image returned
  no OS package results.
- Trivy image-to-CycloneDX SBOM found Go/library components but no RPM
  components.
- The likely scanner path is therefore SBOM-derived OS package scanning, using
  Harbor's generated SBOM as the package source of truth.
