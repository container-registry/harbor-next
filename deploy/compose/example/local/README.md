# Local Harbor with TLS (mkcert)

Zero-config local deployment with HTTPS using [mkcert](https://github.com/FiloSottile/mkcert) for trusted local certificates.

## Prerequisites

- Docker Engine 24+ with Compose v2.24+
- [mkcert](https://github.com/FiloSottile/mkcert) (`brew install mkcert`)
- Pre-built Harbor images (`task image:all-images PUSH=false`)

## Start

```bash
cd deploy/compose/example/local
./setup.sh
docker compose up -d
```

Open https://localhost and login with `admin` / `Harbor12345`.

## Push / Pull

```bash
docker login localhost -u admin -p Harbor12345
docker tag alpine localhost/library/alpine:test
docker push localhost/library/alpine:test
docker pull localhost/library/alpine:test
```

## Stop

```bash
docker compose down        # keep data
docker compose down -v     # destroy volumes
```

## Clean generated files

```bash
./setup.sh clean
```

## How it works

`setup.sh` generates:
- TLS certificates via mkcert (trusted by your OS)
- RSA token signing key for Docker token auth
- `.env` with secrets and `COMPOSE_FILE` pointing to the parent compose + local TLS override

The override compose (`docker-compose.yaml`) adds TLS cert/key volumes and the HTTPS port to the portal service. Everything else comes from the parent `deploy/compose/docker-compose.yaml`.
