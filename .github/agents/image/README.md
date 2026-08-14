# 8gcr Agent Image

This image is the isolated runtime for the upstream sync conflict resolver.

It includes:

- `git`
- `gh`
- `codex`
- `task`
- `stg`
- `podman`
- `buildah`
- `skopeo`
- `regctl`, `regsync`, `regbot`
- `oras`
- `crane`
- `kubectl`
- `trivy`
- Go
- Node/npm
- common shell/build utilities

Build and push the agent image:

```bash
task agent:build-image AGENT_IMAGE=8gears.container-registry.com/8gcr-dev/agent:latest
task agent:push-image AGENT_IMAGE=8gears.container-registry.com/8gcr-dev/agent:latest
```

The GitHub Actions workflow logs in to the registry, pulls this image, and runs it:

```text
AGENT_IMAGE=8gears.container-registry.com/8gcr-dev/agent:latest
```

The `container-registry` runner currently uses Docker, so the default runtime backend is:

```text
AGENT_RUNTIME_BACKEND=docker
```

Codex auth is not baked into the image. Provide one of these at runtime:

- `CODEX_API_KEY`
- `CODEX_AUTH_JSON`, which is written to `/codex-home/auth.json`

Containerfile path:

```text
.github/agents/image/Containerfile
```

Run the current failed sync locally and create a draft PR:

```bash
export CODEX_AUTH_JSON="$(cat ~/.codex/auth.json)"

task agent:local-run-failed-sync \
  AGENT_IMAGE=8gears.container-registry.com/8gcr-dev/agent:latest \
  AGENT_LOCAL_FAILED_RUN_ID=26220417019 \
  AGENT_LOCAL_REPOSITORY=container-registry/8gcr \
  AGENT_LOCAL_TARGET_BRANCH=main
```

The local run prepares the handoff bundle, downloads failed workflow metadata/logs, runs Codex in the local container, pushes an `agent/upstream-sync/main/<run-id>` branch, and opens a draft PR.

For local testing, `task agent:local-run-failed-sync` uses `AGENT_GITHUB_TOKEN` when set, otherwise it falls back to the host `gh auth token`. If Codex envs are unset, it falls back to host `~/.codex/auth.json`.
