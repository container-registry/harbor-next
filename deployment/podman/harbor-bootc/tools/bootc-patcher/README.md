# Bootc Patcher

`bootc-patcher` is a local orchestration tool for Harbor-hosted bootc images.
It turns the Harbor scan result into a repeatable remediation loop:

1. Trigger Harbor vulnerability scan and SBOM generation.
2. Poll until Harbor reports finish.
3. Export Harbor's vulnerability report for audit.
4. Run Trivy locally in JSON mode to produce Copa-compatible input.
5. Optionally map Dakota file findings back to BuildStream elements.
6. In `patch` mode, run `copa patch` and push the patched tag.
7. Trigger Harbor scans for the patched tag.

The tool does not hide the supply-chain tools. Copa, Trivy, Podman, and any
local Chunkah integration remain external commands so their versions are
explicit.

## Plan Mode

```bash
cd deployment/podman/harbor-bootc/tools/bootc-patcher

go run . \
  -harbor-url http://100.100.156.26:18085 \
  -registry 100.100.156.26:18085 \
  -project bluefin \
  -repository dakota-bootc \
  -reference latest \
  -target-tag patched \
  -username admin \
  -password Harbor12345 \
  -buildkit-addr "unix://../../../../../temp/buildkit/buildkitd.sock" \
  -authfile ../../../../../temp/harbor-push/auth.json \
  -workdir ../../../../../temp/harbor-patch/dakota \
  -filemap ../../../../../temp/dakota/files/fakecap-manifest.tsv \
  -tls-verify=false \
  -mode plan
```

Plan mode is non-mutating after scan generation. It writes:

- `harbor-vulnerabilities.json`
- `trivy-report.json`
- `source-remediation.md`, when `-filemap` is set

## Patch Mode

Install Copa and ensure BuildKit is usable first. Patch mode pulls the source
image with Podman, patches a local `localhost/...` tag with Copa, then pushes
the result with Podman. This avoids Copa talking directly to the HTTP registry.

Then run:

```bash
go run . \
  -harbor-url http://100.100.156.26:18085 \
  -registry 100.100.156.26:18085 \
  -project bluefin \
  -repository dakota-bootc \
  -reference latest \
  -target-tag patched \
  -username admin \
  -password Harbor12345 \
  -buildkit-addr "unix://../../../../../temp/buildkit/buildkitd.sock" \
  -authfile ../../../../../temp/harbor-push/auth.json \
  -workdir ../../../../../temp/harbor-patch/dakota \
  -tls-verify=false \
  -mode patch
```

Expected patched image:

```text
100.100.156.26:18085/bluefin/dakota-bootc:patched
```

For the current Dakota report, patch mode is expected to stop before pushing
because Copa only applies OS package-manager updates. The present findings are
Go binary and Python package dependency findings.

## Durable Dakota Fix

Copa is an emergency binary remediation layer. Dakota itself is built from
BuildStream elements, not package-manager commands in a Containerfile. Durable
fixes should use `source-remediation.md` to update the owning `.bst` source refs
or junctions, rebuild with `BUILD_SKIP_NVIDIA=1 just build default`, export,
run Chunkah via the existing publish workflow pattern, and push a normal rebuilt
image.
