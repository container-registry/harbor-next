# Harbor Development Environment

This directory contains all configuration files needed to run Harbor locally for development.

## Philosophy

**Containerized with Hot Reload**: All Harbor services run in Docker containers with Air hot reload. Services communicate via Docker network - no `localhost` issues.

## Quick Start

```bash
# Start full dev environment with Trivy scanner
task dev:up

# Start infrastructure only (for native development)
task dev:infra:up
```

## Architecture

All services run in containers:
- **Core** - Harbor API with Air hot reload (port 8080, Delve 2345)
- **JobService** - Background jobs with Air hot reload (port 8888, Delve 2346)
- **RegistryCtl** - Registry controller for storage operations (port 8085)
- **Trivy Adapter** - Vulnerability scanner (port 8081)
- **PostgreSQL** - Database (port 5432)
- **Redis/Valkey** - Cache/queue (port 6379)
- **Distribution** - Docker Registry (port 50000)

```SVGBob
                    ┌────────────────────────────────────────┐
                    │            Docker Network              │
                    │                                        │
┌──────────┐        │  ┌──────┐  ┌───────────┐  ┌───────┐    │
│  Portal  │◄───────┼─►│ Core │◄─│ JobService│◄─│ Trivy │    │
│ :4200    │        │  │:8080 │  │   :8888   │  │ :8081 │    │
└──────────┘        │  └──┬───┘  └─────┬─────┘  └───────┘    │
   (native)         │     │            │                     │
                    │  ┌──▼────────────▼──┐                  │
                    │  │    PostgreSQL    │                  │
                    │  │      :5432       │                  │
                    │  └──────────────────┘                  │
                    │  ┌──────────────────┐                  │
                    │  │      Redis       │                  │
                    │  │      :6379       │                  │
                    │  └──────────────────┘                  │
                    │  ┌──────────────────┐                  │
                    │  │    Registry      │                  │
                    │  │     :50000       │                  │
                    │  └──────────────────┘                  │
                    └────────────────────────────────────────┘
```

## Directory Structure

```
devenv/
├── docker-compose.yml         # All services (infra + backend + trivy)
├── air.core.toml              # Air hot-reload config for Core
├── air.jobservice.toml        # Air hot-reload config for JobService
├── jobservice.config.yml      # JobService configuration
├── registry.config.yml        # Docker Registry config
├── registry.passwd            # Registry HTTP basic auth credentials
└── README.md                  # This file

dockerfile/
└── dev.core.dockerfile        # Dev image with Go, Air, and Delve
```

## Development Commands

### Full Environment
```bash
task dev:up              # Start everything (Core, JobService, Trivy, Portal)
task dev:up:simple       # Start without Trivy
task dev:status          # Show running containers
task dev:logs            # View all container logs
task dev:clean           # Stop everything and remove volumes
```

### Backend Only
```bash
task dev:backend:up          # Start Core + JobService containers
task dev:backend:down        # Stop backend containers
task dev:backend:logs        # View backend logs
task dev:backend:logs:core   # View Core logs only
task dev:backend:restart     # Restart backend containers
```

### Infrastructure Only
```bash
task dev:infra:up        # Start PostgreSQL, Redis, Registry
task dev:infra:down      # Stop infrastructure
task dev:infra:remove    # Remove with volumes
```

### Frontend
```bash
task dev:frontend:native     # Start Angular with HMR (runs natively)
task dev:frontend:build      # Build frontend for production
task dev:frontend:test       # Run frontend tests in watch mode
task dev:frontend:test:once  # Run frontend tests once
```

### Database
```bash
task dev:db:shell        # Open PostgreSQL shell
task dev:db:migrate      # Run migrations
task dev:db:reset        # Reset to clean state
```

### Utilities
```bash
task dev:gen:private-key # Generate RSA private key for token signing
task dev:info            # Show current SLOT and port assignments
task dev:logs            # View all container logs
```

### Build & Test (see main README)
```bash
task build               # Build all binaries
task build:gen-apis      # Generate API server code from OpenAPI spec
task test                # Run all tests
task test:lint           # Run Go linters
task test:unit           # Run Go unit tests
task test:quick          # Quick validation (fast checks only)
```

## Hot Reload

Code changes are automatically detected and rebuilt:
- **Go files**: Air watches `src/` and rebuilds (~3 seconds)
- **Frontend**: Angular HMR (< 1 second)

Volume mounts with VirtioFS (Docker Desktop 4.x+) provide fast file system event propagation.

## Debugging

Both Core and JobService run under Delve debugger:
- **Core**: `localhost:2345`
- **JobService**: `localhost:2346`

Connect your IDE debugger to these ports. The services start immediately (`--continue` flag) - no need to wait for debugger attach.

## Service URLs

| Service    | URL                              |
|------------|----------------------------------|
| Portal     | http://localhost:4200/           |
| API        | http://localhost:8080/api/v2.0   |
| JobService | http://localhost:8888            |
| Trivy      | http://localhost:8081            |
| Registry   | http://localhost:50000           |

## Credentials

- **Harbor Admin**: `admin` / `Harbor12345`
- **PostgreSQL**: `postgres` / `root123`

## Troubleshooting

### First-time startup is slow
The first run downloads Go modules and builds the dev image. Subsequent runs use cached layers.

### Container can't find modules
The Go module cache is persisted in a Docker volume (`go-mod-cache`). If you see module errors, try:
```bash
task dev:clean
task dev:up
```

### Port already in use
Check for existing processes:
```bash
lsof -i :8080   # Core
lsof -i :8888   # JobService
lsof -i :4200   # Portal
```
