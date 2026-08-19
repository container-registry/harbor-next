# Design: Native npm + Maven over OCI in Harbor ("multiformat")

Port the PROVEN standalone Go POC `multi-format-oci` into Harbor. Gate: maven AND npm packages
PUSH to Harbor and are VISIBLE in the UI with a sensible type label.

Reference POC (read to port): `/Users/vadim/Development/container-registry/multi-format-artifact-via-oci/multi-format-oci`
Harbor tree (target): `/Users/vadim/Development/container-registry/harbor.multi-format-artifact-support`

## 0. Verified facts that shape the design (DO NOT re-litigate; verified against source)

- Next migration file = `0181_2.15.0_schema.up.sql` (highest existing is `0180_2.15.0`; up-only, no .down.sql).
- `csrf.Middleware()` runs globally; `csrfSkipper` only exempts `/v2/ /api/ /service/`
  (`src/core/middlewares/middlewares.go`, `src/server/middleware/csrf/csrf.go`). **MUST add `/npm` and `/maven`
  to `csrfSkipper`** or every PUT 403s. **GATE BLOCKER.**
- `transaction.Middleware` wraps non-GET in a request tx. **Add `/npm`,`/maven` to `dbTxSkippers`** so publish
  manages its own tx and does not pin a pooled conn across registry HTTP round-trips.
- Blob rows + `project_blob` + `artifact_blob` are written ONLY in `src/server/middleware/blob/put_manifest.go`.
  Pushing via `registry.Cli` directly BYPASSES blob accounting → must replicate via `blob.Controller`
  (Sync / Ensure / AssociateWithProject* / AssociateWithArtifact). Skipping it = artifacts list but GC sweeps
  their blobs later. Required for correctness.
- `repository.Ctl.Ensure(ctx, name)` creates the repo row and MUST be called BEFORE `artifact.Ctl.Ensure`
  (`ensureArtifact` calls `repoMgr.GetByName` and errors if absent). Project must pre-exist (else 404, like docker).
- `artifact.Ctl.Ensure(ctx, repo, digest, &artifact.ArtOption{Tags: []string{tag}})` is the UI-visibility entry point.
- Abstractor sets `artifact.ArtifactType = manifest.ArtifactType` if set, else `= manifest.Config.MediaType`;
  type label = `processor.Get(ResolveArtifactType()).GetArtifactType()`.
  Default processor regexp `^application/vnd\.[^.]*\.(.*)\.config\.[^.]*\+json$` → `ToUpper(group1)`.
- Abstractor ACCEPTS ONLY `v1.MediaTypeImageManifest`/schema2 (per-version manifest) and
  `v1.MediaTypeImageIndex`/manifestlist (the `_index`). So per-version manifest MUST be `v1.MediaTypeImageManifest`
  and `_index` MUST be `v1.MediaTypeImageIndex`.
- `base.ManifestProcessor.UnmarshalConfig` short-circuits only when `Config.Size==0`; otherwise pulls the config
  blob. Our config blob is non-empty → the config blob MUST be pushed BEFORE `artifact.Ctl.Ensure`.
- Icons are PATH-based (`src/controller/icon/controller.go` maps digest → `./icons/*.png`); default fallback exists.
  Adding `npm`/`maven` icons is NOT gate-blocking (default icon + correct type label satisfies the gate).
- `registry.Cli` (`src/pkg/registry/client.go`): `PushManifest(repo,ref,mediaType,payload)→(digest,err)`,
  `PullManifest(repo,ref,...accept)`, `PushBlob(repo,digest,size,io.Reader)`, `PullBlob→(size,rc,err)`,
  `BlobExist`, `ManifestExist`, `ListTags`. Arbitrary media types supported. Authenticates as Harbor's internal
  registry credential (NOT the requesting user) → ALL RBAC must be enforced before any store call.

## 1. URL scheme + mapping (project = FIRST path segment after prefix)

npm — prefix `/npm`, client base `http://harbor/npm/<project>/`:
- publish `PUT /npm/<project>/<pkg>` ; packument `GET /npm/<project>/<pkg>` ;
  tarball `GET /npm/<project>/<pkg>/-/<file>.tgz` ;
  dist-tags `GET|PUT|DELETE /npm/<project>/-/package/<pkg>/dist-tags[/<tag>]`.
- `<pkg>` may be scoped `@scope/name`. mergeslash + net/http decode `%2f`→`/` BEFORE routing, so use a
  CATCH-ALL suffix route (`/npm/<project>/...`) and reconstruct the package name from raw remaining segments,
  treating a leading `@`-segment as scope. Do NOT rely on segment count. Test `@scope/name` publish + packument.

maven — prefix `/maven`, settings.xml `<url>http://harbor/maven/<project>/</url>`:
- `PUT|GET /maven/<project>/<g/with/slashes>/<a>/<v>/<file>` ; GA metadata
  `GET /maven/<project>/<g>/<a>/maven-metadata.xml` (synth) ; GAV SNAPSHOT metadata
  `GET /maven/<project>/<g>/<a>/<v>/maven-metadata.xml` (synth).

Object mapping: `<project>` → Harbor project (must pre-exist). npm pkg → repo `<project>/npm/<encoded-name>`.
maven `g:a` → repo `<project>/maven/<encoded-g>/<a>`. version → per-version OCI image manifest, tag=`EncodeTag(version)`.
package control record → `_index` OCI image index, reserved tag `_index` (NEVER Ensure'd → stays invisible).

### Repo-name grammar — REQUIRED codec change (GATE BLOCKER)
docker/distribution rejects repo path components not matching `[a-z0-9]+([._-][a-z0-9]+)*`. multi-format-oci's `EncodeRepo`
producing `com.example:demo` (`:` illegal) or raw scoped `@scope/name` (`@`, uppercase) fails with an opaque 400.
`naming.EncodeRepo` MUST emit grammar-legal LOWERCASE components only: maven group dot-split into path segments,
drop `:`; npm `@`→`_x40`, scope/name as separate segments or `_x2f`, lowercase everything. Add a unit test
`TestEncodeRepoGrammar` asserting every emitted component matches the distribution regexp.

## 2. Code placement (Harbor tree)

```
make/migrations/postgresql/0181_2.15.0_schema.up.sql      # multi_format_package, multi_format_version (NEW, up-only)
src/pkg/multiformat/model/model.go                            # PackageState, Version, FileRef (PORT verbatim)
src/pkg/multiformat/dao/dao.go                                # ORM DAO + advisory-lock helper (REWRITE on lib/orm)
src/pkg/multiformat/naming/naming.go                          # repo/tag/version codec (PORT; EncodeRepo grammar-fixed) + test
src/controller/multiformat/const.go                           # media types incl FORMAT-SPECIFIC config mediaTypes
src/controller/multiformat/store.go                           # store seam over registry.Cli (REPLACES multi-format-oci store)
src/controller/multiformat/index_ops.go                       # _index RMW, canonicalIndex (PORT verbatim)
src/controller/multiformat/mapper.go                          # Publish/SetDistTag/SetYanked/LoadState (PORT)
src/controller/multiformat/mapper_maven.go                    # PublishFile multi-file model (PORT)
src/controller/multiformat/controller.go                      # Controller iface + ctor
src/controller/multiformat/uivisibility.go                    # ensureVisible(): blob-assoc + repo.Ensure + artifact.Ensure
src/controller/multiformat/semver/                            # PORT verbatim (+tests)
src/controller/multiformat/mavenver/                          # PORT verbatim (+tests)
src/server/registry/npm/{route.go,handler.go}             # HTTP adapter (PORT internal/format/npm)
src/server/registry/maven/{route.go,handler.go}           # HTTP adapter (PORT internal/format/maven)
src/server/middleware/multiformatauth/auth.go                    # project resolve + RBAC Can (modeled on v2auth)
src/controller/artifact/processor/npm/npm.go              # type label NPM + ExtraAttrs
src/controller/artifact/processor/maven/maven.go          # type label MAVEN + ExtraAttrs
src/lib/icon/const.go                                     # + DigestOfIconNPM/Maven (EDIT, shared, additive) [optional]
src/controller/icon/controller.go                         # + builtInIcons entries (EDIT, shared, additive) [optional]
src/core/middlewares/middlewares.go                       # csrfSkipper + dbTxSkippers edits (EDIT, shared, SEQUENTIAL)
src/server/server.go                                      # RegisterRoutes(): mount npm + maven (EDIT, shared, SEQUENTIAL)
```
Reuse ~60% verbatim (semver, mavenver, model, index_ops, packument/metadata renderers, filename codec, checksum
derivation). Rewrite ~40% (store seam, DAO, routing/errors/logging, advisory lock, UI-visibility glue).
NOTE: multi-format-oci's single `VersionConfigMediaType` MUST be parameterized per format (npm vs maven config mediaType).

## 3. Store seam (multi-format-oci store.Store → registry.Cli) — method map
- PushBlob(repo,mt,data): dgst=digest.FromBytes(data); if !BlobExist{PushBlob(repo,dgst,len,Reader)}; return desc{mt,dgst,size}.
- FetchBlob(repo,desc): size,rc,_=PullBlob(repo,desc.Digest); return rc (stream straight to ResponseWriter).
- PushManifest(repo,tag,manifest): payload=json.Marshal; dgst=PushManifest(repo,tag,v1.MediaTypeImageManifest,payload).
- PushIndex(repo,tag,bytes): PushManifest(repo,tag,v1.MediaTypeImageIndex,bytes).
- FetchIndex(repo,"_index"): if !ManifestExist→notExist; m=PullManifest(repo,"_index",v1.MediaTypeImageIndex); m.Payload()→ocispec.Index.
- FetchManifest(repo,ref): m=PullManifest(repo,ref,v1.MediaTypeImageManifest); m.Payload()→ocispec.Manifest.
- IterTags(repo) [DR only]: ListTags(repo).
Gaps: no registry CAS on tags → `_index` RMW concurrency guaranteed SOLELY by the Postgres advisory lock.

## 4. Postgres projection (migration 0181)
```sql
CREATE TABLE multi_format_package (
  id BIGSERIAL PRIMARY KEY, project_id BIGINT NOT NULL, format VARCHAR(32) NOT NULL,
  native_name VARCHAR(512) NOT NULL, proj_version BIGINT NOT NULL DEFAULT 0,
  mutable_state JSONB NOT NULL DEFAULT '{}'::jsonb, last_index_digest VARCHAR(255) NOT NULL DEFAULT '',
  creation_time TIMESTAMP DEFAULT now(), update_time TIMESTAMP DEFAULT now(),
  UNIQUE (project_id, format, native_name));
CREATE TABLE multi_format_version (
  id BIGSERIAL PRIMARY KEY, package_id BIGINT NOT NULL REFERENCES multi_format_package(id) ON DELETE CASCADE,
  version VARCHAR(255) NOT NULL, payload_digest VARCHAR(255) NOT NULL DEFAULT '',
  payload_size BIGINT NOT NULL DEFAULT 0, yanked BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMP NOT NULL DEFAULT now(), meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  UNIQUE (package_id, version));
CREATE INDEX idx_multi_format_package_proj_fmt ON multi_format_package(project_id, format);
CREATE INDEX idx_multi_format_version_pkg ON multi_format_version(package_id);
```
Access via `src/lib/orm` (`orm.FromContext(ctx)`). DAO mirrors multi-format-oci: UpsertVersion, SetMutableState, SetYanked,
LoadState, ProjVersion, ListPackageNames, WipeAll. multi-format-oci `namespace TEXT` → `project_id BIGINT`.
Advisory lock + RMW commit ordering: acquire `pg_advisory_lock(hashtext($1))` on a dedicated conn held for the
critical section → read `_index` → immutability check → push version blobs+manifest → RMW `_index` (commit point)
→ projection upsert + proj_version bump in a short explicit `orm.WithTransaction` → `pg_advisory_unlock`.

## 5. UI-visibility mechanism (THE load-bearing path) — `ensureVisible(ctx, repo, manifestDigest, manifestPayload, tag)`
Called by the controller AFTER each per-version manifest push (npm: per Publish; maven: per PublishFile RMW).
NEVER called for `_index`. Order (replicating real OCI push path):
1. Resolve project (done in multiformatauth; project id on ctx).
2. Blob accounting (REQUIRED): parse config+layer descriptors; blob.Ctl.Sync(descriptors);
   AssociateWithProjectByDigest each; blob.Ctl.Ensure(manifestDigest,mediaType,size)+AssociateWithProjectByID;
   blob.Ctl.AssociateWithArtifact(blobDigests, manifestDigest).
3. repository.Ctl.Ensure(ctx, repo) — BEFORE step 4.
4. artifact.Ctl.Ensure(ctx, repo, manifestDigest, &artifact.ArtOption{Tags:[]string{tag}}).
Type label: use BOTH (belt+suspenders):
 (a) format-specific config mediaType: npm=`application/vnd.harbor.npm.config.v1+json`,
     maven=`application/vnd.harbor.maven.config.v1+json` (satisfy default regexp → NPM/MAVEN even with default processor);
 (b) register real processors `src/controller/artifact/processor/{npm,maven}` (embed base.NewManifestProcessor,
     like wasm/chart), GetArtifactType returns "NPM"/"MAVEN", AbstractMetadata parses config into ExtraAttrs.

## 6. Maven multi-file model on registry.Cli
One GAV = one multi-layer v1.MediaTypeImageManifest. Each file (pom, jar, -sources/-javadoc, SNAPSHOT timestamped)
is its own layer blob with mediaType MavenFileMediaType + per-layer annotations (filename/classifier/extension/
timestamp/buildNumber). Config blob = deterministic []FileRef JSON. RMW across separate PUTs under the per-GA
advisory lock: fetch current version manifest, add/replace layer for that filename (idempotent by digest, sorted by
filename), push blobs+manifest, RMW `_index`, commit PG. SnapshotMutable=mavenver.IsSnapshot(version): SNAPSHOT may
append timestamped filenames; same-filename diff-bytes → 409; release any same-filename diff-bytes → 409.
Derived checksums .sha1/.md5/.sha256/.sha512 over EXACT served bytes (md5+sha1 load-bearing). Client-uploaded
sidecars + maven-metadata.xml accepted-and-discarded (200, never stored). maven-metadata.xml SYNTHESIZED from
projection with `<lastUpdated>` from max version created_at (NOT wall-clock). ensureVisible after each PublishFile,
tag=mavenVersionTag(version); GAV shows as ONE artifact with N layers.

## 7. Auth/RBAC (`src/server/middleware/multiformatauth/auth.go`, modeled on v2auth)
security.Context is free on our routes (global security.Middleware). multiformatauth: parse project=first segment;
pid=project.Ctl.Get(ctx,name).ProjectID (404→challenge); resource=rbac_project.NewNamespace(pid).Resource(
rbac.ResourceRepository); GET/HEAD→ActionPull, PUT/POST→ActionPush, DELETE→ActionDelete; securityCtx.Can(...);
on fail 401 `WWW-Authenticate: Basic realm="harbor"`; stash pid on ctx. Both clients send HTTP Basic every request
(npm `_auth`; mvn `<server>`). Robot accounts (robot$proj+name:secret) work. Anonymous pull from public projects works.

## 8. Implementation waves (dependency-ordered)
Wave 0 (parallel, independent files): 0a migration 0181; 0b comparators semver+mavenver (verbatim+tests);
  0c model (verbatim); 0d naming codec + grammar-fixed EncodeRepo + TestEncodeRepoGrammar; 0e store seam;
  0f multiformatauth; 0g icons (optional, additive).
Wave 1 (→0): 1a DAO + advisory-lock helper; 1b core const incl format-specific config mediaTypes;
  1c npm processor; 1d maven processor.
Wave 2 (critical path, sequential): 2a index_ops (verbatim); 2b mapper; 2c mapper_maven; 2d uivisibility glue;
  2e controller iface+ctor.
Wave 3 (parallel: npm vs maven): 3a npm adapter; 3b maven adapter.
Wave 4 (STRICTLY SEQUENTIAL, shared files, last): 4a csrfSkipper + dbTxSkippers edits; 4b RegisterRoutes mount.

## 9. Verification gate (run on dev env SLOT=1)
- `cd src && go build ./...` green; `task test:lint`.
- `task dev:infra:up && task dev:db:migrate` applies 0181; confirm `\d multi_format_package`.
- npm: configure registry `http://127.0.0.1:<port>/npm/<project>/` + `_auth`; `npm publish`; verify artifact type NPM
  tag in `api/v2.0/projects/<p>/repositories`/artifacts; `npm install` round-trip.
- maven: `mvn deploy:deploy-file -Durl=.../maven/<project>/ -DgeneratePom=true`; verify type MAVEN; `mvn dependency:get`.
- CSRF negative: raw PUT with Basic only must NOT 403. RBAC: anon push to private → 401.
- UI: open portal in browser, confirm both packages visible with correct type labels.
