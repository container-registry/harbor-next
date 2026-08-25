# Authoritative-DB migration benchmark harness

Compares core startup / migration timing across DB-migration approaches: bare
`next/main` (baseline), `experiment/authoritative-db-single-file` (Way 1),
and `experiment/authoritative-db-versioned-file` (Way 2).

Real k3d/Helm 3-replica deployment wasn't available in this environment
(rootless podman lacks the `cpuset` cgroup delegation k3s requires — see
`docs/adr` or ask before attempting on a host without that fix). This harness
uses a 3-replica podman-compose stack instead: same concurrent-migration /
advisory-lock behavior, without k8s/Helm fidelity (no rolling restarts,
no Ingress, no HPA).

## Usage

```bash
bash scripts/gen-key.sh                                  # once, generates a gitignored token key
task image:core PUSH=false IMAGE_TAG=<variant-tag>        # build the core image for a variant/branch
bash scripts/run-variant.sh <variant-name> <variant-tag>  # full run: cold start, k6 load, pod-kill, DB sampling
bash scripts/collect-results.sh <variant-name> [<variant-name> ...]
```

Each `run-variant.sh` pass produces `results/<variant>/`:
- `cold-start-per-pod.csv` — time to healthy per replica after `compose up`
- `migration-phase-durations.csv` — per-replica `numbered`/`authoritative`/`versioned` phase timing, parsed from core logs
- `k6-cold.json`, `k6-warm.json` — k6 `--summary-export` aggregated stats (p95/p99 etc, not raw per-request samples)
- `pod-kill-recovery.csv` — time for a killed replica to become healthy again
- `conn-samples.csv`, `lock-samples.csv` — `pg_stat_activity`/`pg_locks` polled every 0.5s throughout

`collect-results.sh` normalizes all variants into `results/summary.csv`.
