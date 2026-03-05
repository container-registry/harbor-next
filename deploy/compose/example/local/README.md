# Local Harbor

Minimal local deployment of Harbor. TLS is disabled via `/dev/null` mounts in `.env.example`.

## Prerequisites

- Docker Engine 24+ with Compose v2.24+
- Pre-built Harbor images (`task image:all-images`) or pull from remote registry

## Quick Start (Taskfile)

```bash
task dev:release:up                                                # local images
task dev:release:up IMAGE_REPO=8gears.container-registry.com/8gcr/ # remote images
task dev:release:up TAG=v2.14.0                                    # specific tag
```

Runs in foreground, auto-cleans on Ctrl+C. To stop manually:

```bash
task dev:release:down          # keep data
task dev:release:down:clean    # remove volumes
```

## Manual Start

```bash
cd deploy/compose/example/local
cp .env.example .env
openssl genpkey -algorithm RSA -outform PEM -pkeyopt rsa_keygen_bits:4096 \
  | openssl rsa -traditional -out ../../config/token_service_key.pem
docker compose -f ../../docker-compose.yaml --env-file .env up -d
```

Open http://localhost and login with `admin` / `Harbor12345`.

## Push / Pull Images

TLS is disabled, so Docker requires the explicit port to use HTTP:

```bash
echo 'Harbor12345' | docker login localhost:80 -u admin --password-stdin
docker tag alpine localhost:80/library/alpine:test
docker push localhost:80/library/alpine:test
docker pull localhost:80/library/alpine:test
```

## Stop

```bash
docker compose -f ../../docker-compose.yaml --env-file .env down        # keep data
docker compose -f ../../docker-compose.yaml --env-file .env down -v     # destroy volumes
```

## Optional: HTTPS with mkcert

For trusted local TLS, install [mkcert](https://github.com/FiloSottile/mkcert) (`brew install mkcert`):

```bash
mkcert -install
mkdir -p certs
mkcert -cert-file certs/tls.crt -key-file certs/tls.key localhost 127.0.0.1 ::1
```

Update `.env`:

```bash
EXT_ENDPOINT=https://localhost
TLS_CONF=./config/nginx/tls.conf
TLS_CERT=./example/local/certs/tls.crt
TLS_KEY=./example/local/certs/tls.key
```
