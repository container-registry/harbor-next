# Artifact Types Guide

`multi-format-artifacts` extends Harbor beyond OCI container images to host and proxy
the package formats developers already use day to day. This guide is for
**end users** — how to push, pull, and configure each format — not for
contributors extending the implementation (see the source pointers at the
bottom of each page for that).

Two independent capabilities are covered, and most ecosystems below support
one or both:

| Capability | What it means | Who sets it up |
|---|---|---|
| **Native hosting** | You `publish`/`push` packages directly into Harbor; Harbor stores and serves them itself. | Project users, per push |
| **Proxy cache** | Harbor sits in front of the ecosystem's public registry (npmjs.org, PyPI, Maven Central, ...) and transparently caches whatever your builds pull. | Project admins, once per project |

## Ecosystems

| Ecosystem | Native hosting | Proxy cache | Guide |
|---|:---:|:---:|---|
| npm | ✅ | ✅ | [npm.md](./npm.md) |
| Maven | ✅ | ✅ | [maven.md](./maven.md) |
| PyPI | — | ✅ | [proxy-cache.md](./proxy-cache.md) |
| Cargo (Rust) | — | ✅ | [proxy-cache.md](./proxy-cache.md) |
| Go modules | — | ✅ | [proxy-cache.md](./proxy-cache.md) |
| Go checksum DB (sumdb) | — | ✅ | [proxy-cache.md](./proxy-cache.md) |
| Homebrew | — | ✅ | [proxy-cache.md](./proxy-cache.md) |
| bootc (bootable OS containers) | n/a — this is a **scanner**, not a storage format | n/a | [bootc.md](./bootc.md) |

Container images, Helm charts, CNAB, WASM, and the other existing OCI
artifact types are unchanged — see the upstream Harbor docs for those.

## Which one do I want?

- **"I want to publish our team's internal npm/Maven packages to Harbor instead of a public registry."**
  → Native hosting. Point your package manager's config at Harbor and
  `npm publish` / `mvn deploy` as normal. See [npm.md](./npm.md) /
  [maven.md](./maven.md).
- **"I want our CI to pull from npmjs.org / PyPI / crates.io / Maven Central
  / proxy.golang.org / Homebrew through Harbor, so builds survive an upstream
  outage and don't get rate-limited."**
  → Proxy cache. One-time project setup, then point your package manager at
  the project instead of the public registry. See
  [proxy-cache.md](./proxy-cache.md) (npm and Maven proxy-cache setup is
  covered on their own pages since they also support native hosting in the
  same project).
- **"I build bootc (bootable container) images and want CVE scanning that
  actually sees the RPMs inside, not just the container layers."**
  → [bootc.md](./bootc.md) — this is Harbor's vulnerability scanning story,
  not a storage format; regular container image push/pull is unchanged.

## How native + proxy interact (npm and Maven)

A single npm or Maven project in Harbor can do both at once. On every
`GET` (packument/metadata or a specific file), Harbor checks **native storage
first**; only on a miss does it fall through to the upstream proxy (if the
project is configured as a proxy cache) and best-effort cache what it
fetched. So:

- A package you've published natively always wins over an upstream package
  of the same name/version — publishing effectively "shadows" upstream.
- A proxy-cache project with nothing published natively behaves exactly like
  a normal pull-through cache.
- `PUT`/publish always writes to native storage; proxy-cache projects are
  pull-only for the ecosystem's public registry, same as Harbor's existing
  Docker Hub proxy cache.

## Source pointers (for contributors, not needed to use this)

- Native npm/Maven backend: `src/pkg/multiformat`, `src/controller/multiformat`,
  `src/server/registry/{npm,maven}`,
  `src/controller/artifact/processor/{npm,maven}`.
- Proxy-cache adapters (PyPI/Cargo/Go/Homebrew, and the npm/Maven fallback):
  `src/server/registry/{pypi,cargo,gomod,gosum,homebrew}`,
  `src/server/registry/pkgproxy`, `src/server/registry/pkgstore`.
- Design record for the native npm/Maven UX:
  `docs/native-package-ux/README.md`.
- bootc scanning: `tools/grype-scanner`, `tools/snyk-scanner`.
