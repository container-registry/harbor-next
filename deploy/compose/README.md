# Harbor Compose

Minimal Docker Compose deployment for Harbor

## Prerequisites

- Docker Engine 24+ with Compose v2.24+
- Harbor images — pulled from `8gears.container-registry.com/8gcr/` by default, or built locally with `task image:all-images`

## Quick Start

```bash
cd deploy/compose

# 1. Configure environment
cp .env.example .env
# Edit .env — set EXT_ENDPOINT, TLS_CERT, TLS_KEY, and all secrets

# 2. Generate token signing key (must be PKCS#1 / "RSA PRIVATE KEY" format)
openssl genpkey -algorithm RSA -outform PEM -pkeyopt rsa_keygen_bits:4096 \
  | openssl rsa -traditional -out config/token_service_key.pem
chmod 644 config/token_service_key.pem   # container UID 10000 ≠ host UID → needs world-readable

# 3. Start
docker compose up -d

# 4. Verify
docker compose ps
curl https://registry.example.com/api/v2.0/systeminfo
```

## TLS Certificates

TLS is enabled by default. Set `TLS_CERT` and `TLS_KEY` in `.env` to absolute paths on the host:

```bash
TLS_CERT=/etc/letsencrypt/live/registry.example.com/fullchain.pem
TLS_KEY=/etc/letsencrypt/live/registry.example.com/privkey.pem
```

**Obtain certificates with Let's Encrypt:**

```bash
certbot certonly --standalone -d registry.example.com
```

**Or generate a self-signed certificate for testing:**

```bash
openssl req -x509 -nodes -days 365 -newkey rsa:4096 \
  -keyout config/certs/tls.key -out config/certs/tls.crt \
  -subj "/CN=registry.example.com"
```

When using self-signed certs, Docker clients require `--insecure-registry` or the CA must be trusted on each host.

## Image Repository

`.env.example` defaults to pulling from the remote registry (`8gears.container-registry.com/8gcr/`).
To use locally built images instead, set `IMAGE_REPO=` (empty) in `.env`.

Images resolve to `${IMAGE_REPO}harbor-core:${HARBOR_TAG}`, so the value must end with `/`.

## Registry Proxy

Set `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` in `.env` when the Registry
service must reach upstream registries through a proxy. Include Harbor's
internal service names in `NO_PROXY`, for example:

```env
HTTP_PROXY=http://proxy.example.com:8080
HTTPS_PROXY=http://proxy.example.com:8080
NO_PROXY=localhost,127.0.0.1,core,registry,registryctl
```

## Architecture

Portal (nginx) serves the Angular UI and reverse-proxies `/v2/`, `/api/`, `/service/`, and `/c/` to Core.

<!--
```SVGBob
                          ":443 (HTTPS)  /  :80 (HTTP)"
                                      │
                                      ▼
                              ┌────────────────┐
                              │    "Portal"    │
                              │    "(nginx)"   │
                              └───────┬────────┘
                                      │
                                      ▼
                              ┌────────────────┐
                              │     "Core"     │
                              │    ":8080"     │
                              └──┬─────┬─────┬─┘
                  ┌──────────────┘     │     └──────────────┐
                  ▼                    ▼                    ▼
          ┌───────────────┐   ┌───────────────┐   ┌────────────────┐
          │ "JobService"  │   │  "Registry"   │   │ "RegistryCtl"  │
          │    ":8888"    │   │    ":5000"    │   │    ":8080"     │
          └───────────────┘   └───────┬───────┘   └───────┬────────┘
                                      └───────────┬───────┘
                                                  │
                                                  ▼
                                          ┌────────────────┐
                                          │"registry-data" │
                                          │   "(volume)"   │
                                          └────────────────┘

          ┌───────────────┐   ┌───────────────┐
          │    "Trivy"    │   │  "Exporter"   │
          │   "Adapter"   │   │               │
          └───────────────┘   └───────────────┘

          ┌───────────────┐   ┌───────────────┐
          │ "PostgreSQL"  │   │    "Redis"    │
          │    ":5432"    │   │    ":6379"    │
          └───────────────┘   └───────────────┘
```
-->
![Diagram](https://kroki.io/svgbob/svg/eNpTUMAFlKxMTIwVNDxCQgKCNRUU9BUUrCwMIHxNJS4FosCjKU3Eqpy2h4uQWT2PpjSQiCYQNLQJ7NeA_KKSxBwl4twM06SRl56ZV6GpRKSmKTjcuAa382cMs3BWUHLOL0pVUiAtnIHpzsKAeE1TcAcsLhFs4UxiOMxA8Sfu2CY6-IAxhSMCCccr6XE4gY660DwMCjMlr_yk4NSissxkUOqAhCKYVApKTc8sLimqVFJAFocLO5fkKKEnC0S6AQIlBRSNYHFTAwMDbOK40hmJsQlNDaTn-Bm0LyeI8cgaCm0hr2QirZSieYmF5gmlImiC001JLElUItFfkBSmpFGWn1Oam0pkjUF6xGHE10AUCVgyYkhRZlkleoZTcq0oANa6qUVKCtgzsJJjSmIBQh6pdMWVvKidTwd1wALbLMUl6UWpwYE-SuglWVBqSmaxkoICzpLR1MTYCFsJaGZsbqk0gAELAEZlQZE=)

- **Portal (nginx)** — serves the static Angular UI and reverse-proxies `/v2/*`, `/api/*`, `/service/*`, and `/c/*` to Core.
- **Core** (`:8080`) — API gateway, authentication, and business logic.
- **JobService** (`:8888`) — asynchronous jobs (scan, replication, GC, retention).
- **Registry** (`:5000`) — `docker/distribution`; blob/manifest storage.
- **RegistryCtl** (`:8080`) — direct storage operations (used by GC).
- **registry-data** — Docker volume mounted at `/var/lib/registry`, shared by Registry and RegistryCtl.
- **Trivy Adapter** — vulnerability scanner. **Exporter** — Prometheus `/metrics`.
- **Infrastructure** — PostgreSQL (`:5432`) and Redis/Valkey (`:6379`) are shared by Core, JobService, Trivy, and Exporter.

## Verify Push/Pull

```bash
echo '<your-admin-password>' | docker login registry.example.com -u admin --password-stdin
docker tag alpine:latest registry.example.com/library/alpine:test
docker push registry.example.com/library/alpine:test
docker pull registry.example.com/library/alpine:test
```
