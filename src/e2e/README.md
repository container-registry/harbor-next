# Harbor E2E Tests

The E2E suite runs Harbor through the dev environment, then runs the godog suite against `http://localhost:8080`.

```bash
task e2e
```

## Host Requirements

- `task`
- Go
- Bun
- Docker with Buildx
- `cosign`
- `curl`, `jq`, and `openssl`
- `npm` and `mvn` — the package scenarios drive the real ecosystem clients
- Network access to clone `https://github.com/container-registry/harbor-next.git`
- Network access to Maven Central, for the plugins `mvn deploy` resolves on a cold local repository

## What `task e2e` Does

1. Clones `harbor-next/main` into `tmp/harbor-next-e2e`.
2. Copies `8gcr-ee/patches` into that clone.
3. Applies the patch series with `git apply --3way`.
4. Copies the E2E harness, Taskfiles, Dockerfiles, config, and dev environment files into the patched clone.
5. Runs `task e2e:test` inside the patched clone.
6. Starts Harbor with the detached dev stack.
7. Waits for the Core API to become reachable.
8. Runs the godog suite with `go test -tags e2e`.
9. Stops the dev environment with `task dev:down`.

## Common Commands

Run the full suite:

```bash
task e2e
```

Run a tag subset:

```bash
task e2e TAGS='@smoke'
```

Increase the test timeout:

```bash
task e2e E2E_TIMEOUT=45m
```

Skip Trivy while starting the dev environment:

```bash
task e2e SKIP_TRIVY=true
```

Use an alternate dev slot when default ports are busy:

```bash
task e2e E2E_SLOT=1
```

Run only the dev-server/test phase from inside `tmp/harbor-next-e2e`:

```bash
task e2e:test
```

## Outputs

The prepared patched workspace is created at:

```text
tmp/harbor-next-e2e
```

Reports are written under:

```text
tmp/harbor-next-e2e/reports
```

The dev environment log is written to:

```text
tmp/harbor-next-e2e/reports/e2e-dev.log
```

CI copies those reports to `reports/` and uploads them as the E2E artifact.

## CI Model

`.github/workflows/e2e.yml` installs the host tools needed by `task dev:up`, then runs:

```bash
task e2e
```

All Harbor-specific orchestration stays in Task so local and CI behavior match.

## Troubleshooting

If Docker is not usable, verify the daemon:

```bash
docker info
```

If Harbor does not become healthy, inspect the dev log:

```bash
less tmp/harbor-next-e2e/reports/e2e-dev.log
```

If scenarios fail, inspect the godog JSON output:

```bash
less tmp/harbor-next-e2e/reports/e2e-test.json
```

For the detailed suite architecture and scenario contract, see `src/e2e/SPEC.md`.
