# Agent Workflows

GitHub Actions only discovers executable workflows in `.github/workflows/`.

The upstream sync resolver entrypoint therefore stays at:

```text
.github/workflows/agent-upstream-sync-resolver.yml
```

That workflow delegates implementation to:

```text
8gcr-ee/agents/agent.yml
.github/agents/scripts/
.github/agents/prompts/
.github/agents/image/
```
