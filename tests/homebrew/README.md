# Homebrew Bottle Compatibility Fixture

This fixture demonstrates that Harbor can store Homebrew bottles as OCI
artifacts and that `brew` can install a bottle from Harbor.

The demo uses `harbor-cli` `0.0.23` from `homebrew/core` because it is a real
Homebrew bottle published to GHCR with Linux and macOS bottle variants.

## Build The Client Image

```bash
tests/homebrew/build-image.sh
```

Equivalent manual command:

```bash
docker build \
  -f tests/homebrew/Containerfile \
  -t harbor-homebrew-compat:brew6 \
  tests/homebrew
```

## Run The Full Showcase Against Local Harbor

Assuming Harbor is reachable on the host at `localhost:8080`:

```bash
tests/homebrew/run-showcase.sh all
```

The script runs inside the container and does four things:

1. Creates the Harbor project `homebrew` if needed.
2. Copies `ghcr.io/homebrew/core/harbor-cli:0.0.23` to
   `host.docker.internal:8080/homebrew/core/harbor-cli:0.0.23`.
3. Verifies the OCI image index in Harbor.
4. Runs `brew install --force-bottle` with `HOMEBREW_ARTIFACT_DOMAIN` pointed at Harbor.

## Run Individual Steps

```bash
tests/homebrew/run-showcase.sh push
tests/homebrew/run-showcase.sh inspect
tests/homebrew/run-showcase.sh fetch
tests/homebrew/run-showcase.sh install
```

## Test A Homebrew Proxy Project

Create a public proxy-cache project backed by the built-in `homebrew` provider
(`https://formulae.brew.sh/api`). The example below uses project `brew`:

```bash
HARBOR_URL=http://host.docker.internal:4700 \
HARBOR_PROJECT=brew \
tests/homebrew/run-showcase.sh proxy
```

The proxy suite sets both native Homebrew mirror variables and disables direct
artifact fallback:

```bash
HOMEBREW_API_DOMAIN=http://host.docker.internal:4700/homebrew/brew/api
HOMEBREW_ARTIFACT_DOMAIN=http://host.docker.internal:4700/homebrew/brew
HOMEBREW_ARTIFACT_DOMAIN_NO_FALLBACK=1
```

It exercises formula and cask metadata, `brew search`, `brew info`, dependency
resolution, bottle fetch and checksum verification, normal install, linkage,
pin/unpin, reinstall, upgrade, Brewfile install/check, uninstall, autoremove,
cleanup, and missing-formula rejection. Formula bottles and dependency bottles
are fetched from GHCR through Harbor's `/homebrew/<project>/v2/...` route.

Open a shell:

```bash
tests/homebrew/run-showcase.sh shell
```

Inside the container:

```bash
run-homebrew-compat push
run-homebrew-compat fetch
run-homebrew-compat install
```

## Push The Client Image Through Harbor

```bash
docker login localhost:8080 -u admin -p Harbor12345

docker tag harbor-homebrew-compat:brew6 \
  localhost:8080/library/harbor-homebrew-compat:brew6

docker push localhost:8080/library/harbor-homebrew-compat:brew6
docker pull localhost:8080/library/harbor-homebrew-compat:brew6
```

Then run from the image in Harbor:

```bash
docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e HARBOR_URL=http://host.docker.internal:8080 \
  -e HARBOR_REGISTRY=host.docker.internal:8080 \
  -e HARBOR_PROJECT=homebrew \
  -e HARBOR_USERNAME=admin \
  -e HARBOR_PASSWORD=Harbor12345 \
  localhost:8080/library/harbor-homebrew-compat:brew6 \
  all
```

## Useful Environment Variables

- `HARBOR_URL`: Harbor HTTP URL used by Homebrew, default `http://host.docker.internal:8080`
- `HARBOR_REGISTRY`: OCI registry host used by `skopeo`, default `host.docker.internal:8080`
- `HARBOR_PROJECT`: target project, default `homebrew`
- `HARBOR_USERNAME`: Harbor username, default `admin`
- `HARBOR_PASSWORD`: Harbor password, default `Harbor12345`
- `HOMEBREW_FORMULA`: formula name, default `harbor-cli`
- `HOMEBREW_FORMULA_VERSION`: formula version, default `0.0.23`
- `HOMEBREW_TAP`: local tap name created inside the container, default `harbor/fixtures`
- `HOMEBREW_OCI_REPOSITORY`: target OCI repository inside the Harbor project, default `core/harbor-cli`
- `HOMEBREW_SOURCE_IMAGE`: source OCI image, default `ghcr.io/homebrew/core/harbor-cli:0.0.23`
- `HOMEBREW_TARGET_IMAGE`: full target OCI image override

## What This Exercises

Homebrew publishes bottles as normal OCI image indexes:

- Index tag: `0.0.23`
- Repository: `homebrew/core/harbor-cli`
- Child manifests: one per bottle platform, for example `0.0.23.x86_64_linux`
- Bottle payload: `application/vnd.oci.image.layer.v1.tar+gzip`
- Metadata: OCI annotations such as `sh.brew.bottle.digest`, `sh.brew.tab`, and `sh.brew.path_exec_files`

`brew` does not follow Harbor's Bearer challenge automatically for bottle blob
downloads, so the fixture sets:

```bash
HOMEBREW_DOCKER_REGISTRY_BASIC_AUTH_TOKEN="$(printf 'admin:Harbor12345' | base64 -w0)"
```

That makes Homebrew send a Basic auth header while it downloads the manifest and
bottle blob from Harbor.
