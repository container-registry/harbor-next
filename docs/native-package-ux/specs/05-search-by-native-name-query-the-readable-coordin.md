# Implementation Spec — Idea #5: Search-by-native-name

> REVISED for the storage-tree base change: repo names are now the readable tree
> (`library/maven/com/acme/widget2`), so the EscapeComponent-over-query mechanism is OBSOLETE.
> Acceptance is unchanged — the GOAL is that searching the readable coordinate finds the package:
> `GET /api/v2.0/projects/library/repositories?q=name=~com.acme:widget2` (and `q=name=~org.springframework.boot`)
> returns the matching repo. Mechanism now: `FilterByName` ORs the raw substring (preserves image
> matches like `nginx`) with a tree variant that rewrites `.`/`:` → `/` when the query contains them,
> so it matches the stored slash-delimited tree. Plain names (`widget2`, `lodash`,
> `spring-boot-starter-test`) already match via substring. No wire-format change.

## Goal
Make repository search match the readable native coordinate (`com.acme:widget2`, `harbor-multi-format-demo`) instead of only the escaped OCI storage path (`mcom_x2eacme_x3awidget2`, `mharbor_x2dmulti_x2dformat_x2ddemo`). No new endpoint, no schema change.

## Confirmed ground truth (read from the tree)
- `naming.EscapeComponent(s string) string` EXISTS (`src/pkg/multiformat/naming/naming.go:52`). It prepends a constant `p` prefix, passes lowercase-alnum bytes through verbatim, and escapes every other byte as `_xNN`. It is **per-byte and position-independent** apart from the leading `p`. Over-96-char output falls back to `p` + `h` + base32(sha256).
- `naming.Decode` does **NOT** exist (only `EscapeComponent`, `EncodeRepo`, `EncodeTag`). Search does **not** need Decode — only the forward `EscapeComponent`.
- `EncodeRepo` = `<project>/<format>/EscapeComponent(nativeName)` (`naming.go:85`). Project + format segments are verbatim; only the name segment is escaped. So matching the escaped name substring against `repository.name` is sufficient and correct.
- Substring math: `EscapeComponent("com.acme:widget")` = `mcom_x2eacme_x3awidget`, which is an `icontains` substring of the stored `mcom_x2eacme_x3awidget2`. `EscapeComponent("harbor-multi-format")` = `mharbor_x2dmulti_x2dformat`, a substring of `mharbor_x2dmulti_x2dformat_x2ddemo`. Confirmed against the two existing test repos.
- Query pipeline: frontend sends `q=name=~<value>` (`repository-gridview.component.ts:228,425,473`). `q.Build` parses `name=~` into keyword `Name` holding a `*q.FuzzyMatchValue`. `src/lib/orm/query.go:200` turns a `FuzzyMatchValue` into `name__icontains` UNLESS the model defines a `FilterByName` method, in which case `setFilters` (`query.go:195`) dispatches to that method (`mk.FilterFunc != nil`). This is exactly the `FilterByBlobDigest` precedent (`src/pkg/repository/model/model.go:45`).
- `repository.name` is the indexed model column on `RepoRecord` (`model.go:35`, `TableName()` -> `repository`).
- `multi_format_package` schema (`make/migrations/postgresql/0181_2.15.0_schema.up.sql`): `native_name VARCHAR(512)`, `UNIQUE (project_id, format, native_name)`, index `idx_multi_format_package_proj_fmt ON multi_format_package(project_id, format)`. There is **no index supporting `native_name LIKE '%x%'`**.

## Chosen approach (minimal): server-side query rewrite via the model FilterBy seam
Add `FilterByName` to `RepoRecord` so the existing `Name` keyword dispatches through it. Inside, OR two `LIKE` conditions over `repository.name`: the **raw** value (preserves today's image-repo matches) and `EscapeComponent(value)` (adds native-coordinate matches). This rides the existing `q`/keyword/FilterFunc pipeline with zero swagger/gen-apis/migration.

### Why not the `multi_format_package.native_name` (FilterByNativeName) path
`multi_format_package` is not the `RepoRecord` model; matching it requires a raw-SQL subquery join (`repository_id` is not in `multi_format_package`; the join key is `project_id`+`format`+name), and `native_name` has no substring index. It is more code, slower, and the indexed `repository.name` already yields correct results after the codec rewrite. Documented as the "richer" alternative only; not implemented.

## Files to touch
1. `src/pkg/repository/model/model.go` — add `FilterByName`, mirroring `FilterByBlobDigest`:
   - Compute `escaped := naming.EscapeComponent(value)`.
   - Return `qs.FilterRaw("name", "like <quoted %raw%> or name like <quoted %escaped%>")` using `orm.QuoteLiteral` and escaping LIKE metacharacters (`%`, `_`) in both operands (the encoded names contain literal `_`).
   - Import `github.com/goharbor/harbor/src/pkg/multiformat/naming`.
2. `src/server/v2.0/handler/repository.go` — none strictly required once `FilterByName` exists (keyword `Name` auto-dispatches in both `ListRepositories` and `ListAllRepositories`). Optionally add a guard so `EscapeComponent` only runs when `value` has a non-`[a-z0-9]` byte, to skip the transform for plain OCI searches. Keep logging on `src/lib/log`.

## API / schema
None. `q` already exists on `GET /projects/{name}/repositories` and `GET /repositories`. No `task build:gen-apis`. No migration.

## Frontend
No functional change. Existing `name=~` wire format is unchanged; the spec assertion `params.q === encodeURIComponent('name=~nginx')` stays green. Portal-visible rollout should wait for Idea #2 (decoded-name rendering) so results don't show escaped paths.

## Verification (SLOT=1)
See the verification field — baseline empty result for `q=name=~com.acme:widget`, post-fix returns `library/maven/mcom_x2eacme_x3awidget2`; npm `harbor-multi-format` returns the npm repo; `nginx`/`widget` regressions preserved; same against the global `/repositories` endpoint.

## Top risks
- `FilterByName` now owns ALL repository name searches — the raw-value OR branch must preserve image-repo matches (regression-test `nginx`).
- LIKE wildcard injection from literal `_` in encoded names — escape `%`/`_` or matches go over-broad. Highest-correctness risk.
- >96-char native names use the sha256 fallback encoding and won't substring-match (rare; out of scope).