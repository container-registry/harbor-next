# Local Harbor

Minimal local deployment of Harbor.

## Prerequisites

- Docker Engine 24+ with Compose v2.24+
- Pre-built Harbor images (`task image:all-images PUSH=false`)

## Start

```bash
cd deploy/compose/example/local
cp .env.example .env
openssl genpkey -algorithm RSA -out ../../config/token_service_key.pem -pkeyopt rsa_keygen_bits:4096
docker compose -f ../../docker-compose.yaml --env-file .env up -d
```

Open http://localhost and login with `admin` / `Harbor12345`.

## Push / Pull

```bash
docker login localhost -u admin -p Harbor12345
docker tag alpine localhost/library/alpine:test
docker push localhost/library/alpine:test
docker pull localhost/library/alpine:test
```

## Stop

```bash
docker compose -f ../../docker-compose.yaml --env-file .env down        # keep data
docker compose -f ../../docker-compose.yaml --env-file .env down -v     # destroy volumes
```

## Optional: HTTPS with mkcert

For trusted local TLS, install [mkcert](https://github.com/FiloSottile/mkcert) (`brew install mkcert`) and generate certificates:

```bash
mkcert -install
mkdir -p certs
mkcert -cert-file certs/tls.crt -key-file certs/tls.key localhost 127.0.0.1 ::1
```

Update `.env`:
```
EXT_ENDPOINT=https://localhost
```

Start with both compose files — the local overlay adds TLS volumes and the HTTPS port:

```bash
docker compose -f ../../docker-compose.yaml -f docker-compose.yaml --env-file .env up -d
```
