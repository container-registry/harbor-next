# container-registry/harbor-next: Go services (core, jobservice, registryctl, exporter, db migrator) and the Angular portal.
FROM oven/bun:1.4.0 AS bun
# dockerfile/portal.dockerfile builds the portal with Alpine's Node 22, not NODE_VERSION from versions.env.
FROM node:22.22.3-bookworm AS node

FROM golang:1.27.2-bookworm
COPY --from=bun /usr/local/bin/bun /usr/local/bin/bun
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -sf /usr/local/lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -sf /usr/local/lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx

# PostgreSQL 16 matches POSTGRES_VERSION in versions.env; Redis stands in for Valkey. Several tests hardcode
# redis://localhost:6379. Chromium runs the karma portal tests.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git make gcc libc6-dev \
      postgresql-common redis-server chromium \
    && /usr/share/postgresql-common/pgdg/apt.postgresql.org.sh -y \
    && apt-get install -y --no-install-recommends postgresql-16 \
    && rm -rf /var/lib/apt/lists/*

RUN archive="task_linux_$(dpkg --print-architecture).tar.gz" \
    && curl -fsSL "https://github.com/go-task/task/releases/download/v3.49.1/${archive}" -o "/tmp/${archive}" \
    && curl -fsSL https://github.com/go-task/task/releases/download/v3.49.1/task_checksums.txt -o /tmp/task_checksums.txt \
    && (cd /tmp && grep "  ${archive}\$" task_checksums.txt | sha256sum -c -) \
    && tar -xzf "/tmp/${archive}" -C /usr/local/bin task \
    && rm -f "/tmp/${archive}" /tmp/task_checksums.txt

RUN pg_ctlcluster 16 main start \
    && su postgres -c "psql -c \"ALTER USER postgres PASSWORD 'root123'\"" \
    && su postgres -c "createdb registry" \
    && pg_ctlcluster 16 main stop
RUN printf '%s\n' '#!/bin/sh' 'set -e' \
      'pg_ctlcluster 16 main status >/dev/null 2>&1 || pg_ctlcluster 16 main start' \
      'redis-cli ping >/dev/null 2>&1 || redis-server --daemonize yes --port 6379 --save "" --appendonly no' \
      'timeout 60 sh -c "until pg_isready -h localhost -q; do sleep 0.5; done"' \
      'timeout 30 sh -c "until redis-cli ping >/dev/null 2>&1; do sleep 0.5; done"' \
      'echo "postgres on localhost:5432 (postgres/root123, db registry), redis on localhost:6379"' \
      > /usr/local/bin/harbor-test-services \
    && chmod +x /usr/local/bin/harbor-test-services

# Same variables as the test-db job in .github/workflows/test.yml.
ENV POSTGRESQL_HOST=localhost \
    POSTGRESQL_PORT=5432 \
    POSTGRESQL_USR=postgres \
    POSTGRESQL_PWD=root123 \
    POSTGRESQL_DATABASE=registry \
    POSTGRES_MIGRATION_SCRIPTS_PATH=/src/make/migrations/postgresql \
    JOB_SERVICE_POOL_REDIS_URL=redis://localhost:6379 \
    _REDIS_URL_REG=redis://localhost:6379/1 \
    _REDIS_URL_CORE=redis://localhost:6379/0 \
    _REDIS_URL_HARBOR=redis://localhost:6379/0 \
    REDIS_HOST=localhost \
    REDIS_PORT=6379 \
    HARBOR_ADMIN=admin \
    HARBOR_ADMIN_PASSWD=Harbor12345 \
    CORE_SECRET=tempString \
    UTTEST=true \
    GOFLAGS=-buildvcs=false \
    CHROME_BIN=/usr/bin/chromium \
    TOKEN_PRIVATE_KEY_PATH=/src/tests/private_key.pem

RUN go install github.com/go-swagger/go-swagger/cmd/swagger@v0.33.1

# scanner contract: the checkout lives inside the image, at /src
# Dockerfile.dockerignore replaces the root .dockerignore, which drops tests/ fixtures such as tests/private_key.pem.
COPY . /src
WORKDIR /src

# Same arguments as `task build:gen-apis`; the server code under src/server/v2.0 is generated, not committed.
RUN swagger generate server --template-dir=./tools/swagger/templates --exclude-main \
      --additional-initialism=CVE --additional-initialism=GC --additional-initialism=OIDC \
      -f ./api/v2.0/swagger.yaml -A harbor --target ./src/server/v2.0

WORKDIR /src/src
RUN go mod download
RUN go build -o /usr/local/bin/harbor-core ./core \
    && go build -o /usr/local/bin/harbor-jobservice ./jobservice \
    && go build -o /usr/local/bin/harbor-registryctl ./registryctl \
    && go build -o /usr/local/bin/harbor-exporter ./cmd/exporter \
    && go build -o /usr/local/bin/harbor-db-migrator ./cmd/standalone-db-migrator

# Compile every test binary, with and without the db tag, while online so `go test` works offline. -exec /bin/true
# skips TestMain, which needs PostgreSQL; running the tests needs harbor-test-services.
RUN go test -count=1 -exec /bin/true ./... >/dev/null
RUN go test -tags db -count=1 -exec /bin/true ./... >/dev/null

# Same steps as dockerfile/portal.dockerfile; postinstall generates the TypeScript API client from swagger.yaml.
WORKDIR /src/src/portal
RUN bun install --frozen-lockfile \
    && bun run generate-build-timestamp \
    && node --max_old_space_size=2048 node_modules/@angular/cli/bin/ng build --configuration production
WORKDIR /src/src/portal/app-swagger-ui
RUN bun install --ignore-scripts && bun run build

WORKDIR /src
