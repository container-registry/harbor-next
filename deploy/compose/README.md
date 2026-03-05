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
chmod 644 config/token_service_key.pem

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

## Architecture

Portal (nginx) serves the Angular UI and reverse-proxies `/v2/`, `/api/`, `/service/`, and `/c/` to Core.

```text
                   :443 (HTTPS)  / :80 (HTTP)
                          │
                   ┌──────▼──────┐
                   │   Portal    │  nginx — static UI + reverse proxy
                   │  (nginx)    │  /v2/*, /api/*, /service/*, /c/* → core
                   └──────┬──────┘
                          │ :8080
                   ┌──────▼──────┐
                   │    Core     │  API gateway, auth, business logic
                   │             │
                   └──┬────┬───┬─┘
                      │    │   │
         ┌────────────┘    │   └───────────┐
         │ :8888           │ :5000         │ :8080
  ┌──────▼──────┐   ┌──────▼─────┐  ┌──────▼───────┐
  │ JobService  │   │  Registry  │  │ RegistryCtl  │
  │             │   │ (distrib.) │  │ (storage ops)│
  └─────────────┘   └─────┬──────┘  └──────┬───────┘
                          └─────────┬──────┘
                             ┌──────▼──────┐
                             │ registry-   │  Docker volume
                             │ data        │  /var/lib/registry
                             └─────────────┘

  ┌─────────────┐   ┌─────────────┐
  │ Trivy       │   │  Exporter   │  Prometheus /metrics
  │ Adapter     │   │             │
  └─────────────┘   └─────────────┘

  ── Infrastructure (shared by Core, JobService, Trivy, Exporter) ──

  ┌──────────────┐  ┌─────────────┐
  │ PostgreSQL   │  │   Redis     │  Valkey
  │        :5432 │  │       :6379 │
  └──────────────┘  └─────────────┘
```

## Verify Push/Pull

```bash
echo '<your-admin-password>' | docker login registry.example.com -u admin --password-stdin
docker tag alpine:latest registry.example.com/library/alpine:test
docker push registry.example.com/library/alpine:test
docker pull registry.example.com/library/alpine:test
```
