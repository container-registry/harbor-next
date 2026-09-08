# Global storage quota

An instance-wide storage limit for Harbor. It answers the question project quotas cannot: how much of the whole instance is used, and how much is left.

Decision record: [harbor-next #839](https://github.com/container-registry/harbor-next/issues/839), theme A.

## What it is

- One hard limit in bytes for the entire instance, set by a system administrator.
- One usage value with a declared source:
  - `accounted`: the sum of blob sizes Harbor knows about (foreign layers excluded) plus system artifacts. This is the same number the admin statistics card shows. It undercounts physical usage (upload temp directories, blobs waiting for garbage collection, objects Harbor did not write).
  - `measured`: the usage measured on the storage volume. Only available when storage usage reporting (theme B of #839) is deployed. A measured value always wins over the accounted one.
- An advisory sum of the storage limits assigned to projects, with the number of projects that have no limit, so an administrator can see over-commitment.
- An optional enforcement flag.

The global quota is independent of project quotas. Project quotas keep working exactly as before.

## Who can see it

System administrators, and system robot accounts granted `system-quota:read`. The portal shows it on the projects page (the "Quota used" card) and on the Project Quotas administration page. Project members do not see it; they keep seeing their project quota only.

## API

| Operation | Permission | Notes |
|---|---|---|
| `GET /api/v2.0/system/quota` | `system-quota:read` (admins, system robots) | `404` when no global quota is set |
| `PUT /api/v2.0/system/quota` | system administrator | body `{"hard": {"storage": <bytes>}, "enforce": <bool>}` |
| `DELETE /api/v2.0/system/quota` | system administrator | removes the quota |

Example:

```bash
# set 2 TiB, advisory only
curl -u admin:$PASS -X PUT -H 'Content-Type: application/json' \
  https://harbor.example.com/api/v2.0/system/quota \
  -d '{"hard": {"storage": 2199023255552}, "enforce": false}'

# read
curl -u admin:$PASS https://harbor.example.com/api/v2.0/system/quota
```

Response:

```json
{
  "hard": {"storage": 2199023255552},
  "used": {"storage": 1319413953331},
  "free": 879609302221,
  "used_source": "accounted",
  "allocated": 1649267441664,
  "unlimited_projects": 3,
  "enforce": false,
  "update_time": "2026-09-08T12:00:00Z"
}
```

`hard` must be between 1 byte and 1024 TiB. Use `DELETE` (or `-1` in the portal editor) to remove the quota.

## Enforcement

With `enforce: true`, Harbor denies registry writes once usage plus the request body would exceed the limit:

- Denied: manifest push, starting a blob upload, uploading blob chunks, completing a blob upload, and copying artifacts through the API. The response is `403` with the message `global storage quota exceeded: used X of Y`.
- Never denied: deleting manifests, artifacts, repositories or projects, garbage collection, and every administration API. Users can always free space.

The check is a read of cached state, so it adds no database lock to the push path. It is best-effort by design:

- Usage is cached for five minutes (`accounted`) or for the measurement interval (`measured`). Concurrent pushes inside that window can overshoot the limit by their combined size.
- If the usage query fails, the write is allowed and a warning is logged.
- Blob chunk uploads count their `Content-Length`; manifest pushes are checked against usage alone.

Enforcement is off by default.

## Metrics

When the metrics exporter runs inside core (`METRIC_EXPORTER_ENABLE`), these gauges are exposed while a global quota is set:

| Metric | Labels | Meaning |
|---|---|---|
| `harbor_system_quota_hard_bytes` | | the limit |
| `harbor_system_quota_used_bytes` | `source=accounted\|measured` | current usage |
| `harbor_system_quota_enforce` | | `1` when enforced, `0` when advisory |

## Storage

The quota lives in the `system_quota` table (one row, created by the harbor-next authoritative schema). No configuration item or environment variable is involved; the API is the single writer.
