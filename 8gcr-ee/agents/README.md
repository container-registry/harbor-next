# Agent Automation

This directory owns 8gcr-specific agent orchestration.

- `agent.yml` is the Taskfile include exposed as `task agent:*` from the root `Taskfile.yml`.
- Runtime assets live under `.github/agents/` so GitHub automation stays grouped.
- The executable workflow entrypoint remains under `.github/workflows/` because GitHub Actions only discovers workflow files there.

## Tasks

- `task agent:prepare-handoff` collects failed workflow metadata and logs with host tools.
- `task agent:prepare-handoff-runtime` collects the same handoff inside the published agent image so self-hosted runners do not need `gh` or `jq` installed.
- `task agent:run-local-runtime` starts the published agent image with the handoff bundle mounted in.
- `task agent:local-run-failed-sync` runs the same path from a developer machine, using host `gh` auth if `AGENT_GITHUB_TOKEN` is unset.
- `task agent:build-image` and `task agent:push-image` rebuild/publish the runtime image when `.github/agents/image/` changes.

The root `Taskfile.yml` includes this file as optional so copied Harbor test harnesses can run without carrying private `8gcr-ee/agents/` files.
