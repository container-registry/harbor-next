# GitHub Agent Assets

Contained assets for the upstream sync resolver agent:

- `image/` builds the runtime image.
- `prompts/` contains the Codex prompt contract.
- `scripts/` contains the handoff, runtime, and entrypoint scripts.
- `workflows/` documents the GitHub workflow entrypoints.

The workflow entrypoint is `.github/workflows/agent-upstream-sync-resolver.yml`. GitHub requires workflow files to live under `.github/workflows/`, so only the entrypoint stays outside this directory.

## Manual Smoke Check

The workflow can run a manual `smoke` job from `workflow_dispatch`. It validates shell syntax, verifies the `task agent:*` include, logs into Harbor with `REGISTRY_USERNAME` / `REGISTRY_PASSWORD`, and pulls `AGENT_IMAGE`.

The smoke job intentionally does not run Codex or create an agent PR.

The production `resolve` job prepares its failed-run handoff inside `AGENT_IMAGE`, not on the host runner. This keeps the runner contract small: Docker or Podman plus Task are enough; `gh` and `jq` come from the agent image.

For failed `sync:merge-upstream` runs, the resolver creates a sync-merge PR from the agent workflow instead of letting `Sync and Verify Patches` resolve non-workflow conflicts inline. Patch-stack rebase failures still use the Codex/StGit path and only export `8gcr-ee/patches/` artifacts.
