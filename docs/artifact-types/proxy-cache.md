# Proxy-cache ecosystems (PyPI, Cargo, Go, Homebrew)

These four ecosystems are pull-through caches only in `multi-format-artifacts` —
Harbor doesn't host your own packages for them (unlike npm/Maven, see
[npm.md](./npm.md) / [maven.md](./maven.md)). The setup pattern is the same
for all four; only the client configuration differs.

## Setup (same for every ecosystem below)

1. **Administration → Registries → New Endpoint** — pick a well-known
   provider for an editable default, or its generic registry counterpart for
   a custom upstream. Defaults: PyPI → `https://pypi.org/simple`, crates.io →
   `https://index.crates.io`, Go Proxy → `https://proxy.golang.org`, and Homebrew →
   `https://formulae.brew.sh/api`. The Homebrew preset also retrieves bottles
   from `https://ghcr.io/v2/homebrew/core`.
2. **New Project** (or edit an existing one) → enable **Proxy Cache** →
   select the registry endpoint you just created.
3. Point the ecosystem's client at the project using the config below.

Every package your builds pull is cached the first time and served locally
after that — no re-fetch from upstream on a cache hit, so builds keep working
through an upstream outage or rate limit.

## PyPI

```ini
# pip.conf / pip.ini
[global]
index-url = https://<harbor-host>/pypi/<project>/simple/
```

or per-invocation: `pip install --index-url https://<harbor-host>/pypi/<project>/simple/ <package>`

## Cargo

```toml
# .cargo/config.toml
[source.crates-io]
replace-with = "harbor"

[source.harbor]
registry = "sparse+https://<harbor-host>/cargo/<project>/"
```

## Go modules

```bash
go env -w GOPROXY=https://<harbor-host>/go/<project>/
```

Checksum verification (Go's sumdb) can be proxied the same way if your
registry endpoint provider is configured for it:

```bash
go env -w GONOSUMCHECK=0
go env -w GOSUMDB=sum.golang.org
go env -w GOPROXY=https://<harbor-host>/go/<project>/
```

(`/go-sumdb/*` is a separate route from `/go/*` — see the source pointers
below if you're wiring `GOSUMDB` through Harbor rather than talking to
`sum.golang.org` directly.)

## Homebrew

```bash
export HOMEBREW_API_DOMAIN="https://<harbor-host>/homebrew/<project>/api"
export HOMEBREW_ARTIFACT_DOMAIN="https://<harbor-host>/homebrew/<project>"
export HOMEBREW_ARTIFACT_DOMAIN_NO_FALLBACK=1
```

Homebrew keeps formula/cask metadata and bottle content on separate origins.
Harbor routes the `/api/...` requests to the configured metadata endpoint and
the rewritten `/v2/homebrew/core/...` manifest/blob requests to GHCR. A
generic **Homebrew Registry** endpoint instead treats the configured URL as a
unified mirror that serves both path families.

## Authentication

All four use standard Harbor Basic auth (a Harbor user or robot account) if
the project isn't public — same credential model as native npm/Maven and the
existing Docker Hub proxy cache.

## Troubleshooting

| Symptom | Cause |
|---|---|
| First install/build of a package is slow, later ones are fast | Expected — first pull is a cache miss (round-trips to upstream), later ones are served from Harbor. |
| `401`/`403` from the client | Confirm the project has pull permission for your credentials, or is public. |
| Package never updates after upstream publishes a new version | Only the specific version your build actually requests gets cached — pin/bump your dependency as usual; Harbor doesn't need re-configuration for new upstream releases. |

## Source pointers

`src/server/registry/{pypi,cargo,gomod,gosum,homebrew}` (protocol adapters),
`src/server/registry/pkgproxy` (shared upstream-fetch + cache-fill logic),
`src/server/registry/pkgstore` (shared OCI-backed cache storage).
