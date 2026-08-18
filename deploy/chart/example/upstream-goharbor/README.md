# Upstream goharbor images

Run the chart against the upstream goharbor images on Docker Hub
(`docker.io/goharbor/*`) instead of the default 8gcr Harbor Next builds.
[`values.yaml`](values.yaml) carries the full configuration.

The whole switch is one value:

```yaml
image:
  source: upstream
```

`image.source` selects the registry and the per-component repository for every
component. Upstream renames two images — the registry container is
`registry-photon` and the trivy adapter is `trivy-adapter-photon` — which the
preset handles for you (see the `harbor.image.sourceMap` helper for the map).
The image tag defaults to the chart `appVersion`.

## Deploy

```bash
# from the chart directory
helm install harbor . -n harbor -f example/upstream-goharbor/values.yaml
```

## Options

- **Per-component override** — set `<component>.image.registry` /
  `.repository` / `.tag` / `.digest` to override the preset for a single image.
- **Digest pinning** — set `<component>.image.digest: sha256:...` to pin by
  digest instead of tag (takes precedence over the tag).
- **Air-gapped mirror** — mirror `docker.io/goharbor/*` into your registry and
  set `global.imageRegistry: mirror.internal`. It overrides the host for every
  image while preserving the repository path, and wins over `image.source`. The
  bundled valkey subchart image is not covered — mirror and override it via the
  `valkey.*` values separately.
