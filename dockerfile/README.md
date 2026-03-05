# Harbor Dockerfiles

All Dockerfiles support multi-architecture builds (linux/amd64, linux/arm64). BuildKit sets `TARGETARCH` automatically.

## Images

| Image | Dockerfile | Base | Build |
|-------|-----------|------|-------|
| core | `core.dockerfile` | scratch | Pre-built binary at `bin/linux-${TARGETARCH}/core` |
| jobservice | `jobservice.dockerfile` | scratch | Pre-built binary |
| registryctl | `registryctl.dockerfile` | scratch | Pre-built binary |
| exporter | `exporter.dockerfile` | scratch | Pre-built binary |
| portal | `portal.dockerfile` | [DHI](#docker-hardened-images-dhi) nginx (debian13) | Multi-stage: Bun/Node Angular build → nginx |
| registry | `registry.dockerfile` | scratch | Multi-stage: Go build of distribution/distribution |
| trivy-adapter | `trivy-adapter.dockerfile` | aquasec/trivy | Multi-stage: Go build of harbor-scanner-trivy |

Dev Dockerfiles (`dev.core.dockerfile`, `dev.portal.dockerfile`) are used by the dev environment for hot reload.

## Building

```bash
# Single image via Taskfile
task image:core

# All images
task image:all-images

# Manual build (binary-based — build binary first)
task build:binary:core:linux-amd64
docker buildx build --platform linux/amd64 -t harbor-core:dev -f dockerfile/core.dockerfile .

# Manual build (multi-stage — builds from source)
docker buildx build --platform linux/amd64 -t harbor-portal:dev -f dockerfile/portal.dockerfile .
```

## Runtime Mounts

| Service | Required Mount | Source |
|---------|---------------|--------|
| Core | `/migrations` | `make/migrations/` (baked into image) |
| Core | `/icons`, `/views` | `src/core/views/` (baked into image) |
| Core | `/etc/core/token_service_key.pem` | Generated RSA key |
| Portal | `/etc/nginx/nginx.conf` | `config/portal/nginx.conf` (baked into image) |
| Portal | `/etc/nginx/proxy.d/*.conf` | Reverse proxy + TLS config (mounted at runtime) |
| JobService | `/etc/jobservice/config.yml` | Mounted at runtime |
| RegistryCtl | `/etc/registryctl/config.yml` | Mounted at runtime |
| Registry | `/etc/registry/config.yml` | Mounted at runtime |

## Docker Hardened Images (DHI)

`portal.dockerfile` uses `dhi.io/nginx` (via our proxy at `8gears.container-registry.com/dhi.io/nginx`) — a CIS-benchmark-compliant, non-root base image rebuilt on a regular cadence with CVE patches.

Pulling requires authentication against our proxy. Run `docker login 8gears.container-registry.com` before building.

## Design Decisions

- **scratch base** for Go services: no shell, no package manager, minimal attack surface. CA certificates copied from Alpine.
- **tmpfs for /tmp**: Scratch images have no writable filesystem. Services that need `/tmp` (core, jobservice, registryctl, exporter, registry) get it via `tmpfs` mounts in docker-compose, not baked into the image. This keeps images immutable and avoids layer bloat.
- **Non-root execution**: All images run as non-root. Scratch images create a `harbor` user (UID 10000). Portal runs as UID 65532 with GID 0 write access (OpenShift-compatible).
- **No debug images**: Use `docker debug <container>` (Docker Desktop) to attach a shell to scratch containers.
- **No make/photon**: These Dockerfiles replace the legacy `make/photon/` Dockerfiles entirely.
