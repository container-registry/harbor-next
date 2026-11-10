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

# 3. Generate the secret key Core uses to encrypt stored credentials (exactly 16 alphanumeric chars).
#    Generated once; see "Secret Key" below before touching an existing one.
[ -f config/secret_key ] || openssl rand -base64 32 | tr -dc 'A-Za-z0-9' | head -c 16 > config/secret_key
chmod 644 config/secret_key

# 4. Start
docker compose up -d

# 5. Verify
docker compose ps
curl https://registry.example.com/api/v2.0/systeminfo
```

## Secret Key

`config/secret_key` is the AES key Core uses to encrypt credentials stored in
the database (replication endpoints, scanner credentials, OIDC secrets). It
belongs to the database for the lifetime of the install: back it up together
with the `postgres-data` volume and never regenerate it while that volume
exists. With a different key Harbor cannot decrypt the stored credentials, the
same invariant the Helm chart documents for `secretKey` in
[`deploy/chart/docs/MIGRATION.md`](../chart/docs/MIGRATION.md).

**Upgrading an existing deployment.** Compose files before this one did not
mount the key, so an existing install has no `config/secret_key`. Create it once
with step 3 above before the first `docker compose up` on the new version, then
keep it. The mount is declared with `create_host_path: false`, so a missing
file fails the start instead of being created as an empty directory.

## PostgreSQL 18

The bundled database runs `postgres:18-alpine`. The `postgres-data` volume is
mounted at `/var/lib/postgresql` and the data lives in `18/docker` inside it,
the layout the PostgreSQL 18 image expects. The version is fixed in the compose
file and `POSTGRES_VERSION` is no longer read, because an older image on this
mount would ignore the existing data and start an empty database.

Swapping the image does not upgrade a data directory. On a volume created by an
earlier compose file (PostgreSQL 16, or 15 if you pinned it), the PostgreSQL 18
container refuses to start and logs that it found data from an older version.
Move the data over with a dump and restore before you update the compose file:

```bash
cd deploy/compose

# 1. With the OLD compose file: stop Harbor, keep only the database running
docker compose down
docker compose up -d --wait postgresql

# 2. Dump the Harbor database as the configured user (DB_USER, DB_NAME)
docker compose exec -T postgresql \
  sh -c 'pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB"' > harbor.sql
grep -q -- '-- PostgreSQL database dump complete' harbor.sql && echo "dump OK"

# 3. Remove the old volume, guarded so it only happens after a complete dump
project=$(docker compose config | sed -n 's/^name: //p')
docker compose down
grep -q -- '-- PostgreSQL database dump complete' harbor.sql \
  && docker volume rm "${project}_postgres-data"

# 4. Update to the new compose file, then restore into the fresh PostgreSQL 18
docker compose up -d --wait postgresql
docker compose exec -T postgresql \
  sh -c 'psql -v ON_ERROR_STOP=1 -q -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < harbor.sql > /dev/null \
  && echo "restore OK"

# 5. Start Harbor
docker compose up -d
```

Remove the old volume only after step 2 printed `dump OK`. Without it, keep
the old volume and the old compose file and look into the error first. If step
4 does not print `restore OK`, run `docker compose down`, remove the new volume
the same way as in step 3 and repeat step 4. Keep `harbor.sql` and
`config/secret_key` together until the upgraded Harbor works.

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

**Non-default HTTPS port.** Plain HTTP on `PORT_HTTP` redirects to HTTPS, and nginx
cannot read `PORT_HTTPS` from the environment. When `PORT_HTTPS` is not `443`, copy
`config/nginx/tls.conf`, set `$https_port` in the copy to the same port, and point
`TLS_CONF` at it:

```bash
mkdir -p config
sed 's/^set \$https_port 443;/set $https_port 8443;/' ../../config/nginx/tls.conf > config/tls.conf
# .env
PORT_HTTPS=8443
TLS_CONF=./config/tls.conf
```

## Image Repository

`.env.example` defaults to pulling from the remote registry (`8gears.container-registry.com/8gcr/`).
To use locally built images instead, set `IMAGE_REPO=` (empty) in `.env`.

Images resolve to `${IMAGE_REPO}harbor-core:${HARBOR_TAG}`, so the value must end with `/`.

Keep `HARBOR_TAG` in step with this compose file. Its portal healthcheck probes
`127.0.0.1:8081/healthz`, which older portal images do not serve, so an older
portal image stays `unhealthy` even though it works. Pull or build a portal image
from the same version when updating the compose file.

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
