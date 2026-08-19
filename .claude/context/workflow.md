# Development Workflow

`task --list-all` is authoritative. Common ones:

| Goal | Command |
|------|---------|
| Full dev env (Core+Job+RegCtl+Trivy+Portal) | `task dev:up` |
| Infra only (PG/Valkey/Registry) | `task dev:infra:up` |
| Backend hot-reload | `task dev:backend:up` |
| Frontend HMR | `task dev:frontend:native` (port 4200) |
| Build all binaries / images | `task build` / `task images` |
| Build one image | `task image:core` |
| All tests | `task test` |
| Go unit + race | `task test:unit` (or `cd src && go test ./...`) |
| Go lint | `task test:lint` (`.golangci.yaml`: bodyclose, errcheck, goheader, govet, ineffassign, misspell, revive, staticcheck, whitespace) |
| OpenAPI lint | `task test:lint:api` (Spectral) |
| Vuln check | `task test:vuln-check` |
| Frontend lint/test | `task test:frontend:lint` / `task test:frontend:test` |
| DB migrate / reset / shell | `task dev:db:migrate` / `dev:db:reset` / `dev:db:shell` |
| Dev port info (SLOT-aware) | `task dev:info` |

See `QUICKSTART.md` for setup.

## Hard rules

- **API-first:** any REST change starts in `api/v2.0/swagger.yaml`, then `task build:gen-apis` regenerates `src/server/v2.0/restapi/` and `src/server/v2.0/handler/`. Never hand-edit the generated server.
- **Schema change → migration:** new file in `make/migrations/postgresql/` (next `XXXX_` sequence).
- **Mocks:** generated from `.mockery.yaml` into `src/testing/`.
- **Cross-arch image builds locally:** `IMAGE_PLATFORMS` only takes effect when `IS_CI=true`. To build multi-arch from an ARM Mac for an amd64 cluster, override `PLATFORMS` directly: `task image:all-images PLATFORMS=linux/amd64,linux/arm64 IMAGE_REGISTRY=ttl.sh IMAGE_NAMESPACE=harbor-next`.

## Common task recipes

- **New API endpoint:** edit swagger → `task build:gen-apis` → implement controller in `src/controller/` → integration test in `tests/`.
- **New job:** add under `src/jobservice/job/impl/<name>/` implementing `job.Interface` (see `src/jobservice/job/interface.go`) → trigger from a controller via `src/pkg/task/`.
- **Frontend change:** work under `src/portal/`; lint + test before building.
