# Copilot instructions for Harbor

## Build, test, lint (Taskfile-first)
- List tasks: `task --list-all`
- Build:
  - `task build` (all binaries)
  - `task build:binary:core:linux-amd64` (single binary)
  - `task images` (all Docker images)
  - `task build:gen-apis` (regen Go API server from `api/v2.0/swagger.yaml`)
- Test/lint:
  - `task test` (all checks)
  - `task test:unit` or `cd src && go test -v -race -cover ./...`
  - `task test:lint` (golangci-lint)
  - `task test:lint:api` (Spectral lint for OpenAPI spec)
  - `task test:vuln-check` (govulncheck)
  - Frontend: `cd src/portal && npm run test`, `npm run lint`
- Run a single test:
  - Go unit: `cd src && go test ./path/to/pkg -run TestName`
  - Robot E2E: `robot --include <tag> -V /drone/tests/e2e_setup/robotvars.py /drone/tests/robot-cases/Group1-Nightly/Trivy.robot`

## High-level architecture
- **Core** (`src/core`): main Beego service; serves `/api/v2.0`, proxies `/v2` registry requests, and serves Portal static assets in prod.
- **API layer**: generated from `api/v2.0/swagger.yaml` into `src/server/v2.0/{models,restapi}`; business logic lives in `src/controller`, data access in `src/pkg` and `src/lib`.
- **JobService** (`src/jobservice`): background jobs (scan/replication/GC/retention) via Redis-backed work queue; communicates with Core over HTTP.
- **RegistryCtl** (`src/registryctl`): direct storage operations used by GC; **Registry** (distribution) stores blobs/manifests.
- **Portal** (`src/portal`): Angular UI; frontend API client is generated from Swagger.
- **Stores**: PostgreSQL for metadata, Redis/Valkey for cache/queue; Trivy adapter optional in dev.

## Key conventions
- Taskfile is the source of truth; avoid Makefiles except SQL migrations in `make/migrations/postgresql/`.
- Go module root is `src/`; run `go test`/`go build` from there.
- API changes must update `api/v2.0/swagger.yaml` and run:
  - `task build:gen-apis` (Go server code)
  - `task build:gen-apis:frontend` (Angular API client)
- Generated code lives in `src/server/v2.0/...` and `src/portal/ng-swagger-gen/...`; do not edit manually.
- DB migrations go in `make/migrations/postgresql/XXXX_description.up.sql`; apply with `task dev:db:migrate`.
- Mock generation uses `.mockery.yaml`; tasks `task test:generate-mocks` and `task test:check-mocks`.
