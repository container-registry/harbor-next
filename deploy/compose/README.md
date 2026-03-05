# Harbor Production Compose

Minimal Docker Compose deployment for Harbor.

## Prerequisites

- Docker Engine 24+ with Compose v2.24+
- Pre-built Harbor images (see `task image:all-images`)

## Quick Start

```bash
cd deploy/compose

# 1. Configure environment
cp .env.example .env
# Edit .env — fill in HARBOR_TAG, EXT_ENDPOINT, and all secrets

# 2. Generate token signing key (must be PKCS#1 / "RSA PRIVATE KEY" format)
openssl genrsa -traditional -out config/token_service_key.pem 4096

# 3. Start
docker compose up -d

# 4. Verify
docker compose ps
curl http://localhost/api/v2.0/systeminfo
```

## TLS

To enable HTTPS termination at the portal:

1. Place your cert and key:
   ```bash
   mkdir -p certs
   cp /path/to/tls.crt certs/
   cp /path/to/tls.key certs/
   ```

2. Uncomment the TLS lines in `docker-compose.yaml` under the `portal` service:
   ```yaml
   volumes:
     - ./config/nginx/tls.conf:/etc/nginx/proxy.d/tls.conf:ro
     - ./config/certs/tls.crt:/etc/nginx/ssl/tls.crt:ro
     - ./config/certs/tls.key:/etc/nginx/ssl/tls.key:ro
   ports:
     - "${PORT_HTTPS:-443}:8443"
   ```

3. Set `EXT_ENDPOINT=https://your-domain` in `.env`.

## Architecture

Portal (nginx) serves the Angular UI and reverse-proxies `/v2/`, `/api/`, `/service/`, and `/c/` to Core. This eliminates the need for a separate nginx reverse proxy container.

## Verify Push/Pull

```bash
docker login localhost -u admin -p <your-admin-password>
docker tag alpine:latest localhost/library/alpine:test
docker push localhost/library/alpine:test
docker pull localhost/library/alpine:test
```
