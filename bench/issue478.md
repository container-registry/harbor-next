# Issue 478 benchmark notes

This harness uses an isolated Kind cluster and the upstream Harbor Helm chart
(`harbor` 1.19.1 / Harbor 2.15.1). The reported code path in this checkout is
unchanged: Jobservice reads the scanner response as a full string, unmarshals
it twice, and `IMAGE_SCAN` has no per-job concurrency limit.

## Configuration

- `kind-issue478.yaml` maps Harbor's HTTP NodePort to `localhost:18080`.
- `harbor-issue478-values.yaml` enables Core and Jobservice debug logs and
  applies a **512 MiB** Jobservice memory limit.
- `issue478-image/` creates a scratch image containing a static Go binary and
  an 8 MiB opaque payload (about 9.1 MiB in Harbor).
- The scanner adapter was built from
  `container-registry/harbor-scanner-trivy` tags `v0.39.1` and `v0.40.0`.

## Fast deployment

The benchmark environment can be deployed without manually creating a Kind
cluster, loading the locally built scanner image, or assembling a Helm command:

```sh
task --taskfile bench/Taskfile.yml deploy
```

This reuses a healthy `issue478` Kind cluster, loads the explicit local adapter
image (`localhost/issue478/trivy-adapter:v0.39.1` by default), and installs the
pinned Harbor chart with the debug and 512 MiB Jobservice settings. If the
release already exists, it preserves it and only verifies health. It prints the
local Harbor URL only after the API health endpoint is ready.

To explicitly apply chart values (for example, when switching scanner adapter
versions), use:

```sh
SCANNER_ADAPTER_VERSION=v0.40.0 task --taskfile bench/Taskfile.yml deploy:apply
```

For a clean run, use the explicit destructive task:

```sh
task --taskfile bench/Taskfile.yml deploy:fresh
```

It deletes only the dedicated `issue478` cluster and recreates it. Use
`deploy:status` for health and `deploy:down` to delete that benchmark cluster.

## Runs

Each run pushed four distinct manifests, then sent four concurrent scan
requests. Jobservice cgroup `memory.current` was sampled every second.

| Adapter | Reports | Vulnerability records/report | Peak Jobservice memory | OOM |
| --- | ---: | ---: | ---: | --- |
| v0.39.1 | 4 | 0 | 21.1 MiB | No |
| v0.40.0 | 4 | 0 | 37.5 MiB | No |

The absolute peaks are not a performance comparison: the adapter switch rolled
Core and Jobservice, so the v0.40.0 run began from a colder process. Both runs
completed within seconds, with all four conversion log entries and no restart
or OOM event.

## Conclusion

The requested small scratch/Go images cannot reproduce the incident. Trivy
reports zero vulnerabilities for them, while the incident requires a report
with about 5,250 vulnerability records. The results therefore neither confirm
nor refute the large-report memory claim; they establish a clean, repeatable
baseline and show that changing the scanner adapter alone is not enough to
trigger the failure with this workload.

Generated Core/Jobservice logs are intentionally not versioned because they
contain short-lived scanner robot credentials.
