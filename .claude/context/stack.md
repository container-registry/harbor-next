# Tech Stack

## Backend (Go)
- Module: `github.com/goharbor/harbor/src`. Go version pinned in `versions.env` (with `godebug x509negativeserial=1`).
- Web: Beego v2 (`github.com/beego/beego/v2`).
- API: go-swagger — REST handlers auto-generated from `api/v2.0/swagger.yaml`.
- ORM: custom (`src/lib/orm`) on top of `github.com/lib/pq`.
- Migrations: golang-migrate, files in `make/migrations/postgresql/` named `XXXX_version_description.up.sql`.
- AuthZ: Casbin RBAC.
- Cache: pluggable (`src/lib/cache/{memory,redis}`); Redis client `gomodule/redigo`.
- Job queue: **Harbor's fork** `github.com/goharbor/work v0.5.1-patch` (gocraft/work + Harbor patches). Don't assume upstream gocraft/work behavior.
- Logging: `github.com/goharbor/harbor/src/lib/log`. Use `log.Infof/Warningf/Errorf/Debugf` — never `log.Printf` or `fmt.Printf`. Safe to call before init.
- Other: `docker/distribution` (registry), `helm.sh/helm/v3` (charts).

## Frontend
- Angular 16 + Clarity Design System, SCSS, Karma/Jasmine. Source in `src/portal/`.

## Build & deploy
- Taskfile only — **never write Makefiles**. The only surviving Make artifact is `make/migrations/postgresql/`.
- Versions are centralized in `versions.env` (loaded by Taskfile dotenv). Tools patch-pinned, base images minor-pinned.
- Deploy targets: Docker Compose, Helm chart.
