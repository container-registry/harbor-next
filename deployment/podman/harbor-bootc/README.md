# Harbor Bootc Podman Compose

Minimal Podman Compose deployment for Harbor Next bootc validation.

This follows the `deploy/compose` flow:

1. Copy `.env.example` to `.env`.
2. Set secrets and endpoint values.
3. Generate `config/token_service_key.pem`.
4. Run compose.

The portal container is the only public entry point. It serves the Angular UI and reverse-proxies `/v2/`, `/api/`, `/service/`, `/c/`, `/npm/`, and `/maven/` to core.

## Images

Default image settings:

```env
IMAGE_REPO=8gears.container-registry.com/8gcr-dev/
IMAGE_SUFFIX=
HARBOR_TAG=bootc
```

These resolve to:

```text
8gears.container-registry.com/8gcr-dev/harbor-portal:bootc
8gears.container-registry.com/8gcr-dev/harbor-core:bootc
8gears.container-registry.com/8gcr-dev/harbor-jobservice:bootc
8gears.container-registry.com/8gcr-dev/harbor-registryctl:bootc
8gears.container-registry.com/8gcr-dev/harbor-registry:bootc
8gears.container-registry.com/8gcr-dev/harbor-exporter:bootc
8gears.container-registry.com/8gcr-dev/harbor-trivy-adapter:bootc
8gears.container-registry.com/8gcr-dev/harbor-grype-adapter:bootc
```

The published `bootc` image set targets `linux/amd64`. PostgreSQL and Valkey
continue to use their upstream images declared in `compose.yaml`.

Build and push the image set from the repository root with:

```bash
mkdir -p temp/build-tmp
TMPDIR="$PWD/temp/build-tmp" task image:all-images \
  IMAGE_TAG=bootc \
  REGISTRY_ADDRESS=8gears.container-registry.com \
  REGISTRY_PROJECT=8gcr-dev \
  PLATFORMS=linux/amd64
```

The build task publishes Trivy under its standard `trivy-adapter` repository.
Create the deployment alias after the build:

```bash
skopeo copy --all \
  docker://8gears.container-registry.com/8gcr-dev/trivy-adapter:bootc \
  docker://8gears.container-registry.com/8gcr-dev/harbor-trivy-adapter:bootc
```

## Quick Start

```bash
cd deployment/podman/harbor-bootc

cp .env.example .env
# The example values are local-dev defaults. Replace secrets before any shared use.

openssl genpkey -algorithm RSA -outform PEM -pkeyopt rsa_keygen_bits:4096 \
  | openssl rsa -traditional -out config/token_service_key.pem
chmod 644 config/token_service_key.pem

mkdir -p runtime/podman/root runtime/podman/runroot runtime/podman/tmp runtime/trivy-tmp runtime/grype-cache
chmod 1777 runtime/trivy-tmp

podman-compose \
  --podman-args "--root $(pwd)/runtime/podman/root --runroot $(pwd)/runtime/podman/runroot --tmpdir $(pwd)/runtime/podman/tmp" \
  --env-file .env \
  -f compose.yaml \
  up -d
```

The default endpoint is:

```text
http://hetzner-bootc.tail6c5ea9.ts.net:18085
```

The compose file binds the portal to `0.0.0.0:18085` by default so it is reachable through localhost and Tailscale. To restrict or change it, update these values in `.env`:

```env
BIND_ADDR=100.100.156.26
PORT_HTTP=18086
EXT_ENDPOINT=http://hetzner-bootc.tail6c5ea9.ts.net:18086
```

## Verify

```bash
curl -fsS http://hetzner-bootc.tail6c5ea9.ts.net:18085/api/v2.0/ping
curl -fsS -u 'admin:Harbor12345' http://hetzner-bootc.tail6c5ea9.ts.net:18085/api/v2.0/health | jq .
curl -fsS -u 'admin:Harbor12345' http://hetzner-bootc.tail6c5ea9.ts.net:18085/api/v2.0/scanners | jq .
podman login --tls-verify=false hetzner-bootc.tail6c5ea9.ts.net:18085 -u admin -p Harbor12345
```

## Stop

```bash
cd deployment/podman/harbor-bootc

podman-compose \
  --podman-args "--root $(pwd)/runtime/podman/root --runroot $(pwd)/runtime/podman/runroot --tmpdir $(pwd)/runtime/podman/tmp" \
  --env-file .env \
  -f compose.yaml \
  down
```

## Podman Differences From `deploy/compose`

This file keeps the Harbor Next service layout and healthchecks, but uses simple `depends_on` ordering instead of `condition: service_healthy`. On this host, `podman-compose` waited indefinitely on health conditions even while Harbor's `/api/v2.0/health` reported healthy.

The local `config/proxy.conf` also preserves `$http_host` so the Docker/Podman bearer-token challenge keeps the high port in `http://localhost:18085/service/token`.

## Bootc Test Flow

After startup, create or use a project and copy a bootc image into Harbor:

```bash
podman login --tls-verify=false localhost:18085 -u admin -p Harbor12345
skopeo copy docker://ghcr.io/ublue-os/bluefin:latest-20260704 docker://localhost:18085/library/bluefin-bootc:latest-20260704
```

Then trigger SBOM generation and a vulnerability scan in Harbor. The bootc deployment enables Grype as the default vulnerability scanner and keeps Trivy available for SBOM generation.

## Scanner Debug Tools

The scanner investigation harnesses live under:

```text
deployment/podman/harbor-bootc/tools/scanner-debug/
```

Run the Harbor acceptance loop:

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/scan-loop.sh
```

Generate and validate SPDX package inventories for the bootc matrix:

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/sbom-check.sh
```

Run the SBOM-derived Trivy probe:

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/trivy-sbom-force-os.sh
```

Run the Grype SBOM comparison:

```bash
deployment/podman/harbor-bootc/tools/scanner-debug/grype-sbom-scan.sh
```
