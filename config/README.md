# Harbor Service Configs

Canonical configuration files shared across all deployment modes. These are the recommended production baseline.

## Files

| File | Service | Mounted to | Consumers |
|------|---------|------------|-----------|
| `jobservice.yml` | JobService | `/etc/jobservice/config.yml` | `deploy/compose/docker-compose.yaml`, `devenv/air.jobservice.toml` |
| `registry.yml` | Registry + RegistryCtl | `/etc/registry/config.yml` | `deploy/compose/docker-compose.yaml`, `devenv/docker-compose.yml` |
| `registryctl.yml` | RegistryCtl | `/etc/registryctl/config.yml` | `deploy/compose/docker-compose.yaml` |
| `nginx/proxy.conf` | Portal (nginx) | `/etc/nginx/proxy.d/proxy.conf` | `deploy/compose/docker-compose.yaml` |
| `nginx/tls.conf` | Portal (nginx) | `/etc/nginx/proxy.d/tls.conf` | `deploy/compose/docker-compose.yaml` (via `TLS_CONF`) |
| `portal/nginx.conf` | Portal (nginx) | `/etc/nginx/nginx.conf` | Baked into portal image at build time |

## Production Defaults

- Log levels: **INFO** everywhere (jobservice loggers, registryctl)
- Registry storage delete: **enabled** (required for garbage collection)
- Registry HTTP secret: placeholder — **must** be overridden via `REGISTRY_HTTP_SECRET` env var
- JobService protocol: HTTP (runs behind Core, not exposed externally)
- TLS: terminated at nginx via `nginx/tls.conf` — not at individual services

## Overriding for Dev

The dev environment (`devenv/docker-compose.yml`) mounts these same configs and uses environment variables for the few differences:

| Setting | Prod value | Dev override | Mechanism |
|---------|-----------|--------------|-----------|
| `storage.delete.enabled` | `true` | `false` | `REGISTRY_STORAGE_DELETE_ENABLED` env var (Distribution native) |

Log levels in dev are controlled per-service via `LOG_LEVEL` env var in `devenv/docker-compose.yml` (for Core, RegistryCtl) — but jobservice log level is read from the config file only (`loadEnvs()` doesn't cover it). To use DEBUG logging in jobservice, edit `config/jobservice.yml` locally.

## Overriding for Custom Deployments

Registry (Distribution) supports environment variable overrides using the `REGISTRY_` prefix with `_`-delimited path (e.g. `REGISTRY_STORAGE_FILESYSTEM_ROOTDIRECTORY`). See [Distribution configuration docs](https://distribution.github.io/distribution/about/configuration/).

JobService and RegistryCtl have limited env override support — most settings require editing the YAML files directly. To customize without modifying tracked files, copy the relevant config, edit it, and point your volume mount at the copy.
