# Harbor Dockerfiles

This directory contains Dockerfiles for building Harbor component images.

## Overview

All Dockerfiles support multi-architecture builds (linux/amd64, linux/arm64). BuildKit sets `TARGETARCH` automatically per platform, so you don't need to pass it manually.

**Image Types:**
- **Production images**: Use scratch/minimal base images for security and size
- **Debug support**: Use `docker debug` or `podman debug` instead of separate debug images

## Components

### Binary-Based Services (Production - FROM scratch)

These services use pre-built Go binaries with minimal base images:

- **`core.dockerfile`** - Harbor Core API service (scratch + CA certs + binary + runtime deps)
- **`jobservice.dockerfile`** - Background job processing service (scratch + CA certs + binary)
- **`registryctl.dockerfile`** - Registry controller service (scratch + CA certs + binary)
- **`exporter.dockerfile`** - Prometheus metrics exporter (scratch + CA certs + binary)

**Build Context Requirements:**
- Binary must be at: `bin/linux-${TARGETARCH}/<service-name>` (BuildKit sets `TARGETARCH`)
- Example: `bin/linux-amd64/core`

**Build Command Example:**
```bash
# Build binary first
task build:binary:core:linux-amd64

# Build image (TARGETARCH is set by BuildKit)
docker buildx build \
  --platform linux/amd64 \
  -t goharbor/harbor-core:dev \
  -f dockerfile/core.dockerfile \
  .
```

### Multi-Stage Build Services

These services include build stages:

- **`portal.dockerfile`** - Angular frontend (Node.js + Bun build → nginx:alpine)
  - Builds Angular app with Swagger UI
  - Final stage: `nginx:${NGINX_VERSION}-alpine`
  - Includes CA certificates

- **`registry.dockerfile`** - Docker Distribution registry (golang:alpine build → scratch)
  - Clones and builds distribution/distribution (version from `DISTRIBUTION_VERSION`)
  - Includes CVE-2025-22872 fix
  - Final stage: scratch with CA certs
  - Sets OTEL_TRACES_EXPORTER=none

- **`trivy-adapter.dockerfile`** - Trivy vulnerability scanner (golang build → aquasec/trivy base)
  - Builds harbor-scanner-trivy (version from `HARBOR_SCANNER_TRIVY_VERSION`)
  - Downloads trivy binary (version from `TRIVY_VERSION`)
  - Final stage: aquasec/trivy base image (version from `TRIVY_BASE_IMAGE_VERSION`)

- **`nginx.dockerfile`** - Nginx reverse proxy ([DHI](#docker-hardened-images-dhi) base)
  - Uses `dhi.io/nginx:${NGINX_VERSION}-debian13` (non-root, CIS-compliant)
  - Runs as UID 65532 (nginx); OpenShift-compatible (GID 0 write access)
  - No custom config in image (config provided at runtime)

**Build Command Example:**
```bash
docker buildx build \
  --platform linux/amd64 \
  -t goharbor/harbor-portal:dev \
  -f dockerfile/portal.dockerfile \
  .
```

## Debugging

Instead of maintaining separate debug images, use Docker/Podman debug tools:

**Docker Debug:**
```bash
# Start container
docker run -d --name harbor-core goharbor/harbor-core:dev

# Attach debugger
docker debug harbor-core
```

**Podman Debug (experimental):**
```bash
# Start container
podman run -d --name harbor-core goharbor/harbor-core:dev

# Debug (if available in your podman version)
podman debug harbor-core
```

## Directory Structure

```
dockerfile/
├── README.md                    # This file
├── core.dockerfile              # Core service (scratch base)
├── jobservice.dockerfile        # Job service (scratch base)
├── registryctl.dockerfile       # Registry controller (scratch base)
├── exporter.dockerfile          # Metrics exporter (scratch base)
├── portal.dockerfile            # Angular frontend (nginx:${NGINX_VERSION}-alpine)
├── registry.dockerfile          # Docker registry (scratch base)
├── trivy-adapter.dockerfile     # Trivy scanner (aquasec/trivy base)
└── nginx.dockerfile             # Nginx proxy (dhi.io/nginx:${NGINX_VERSION}-debian13)
```

## Usage with Taskfile

These Dockerfiles are used by Taskfile tasks in `taskfile/image.yml`:

```bash
# Build single image
task image:core:linux-amd64

# Build all images
task image:all-images
```

## Runtime Dependencies

Some services require additional files:

- **Core**: `/migrations` (from `make/migrations`), `/icons`, `/views` (from `src/core/views`)
- **Portal**: `config/portal/nginx.conf` (nginx configuration)
- **Jobservice**: Config mounted at `/etc/jobservice/config.yml`
- **Registryctl**: Config mounted at `/etc/registryctl/config.yml`
- **Registry**: Config mounted at `/etc/registry/config.yml`

## Differences from make/photon

These Dockerfiles **do not use** the legacy Dockerfiles in `make/photon/`. Key differences:

- ✅ Uses modern multi-stage Dockerfile patterns
- ✅ Production images use **scratch** base (not Alpine) for security
- ✅ Support for multi-architecture builds (linux/amd64, linux/arm64)
- ✅ Optimized layer caching
- ✅ Minimal attack surface (scratch base = no shell, no package manager)
- ✅ Clear separation of build and runtime stages
- ✅ Use docker/podman debug instead of debug images

## Image Base Summary

| Component | Base Image | Size | Notes |
|-----------|------------|------|-------|
| core | scratch | Minimal | CA certs + binary + deps |
| jobservice | scratch | Minimal | CA certs + binary |
| registryctl | scratch | Minimal | CA certs + binary |
| exporter | scratch | Minimal | CA certs + binary |
| portal | nginx:${NGINX_VERSION}-alpine | ~50MB | Includes built Angular app |
| registry | scratch | Minimal | CA certs + registry binary |
| trivy-adapter | aquasec/trivy (TRIVY_BASE_IMAGE_VERSION) | ~400MB | Includes trivy scanner |
| nginx | dhi.io/nginx:${NGINX_VERSION}-debian13 | ~80MB | [DHI](#docker-hardened-images-dhi), non-root, CIS-compliant |

## Docker Hardened Images (DHI)

The `nginx.dockerfile` uses a base image from `dhi.io`, a commercial registry providing Docker Hardened Images. These are CIS-benchmark-compliant, non-root images rebuilt on a regular cadence with CVE patches.

**For CI / contributors:**
- Pulling from `dhi.io` requires authentication. Set `DHI_USERNAME` and `DHI_PASSWORD` and run `docker login dhi.io` before building.
- If you don't have DHI credentials, you can substitute `nginx:${NGINX_VERSION}-alpine` for local development, but the production image intentionally uses DHI for its hardened, non-root configuration.

## Notes

- **Scratch base images** have no shell, no package manager - maximum security
- CA certificates are copied from Alpine for HTTPS support
- Binary-based images expect binaries to be built before image build
- Multi-stage builds happen entirely in Docker (no pre-built binaries needed)
- Portal requires Bun (version from `BUN_VERSION`) for faster builds
- Registry includes CVE-2025-22872 security fix
- All images use modern multi-stage build patterns
