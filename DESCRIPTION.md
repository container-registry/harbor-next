# perf: Skip the request-wide DB transaction for POST /v2/&lt;name&gt;/blobs/uploads

## Idea

`dbTxSkippers` (src/core/middlewares/middlewares.go) already skip the request-wide
`transaction.Middleware` for PATCH and PUT blob upload via
`distribution.BlobUploadURLRegexp`, but that regexp requires a session id in the
path. POST `/v2/<name>/blobs/uploads` (initiate upload / cross-repo mount) has no
session id, so it did NOT match: every initiate-upload request ran inside a
`BEGIN ... COMMIT` that stayed open across the proxied registry round-trip,
holding one pooled Postgres connection per in-flight layer upload of every push.

The only DB writes on this route are for the cross-repo mount case
(`?mount=<digest>`): the blob middleware's after-hook associates the mounted blob
with the target project, and the quota middleware reserves the mounted blob's
size. The plain-initiate case reserves `{storage: 0}`, which
`updateUsageByDB` short-circuits without any UPDATE (`types.Equals` early
return), so it performs no writes at all.

## Change

- `src/core/middlewares/middlewares.go`: added
  `middleware.MethodAndPathSkipper(http.MethodPost, distribution.InitiateBlobUploadRegexp)`
  to `dbTxSkippers` (the regexp already existed in `src/pkg/distribution/distribution.go`)
  and extended the explanatory comment.
- `src/server/middleware/blob/post_initiate_blob_upload.go`: wrapped the
  mount-case after-hook (project lookup + `AssociateWithProjectByDigest`) in
  `orm.WithTransaction` with op name `tx-post-initiate-blob-mw`, exactly the
  pattern `PutBlobUploadMiddleware` uses (`tx-put-blob-mw`).
- `src/core/middlewares/middlewares_test.go`: added `Test_dbTxSkippers` covering
  POST initiate (skip), POST initiate with `?mount=` (skip), PATCH/PUT session
  URLs (skip), PUT manifest (not skipped), POST API route (not skipped).

The quota reserve path needs no change: `quotaController.Request` uses
optimistic-lock UPDATEs with its own retry/rollback compensation and already
runs tx-less on the PUT blob upload route (which has skipped the request tx
since the original skipper was introduced).

## Measured numbers

Same harness, same machine, alpine:latest via crane, Postgres `log_statement=all`:

| metric      | baseline main@177e51e99 | this branch | delta |
|-------------|------------------------|-------------|-------|
| push_stmts  | 286                    | 277         | -9    |
| push_tx     | 9                      | 3           | -6    |
| pull_stmts  | 55                     | 55          | 0     |

push_tx dropping 9 -> 3 confirms the initiate-upload BEGIN/COMMIT pairs are gone
(crane issued more than the theoretical 2 POSTs for this image in the harness
run; each removed tx saves its BEGIN + COMMIT). Pull is untouched by design:
GET/HEAD were already skipped.

The statement-count delta is modest for a 1-layer image; the real win is
connection-hold time: previously each POST held a pooled connection for the
entire proxied registry round-trip, now it holds none (plain initiate) or a
short manual tx (mount case). This scales with layer count (~6-10 POSTs per
single-arch push, ~100+ for a 17-manifest index) and with concurrency, which is
what matters for the shared multi-tenant Postgres.

## Risks

- Mount case loses atomicity with the HTTP response: the registry may return
  201 for the mount while the association tx fails, leaving the mounted blob
  temporarily unassociated with the project. Exposure is bounded:
  `blob.PutManifestMiddleware` unconditionally re-associates all referenced
  blobs with the project on the subsequent manifest PUT.
- Quota reserve for mounts now commits independently of the response; that is
  the same compensation semantics (`rollbackResources` on failure, `Refresh`
  heals drift) the PUT blob upload route has always had.
- `InitiateBlobUploadRegexp` is a prefix match and also matches session-id
  paths, but POST on a session URL is not part of the distribution API (405)
  and those paths are already skipped for PATCH/PUT anyway.

## Rollback

Revert the commit. The two functional edits are independent but should be
reverted together: reinstating the request-wide tx while keeping the manual tx
in the after-hook would only nest a savepoint, and keeping the skipper without
the manual tx would run the mount-case writes in autocommit (two independent
statements instead of one tx).
