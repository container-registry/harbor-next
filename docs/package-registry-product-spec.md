# Harbor Package Registry Product Spec

## npm product coverage

### Enable and configure

- [ ] I disable Multi-Format Artifacts. I should not see npm providers, npm project settings, or npm client instructions.
- [ ] I enable Multi-Format Artifacts. I should see `npmjs` and `npm Registry` providers immediately.
- [ ] I select `npmjs`. I should get `https://registry.npmjs.org` prefilled and editable.
- [ ] I select `npm Registry`. I should enter any npm-compatible registry URL, credentials, TLS policy, CA, and optional headers.
- [ ] I test an npm upstream. I should see DNS, TLS, auth, npm protocol, and latency results separately.
- [ ] I save an npm upstream. I should see name, provider, URL, priority, mode, health, and last check.
- [ ] I create several npm upstreams. I should reorder them and get deterministic first-match resolution.
- [ ] I disable one npm upstream. I should keep cached packages and stop new requests to that upstream.
- [ ] I delete an npm upstream with cached packages. I should choose whether to retain or purge its cache.
- [ ] I disable Multi-Format Artifacts while npm upstreams or packages exist. I should get a blocking warning listing those resources.

### Discover setup

- [ ] I open npm setup instructions for project `<PROJECT>`. I should see `<HARBOR>/npm/<PROJECT>/` as registry URL.
- [ ] I open npm setup instructions for a public project. I should see commands without mandatory credentials.
- [ ] I open npm setup instructions for a private project. I should see user and robot-account auth choices.
- [ ] I copy npm setup instructions. I should get commands containing external Harbor URL, never Core or Portal container URLs.
- [ ] I copy npm setup instructions. I should not receive stored passwords, tokens, or upstream credentials.

### Configure and authenticate client

- [ ] I run `npm config set registry <HARBOR>/npm/<PROJECT>/`. I should route unscoped npm requests through Harbor.
- [ ] I run `npm config set @<SCOPE>:registry <HARBOR>/npm/<PROJECT>/`. I should route that scope through Harbor.
- [ ] I run `npm login --auth-type=legacy --registry <HARBOR>/npm/<PROJECT>/`. I should store working Harbor credentials in npm config.
- [ ] I run `npm ping --registry <HARBOR>/npm/<PROJECT>/`. I should receive a successful npm registry response.
- [ ] I run `npm whoami --registry <HARBOR>/npm/<PROJECT>/`. I should receive my Harbor username.
- [ ] I run `npm whoami` with invalid credentials. I should receive an npm-compatible auth error.
- [ ] I authenticate using a pull-only robot account. I should install packages and should not publish packages.
- [ ] I disable that robot account. I should lose npm access immediately.

### Publish hosted packages

- [ ] I run `npm pack`. I should create a valid package tarball for publication.
- [ ] I run `npm publish --registry <HARBOR>/npm/<PROJECT>/`. I should publish package name, version, tarball, integrity, and metadata.
- [ ] I run `npm publish --tag beta --registry <HARBOR>/npm/<PROJECT>/`. I should publish the version and point `beta` to it.
- [ ] I publish `@<SCOPE>/<PACKAGE>`. I should preserve its scope and package identity.
- [ ] I publish a package containing dependencies, peer dependencies, optional dependencies, engines, license, and repository metadata. I should retrieve those fields unchanged with `npm view`.
- [ ] I publish the same name and version with different content. I should receive an immutable-version conflict.
- [ ] I publish a lower version after a higher version. I should not move `latest` backward unless I explicitly change its dist-tag.
- [ ] I publish a prerelease without `--tag`. I should not make it the stable default when a stable release exists.
- [ ] I run `npm publish --dry-run`. I should validate locally without creating a Harbor package.
- [ ] I run `npm publish --provenance` with valid provenance support. I should preserve and expose its linked attestation.

### Read metadata and packages

- [ ] I run `npm view <PACKAGE> --registry <HARBOR>/npm/<PROJECT>/`. I should receive complete visible package metadata.
- [ ] I run `npm view <PACKAGE> versions --json --registry <HARBOR>/npm/<PROJECT>/`. I should receive every visible hosted and upstream version.
- [ ] I run `npm view <PACKAGE>@<VERSION> dist.integrity --registry <HARBOR>/npm/<PROJECT>/`. I should receive integrity matching downloaded tarball bytes.
- [ ] I run `npm install <PACKAGE> --registry <HARBOR>/npm/<PROJECT>/`. I should install version selected by effective `latest` dist-tag.
- [ ] I run `npm install <PACKAGE>@<VERSION> --registry <HARBOR>/npm/<PROJECT>/`. I should install that exact version.
- [ ] I run `npm install '<PACKAGE>@<RANGE>' --registry <HARBOR>/npm/<PROJECT>/`. I should install highest matching visible version.
- [ ] I run `npm install <PACKAGE>@<TAG> --registry <HARBOR>/npm/<PROJECT>/`. I should install version referenced by that dist-tag.
- [ ] I run `npm install @<SCOPE>/<PACKAGE> --registry <HARBOR>/npm/<PROJECT>/`. I should install scoped package without losing scope.
- [ ] I run `npm ci --registry <HARBOR>/npm/<PROJECT>/`. I should install exact lockfile versions and verify integrity.
- [ ] I run `npm pack <PACKAGE>@<VERSION> --registry <HARBOR>/npm/<PROJECT>/`. I should download exact registry tarball.
- [ ] I run `npm search <QUERY> --registry <HARBOR>/npm/<PROJECT>/`. I should receive visible matching packages or a clear documented unsupported-operation error.

### Manage dist-tags and package state

- [ ] I run `npm dist-tag ls <PACKAGE> --registry <HARBOR>/npm/<PROJECT>/`. I should see every visible dist-tag.
- [ ] I run `npm dist-tag add <PACKAGE>@<VERSION> <TAG> --registry <HARBOR>/npm/<PROJECT>/`. I should point that tag to that hosted version.
- [ ] I run `npm dist-tag rm <PACKAGE> <TAG> --registry <HARBOR>/npm/<PROJECT>/`. I should remove only that tag.
- [ ] I remove `latest`. I should not silently recreate it until defined product fallback rules run.
- [ ] I delete the version referenced by `latest`. I should move `latest` to next eligible stable version or remove it according to project policy.
- [ ] I run `npm deprecate <PACKAGE>@<RANGE> '<MESSAGE>' --registry <HARBOR>/npm/<PROJECT>/`. I should expose that warning during later installs.
- [ ] I run `npm unpublish <PACKAGE>@<VERSION> --registry <HARBOR>/npm/<PROJECT>/`. I should delete that hosted version when Harbor deletion policy permits it.
- [ ] I run `npm unpublish` against cached upstream content. I should receive a clear error directing me to cache purge controls.
- [ ] I use an npm native team or access-control command Harbor does not implement. I should receive a clear unsupported-operation response and should manage access through Harbor RBAC.

### Proxy and cache npmjs

- [ ] I attach npmjs to project `<PROJECT>`. I should install public npm packages from the same Harbor project URL.
- [ ] I request an uncached package packument. I should receive all upstream versions and dist-tags.
- [ ] I install one version from a packument containing many versions. I should cache only requested assets and keep every upstream version visible.
- [ ] I install a second uncached version. I should fetch its tarball lazily without losing first version.
- [ ] I publish a local version absent upstream. I should see local and upstream versions in one packument.
- [ ] I publish a local version matching an upstream version. I should receive local metadata and local tarball for collision.
- [ ] I define a local dist-tag matching an upstream dist-tag. I should receive local tag when local-precedence policy is enabled.
- [ ] I enable upstream-tag precedence. I should use upstream `latest` only when its referenced version is visible.
- [ ] I disable upstream-tag precedence. I should calculate `latest` from local product rules.
- [ ] I install the same upstream tarball twice. I should get a cache hit on second install.
- [ ] I stop npmjs after caching a version. I should install cached version successfully.
- [ ] I stop npmjs before caching another version. I should get a clear upstream-unavailable error.
- [ ] I receive upstream 404 for one package. I should try next npm upstream, then cache final not-found result for configured TTL.
- [ ] I invalidate npm metadata cache. I should refresh packument next request without deleting tarballs.
- [ ] I invalidate npm negative cache. I should retry previously missing package next request.
- [ ] I run `npm audit --registry <HARBOR>/npm/<PROJECT>/`. I should receive proxied audit results or a clear documented unsupported-operation error.

### npm policy and product UI

- [ ] I browse npm packages in Harbor. I should see package name, scope, versions, dist-tags, source, cache state, size, pulls, and update time.
- [ ] I open one npm version. I should see dependencies, integrity, tarball, provenance, scan status, origin, and timestamps.
- [ ] I quarantine one npm version. I should receive HTTP 403 when installing that version.
- [ ] I quarantine one npm version. I should keep allowed versions installable when range resolution can avoid quarantined version.
- [ ] I enable block-until-scan. I should not install new hosted or cached npm versions before required checks finish.
- [ ] I purge npm cache for one upstream. I should preview affected packages and bytes and should not delete hosted versions.
- [ ] I view npm upstream health. I should see status, last success, last failure, latency, requests, hits, misses, and bytes fetched.

## PyPI product coverage

### Enable and configure

- [ ] I disable Multi-Format Artifacts. I should not see PyPI providers, PyPI project settings, or Python client instructions.
- [ ] I enable Multi-Format Artifacts. I should see `PyPI` and `PyPI Registry` providers immediately.
- [ ] I select `PyPI`. I should get `https://pypi.org` prefilled and editable.
- [ ] I select `PyPI Registry`. I should enter any PEP 503 or PEP 691 compatible index URL, credentials, TLS policy, CA, and optional headers.
- [ ] I test a PyPI upstream. I should see DNS, TLS, auth, Simple API negotiation, and latency results separately.
- [ ] I save a PyPI upstream. I should see name, provider, URL, priority, mode, index state, health, and last check.
- [ ] I create several PyPI upstreams. I should reorder them and get deterministic Harbor resolution.
- [ ] I disable one PyPI upstream. I should keep cached distributions and stop new requests to that upstream.
- [ ] I delete a PyPI upstream with cached distributions. I should choose whether to retain or purge its cache.
- [ ] I disable Multi-Format Artifacts while PyPI upstreams or packages exist. I should get a blocking warning listing those resources.

### Discover setup

- [ ] I open Python setup instructions for project `<PROJECT>`. I should see `<HARBOR>/pypi/<PROJECT>/simple/` as index URL.
- [ ] I open Python publish instructions for project `<PROJECT>`. I should see correct Harbor upload URL separately from Simple API URL.
- [ ] I open setup instructions for a public project. I should see pip, uv, Poetry, and Twine commands without mandatory credentials.
- [ ] I open setup instructions for a private project. I should see user and robot-account auth choices.
- [ ] I copy Python setup instructions. I should not receive stored passwords, tokens, or upstream credentials.

### Configure and authenticate clients

- [ ] I run `python -m pip config set global.index-url <HARBOR>/pypi/<PROJECT>/simple/`. I should route pip package resolution through Harbor.
- [ ] I run `python -m pip config list -v`. I should see Harbor as active primary index.
- [ ] I configure Harbor with `--index-url`. I should not need `--extra-index-url` for packages supplied by configured upstreams.
- [ ] I use `--extra-index-url` with a private namespace collision. I should see a dependency-confusion warning in Harbor setup guidance.
- [ ] I configure `[[tool.uv.index]]` with Harbor URL. I should route uv resolution through Harbor.
- [ ] I run `poetry source add --priority=primary harbor <HARBOR>/pypi/<PROJECT>/simple/`. I should route Poetry resolution through Harbor.
- [ ] I configure `.pypirc` with Harbor upload URL. I should publish through Twine using repository alias `harbor`.
- [ ] I authenticate using a pull-only robot account. I should install distributions and should not upload them.
- [ ] I disable that robot account. I should lose Python package access immediately.

### Build and publish hosted distributions

- [ ] I run `python -m build`. I should produce valid wheel and source distribution files.
- [ ] I run `python -m twine check dist/*`. I should validate distribution metadata before upload.
- [ ] I run `python -m twine upload --repository harbor dist/*`. I should publish wheel and sdist into one package version.
- [ ] I run `python -m twine upload --repository-url <UPLOAD_URL> dist/*`. I should publish through explicit Harbor URL.
- [ ] I run `uv publish --publish-url <UPLOAD_URL> dist/*`. I should publish valid Python distributions through Harbor.
- [ ] I run `poetry publish --repository harbor`. I should publish Poetry-built distributions through Harbor.
- [ ] I upload wheel and sdist for same name and version. I should see both files under one version.
- [ ] I upload same filename with different bytes. I should receive an immutable-file conflict.
- [ ] I upload invalid wheel, filename, or core metadata. I should receive a Python-native validation error.
- [ ] I upload a project using `.`, `_`, or `-` variants. I should store and resolve one normalized project identity.
- [ ] I upload `Requires-Python`, extras, dependencies, license, classifiers, and project URLs. I should retrieve those fields unchanged.

### Install and resolve hosted distributions

- [ ] I run `python -m pip install --dry-run <PACKAGE> --index-url <INDEX_URL> -v`. I should validate client and index configuration without installation.
- [ ] I run `python -m pip index versions <PACKAGE> --index-url <INDEX_URL>`. I should see every visible version.
- [ ] I run `python -m pip install <PACKAGE>==<VERSION> --index-url <INDEX_URL>`. I should install exact version.
- [ ] I run `python -m pip install '<PACKAGE>>=<MIN>,<<MAX>' --index-url <INDEX_URL>`. I should install highest compatible visible version.
- [ ] I run `python -m pip install '<PACKAGE>[<EXTRA>]' --index-url <INDEX_URL>`. I should resolve declared extra dependencies.
- [ ] I run `python -m pip install --pre <PACKAGE> --index-url <INDEX_URL>`. I should include eligible prereleases.
- [ ] I run `python -m pip install --only-binary=:all: <PACKAGE> --index-url <INDEX_URL>`. I should select compatible wheel or fail clearly.
- [ ] I run `python -m pip install --no-binary=:all: <PACKAGE> --index-url <INDEX_URL>`. I should select sdist or fail clearly.
- [ ] I run `python -m pip download <PACKAGE>==<VERSION> --index-url <INDEX_URL>`. I should download exact advertised distribution.
- [ ] I run `python -m pip install --require-hashes -r requirements.txt --index-url <INDEX_URL>`. I should install only files matching locked hashes.
- [ ] I run `uv pip install <PACKAGE> --index-url <INDEX_URL>`. I should install through Harbor.
- [ ] I run `uv lock`. I should lock Harbor-resolved versions and sources reproducibly.
- [ ] I run `uv sync --index-url <INDEX_URL>`. I should reproduce locked environment through Harbor.
- [ ] I run `poetry add --source harbor <PACKAGE>`. I should resolve and lock package through Harbor.
- [ ] I run `poetry install`. I should install locked Harbor dependencies.

### Simple API and metadata

- [ ] I request `<HARBOR>/pypi/<PROJECT>/simple/`. I should receive project listing, never Harbor Portal HTML.
- [ ] I request `/simple/<NORMALIZED_PACKAGE>/`. I should receive every visible distribution file for that project.
- [ ] I request a mixed-case or punctuation variant. I should redirect or resolve to normalized project name.
- [ ] I send `Accept: application/vnd.pypi.simple.v1+html`. I should receive valid Simple API HTML content type.
- [ ] I send `Accept: application/vnd.pypi.simple.v1+json`. I should receive valid PEP 691 JSON content type.
- [ ] I request PEP 691 JSON. I should receive API version, project name, files, hashes, URLs, sizes, upload times, and supported metadata.
- [ ] I request PEP 658 metadata. I should fetch core metadata without downloading full distribution when available.
- [ ] I request PEP 714 metadata fields. I should receive current `core-metadata` names instead of obsolete aliases.
- [ ] I request a distribution URL containing hash fragment. I should receive bytes matching advertised hash.
- [ ] I request a project containing no downloadable files. I should receive valid empty index response, not 404 or Portal HTML.

### Yank, delete, and quarantine

- [ ] I yank one Python release through Harbor UI or API. I should retain its files and expose yanked metadata.
- [ ] I install without pinning after a release is yanked. I should avoid yanked release when an eligible non-yanked release exists.
- [ ] I install an exact yanked version. I should follow pip-compatible yanked behavior and show yank reason.
- [ ] I unyank one release through Harbor UI or API. I should restore normal resolver visibility.
- [ ] I delete one hosted distribution file. I should retain other files in same version.
- [ ] I delete one hosted version. I should remove all its distribution files after confirmation.
- [ ] I attempt to delete cached upstream content as hosted content. I should receive a clear error directing me to cache purge.
- [ ] I quarantine one distribution version. I should receive HTTP 403 when pip, uv, or Poetry downloads it.
- [ ] I release one quarantined version. I should restore downloads without re-upload.

### Proxy and cache PyPI

- [ ] I attach PyPI upstream to project `<PROJECT>`. I should install public Python packages from same Harbor index URL.
- [ ] I request uncached project metadata. I should receive every visible upstream file and version.
- [ ] I request Simple API HTML then JSON. I should negotiate each representation without corrupting shared metadata cache.
- [ ] I download one wheel from project containing many files. I should cache requested wheel and keep all upstream files visible.
- [ ] I download an sdist later. I should fetch it lazily without losing cached wheel.
- [ ] I publish a local version absent upstream. I should see local and upstream versions in one project index.
- [ ] I publish a local file colliding with upstream filename. I should receive local file according to local-precedence policy.
- [ ] I install same upstream distribution twice. I should get a cache hit on second install.
- [ ] I stop PyPI after caching a distribution. I should install cached distribution successfully.
- [ ] I stop PyPI before caching another distribution. I should get clear upstream-unavailable error.
- [ ] I receive upstream 404 for one normalized project. I should try next PyPI upstream, then cache final not-found result for configured TTL.
- [ ] I resync PyPI upstream index. I should refresh package metadata without deleting cached files.
- [ ] I invalidate PyPI metadata cache. I should refresh project pages next request without deleting distributions.
- [ ] I purge PyPI cache for one upstream. I should preview affected projects, versions, files, and bytes without deleting hosted distributions.

### PyPI policy and product UI

- [ ] I browse Python packages in Harbor. I should see normalized name, display name, versions, file count, source, cache state, size, pulls, and update time.
- [ ] I open one Python version. I should see wheels, sdists, hashes, Python requirement, yank state, scan status, origin, and timestamps.
- [ ] I enable block-until-scan. I should not install new hosted or cached Python files before required checks finish.
- [ ] I view PyPI upstream indexing. I should see progress, last successful sync, next sync, package count, and failure reason.
- [ ] I view PyPI upstream health. I should see status, latency, requests, hits, misses, and bytes fetched.

## Rust and Cargo product coverage

### Enable and configure

- [ ] I disable Multi-Format Artifacts. I should not see Cargo providers, Cargo project settings, or Rust client instructions.
- [ ] I enable Multi-Format Artifacts. I should see `crates.io` and `Cargo Registry` providers immediately.
- [ ] I select `crates.io`. I should get `https://index.crates.io/` prefilled and editable.
- [ ] I select `Cargo Registry`. I should enter any Cargo sparse registry URL, credentials, TLS policy, CA, and optional headers.
- [ ] I test a Cargo upstream. I should see DNS, TLS, auth, sparse `config.json`, index, download endpoint, and latency results separately.
- [ ] I save a Cargo upstream. I should see name, provider, URL, priority, mode, index state, health, and last check.
- [ ] I create several Cargo upstreams. I should reorder them and get deterministic Harbor resolution.
- [ ] I disable one Cargo upstream. I should keep cached crates and stop new requests to that upstream.
- [ ] I delete a Cargo upstream with cached crates. I should choose whether to retain or purge its cache.
- [ ] I disable Multi-Format Artifacts while Cargo upstreams or crates exist. I should get a blocking warning listing those resources.

### Discover setup

- [ ] I open Cargo setup instructions for project `<PROJECT>`. I should see `sparse+<HARBOR>/cargo/<PROJECT>/` as registry index.
- [ ] I open Cargo setup instructions. I should see `.cargo/config.toml`, credentials, environment-variable, source-replacement, publish, and install examples.
- [ ] I open setup instructions for a public project. I should see commands without mandatory credentials.
- [ ] I open setup instructions for a private project. I should see user and robot-account token choices.
- [ ] I copy Cargo setup instructions. I should not receive stored passwords, tokens, or upstream credentials.

### Configure and authenticate Cargo

- [ ] I add `[registries.harbor] index = "sparse+<HARBOR>/cargo/<PROJECT>/"` to `.cargo/config.toml`. I should address Harbor as registry `harbor`.
- [ ] I configure `[source.crates-io] replace-with = "harbor"`. I should resolve crates.io dependencies only through Harbor.
- [ ] I run `cargo login --registry harbor`. I should store a working Harbor registry token.
- [ ] I set `CARGO_REGISTRIES_HARBOR_TOKEN=<TOKEN>`. I should authenticate without writing token to project files.
- [ ] I request Cargo `config.json` anonymously from a private project. I should receive Cargo-compatible auth challenge.
- [ ] I authenticate to a private project. I should receive `auth-required: true` behavior consistently for index and downloads.
- [ ] I authenticate using a pull-only robot account. I should download crates and should not publish, yank, or modify ownership.
- [ ] I disable that robot account. I should lose Cargo access immediately.

### Package and publish hosted crates

- [ ] I run `cargo package`. I should produce a valid `.crate` archive without publishing it.
- [ ] I run `cargo publish --dry-run --registry harbor`. I should validate crate packaging without creating Harbor content.
- [ ] I run `cargo publish --registry harbor`. I should publish crate archive, index entry, checksum, features, dependencies, and metadata.
- [ ] I publish a crate with normal, build, dev, optional, renamed, target-specific, and registry dependencies. I should retrieve correct dependency metadata.
- [ ] I publish a crate with features, default features, license, repository, documentation, keywords, categories, and Rust version. I should retrieve those fields unchanged.
- [ ] I publish a prerelease version. I should preserve Cargo semantic-version behavior.
- [ ] I publish same crate name and version with different bytes. I should receive immutable-version conflict.
- [ ] I publish a crate depending on unavailable private dependency. I should receive a clear dependency-validation result according to project policy.
- [ ] I publish using unsupported Git index protocol. I should receive clear guidance to use `sparse+` URL.

### Discover and consume crates

- [ ] I run `cargo search <QUERY> --registry harbor`. I should receive matching visible crates or a clear documented unsupported-operation error.
- [ ] I run `cargo info <CRATE> --registry harbor`. I should receive visible crate metadata and versions.
- [ ] I run `cargo add <CRATE> --registry harbor`. I should add compatible dependency from Harbor.
- [ ] I run `cargo add <CRATE>@<VERSION> --registry harbor`. I should add exact requested requirement.
- [ ] I run `cargo fetch`. I should download lockfile dependencies through Harbor.
- [ ] I run `cargo build`. I should compile dependencies downloaded through Harbor.
- [ ] I run `cargo test`. I should compile and run tests using Harbor-resolved dependencies.
- [ ] I run `cargo update -p <CRATE>`. I should select highest compatible non-yanked visible version.
- [ ] I run `cargo update -p <CRATE> --precise <VERSION>`. I should select requested eligible version.
- [ ] I run `cargo install <CRATE> --registry harbor`. I should download, compile, and install binary crate.
- [ ] I run `cargo vendor`. I should vendor exact Harbor-resolved crate sources and checksums.
- [ ] I run `cargo metadata`. I should report Harbor-resolved dependency graph without downloading from bypassed registries.
- [ ] I rebuild from `Cargo.lock` with upstream unavailable. I should succeed when every locked crate is cached in Harbor.

### Sparse index behavior

- [ ] I request `<HARBOR>/cargo/<PROJECT>/config.json`. I should receive correct Harbor API and crate download URLs.
- [ ] I request one sparse index path. I should receive every visible version for that crate.
- [ ] I request sparse metadata with `If-None-Match`. I should receive `304 Not Modified` when unchanged.
- [ ] I request sparse metadata after publish, yank, or unyank. I should receive changed ETag and current entries.
- [ ] I request a crate archive. I should receive bytes matching index SHA-256 checksum.
- [ ] I request a mixed-case crate name. I should follow Cargo crate-name and index-path rules.
- [ ] I request a nonexistent crate. I should receive Cargo-compatible 404, never Harbor Portal HTML.
- [ ] I request a malformed sparse path. I should receive Cargo-compatible client error.

### Yank, unyank, ownership, and delete

- [ ] I run `cargo yank --registry harbor <CRATE>@<VERSION>`. I should mark that hosted version yanked without deleting archive.
- [ ] I run `cargo yank --undo --registry harbor <CRATE>@<VERSION>`. I should restore normal resolver visibility.
- [ ] I add a new dependency after a version is yanked. I should not resolve newly to that yanked version.
- [ ] I build an existing lockfile containing a yanked version. I should download it when Cargo rules permit locked yanked versions.
- [ ] I run supported Cargo owner commands. I should map ownership to Harbor RBAC or return clear guidance to manage access in Harbor.
- [ ] I delete one hosted crate version through Harbor UI or API. I should require confirmation and update sparse index.
- [ ] I attempt to delete cached crates.io content as hosted content. I should receive a clear error directing me to cache purge.
- [ ] I quarantine one crate version. I should receive HTTP 403 when Cargo downloads it.
- [ ] I release one quarantined crate version. I should restore downloads without republishing it.

### Proxy and cache crates.io

- [ ] I attach crates.io upstream to project `<PROJECT>`. I should resolve public crates from same Harbor sparse registry URL.
- [ ] I complete initial crates.io index sync. I should see progress, package count, last success, and next sync.
- [ ] I request uncached crate metadata. I should receive every visible crates.io version.
- [ ] I download one version from crate containing many versions. I should cache requested archive and keep every upstream version visible.
- [ ] I download another version later. I should fetch it lazily without losing first version.
- [ ] I publish a local crate version absent upstream. I should see local and upstream versions in one sparse entry.
- [ ] I publish a local crate version matching upstream version. I should receive local index entry and archive according to local-precedence policy.
- [ ] I download same upstream crate twice. I should get cache hit on second download.
- [ ] I stop crates.io after caching one crate. I should build successfully using cached crate.
- [ ] I stop crates.io before caching another crate. I should get clear upstream-unavailable error.
- [ ] I receive upstream 404 for one crate. I should try next Cargo upstream, then cache final not-found result for configured TTL.
- [ ] I resync crates.io index. I should refresh metadata without deleting cached `.crate` archives.
- [ ] I invalidate Cargo metadata cache. I should refresh sparse entry next request without deleting archives.
- [ ] I purge Cargo cache for one upstream. I should preview affected crates, versions, and bytes without deleting hosted crates.

### Cargo policy and product UI

- [ ] I browse Cargo packages in Harbor. I should see crate name, versions, yank state, source, cache state, size, pulls, and update time.
- [ ] I open one crate version. I should see checksum, dependencies, features, Rust version, license, scan status, origin, and timestamps.
- [ ] I enable block-until-scan. I should not download new hosted or cached crates before required checks finish.
- [ ] I view Cargo upstream indexing. I should see progress, last successful sync, next sync, crate count, and failure reason.
- [ ] I view Cargo upstream health. I should see status, latency, requests, hits, misses, and bytes fetched.

## Cross-format execution

- [ ] I run every npm checkbox with real npm CLI. I should not pass npm coverage using HTTP mocks alone.
- [ ] I run every PyPI checkbox with real pip, uv, Poetry, or Twine client named by scenario. I should not pass Python coverage using HTTP mocks alone.
- [ ] I run every Rust checkbox with real Cargo CLI. I should not pass Cargo coverage using HTTP mocks alone.
- [ ] I run proxy tests in pull requests. I should use controlled local upstream fixtures and deterministic package data.
- [ ] I run public-registry compatibility tests on schedule. I should test npmjs, PyPI, and crates.io without making CI depend on their uptime.
- [ ] I fail one scenario. I should capture client command, redacted output, Harbor logs, upstream requests, cache state, and correlation ID.
- [ ] I run scenarios in random order and parallel. I should get identical results without shared package state.
