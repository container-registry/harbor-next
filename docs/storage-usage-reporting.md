# Storage usage reporting

How Harbor learns the real size and usage of its registry storage volume.

Decision record: [harbor-next #839](https://github.com/container-registry/harbor-next/issues/839), theme B.

## What it is

The registry's storage is mounted into the registry process and into `registryctl`, never into `harbor-core`. That is why the old `GET /systeminfo/volumes` was wrong on Kubernetes: it measured the core container's own disk.

With storage usage reporting, `registryctl` measures the volume (`statfs` on the registry's `rootdirectory`) and core pulls that measurement:

```
harbor-core  --GET /api/registry/storage-->  registryctl  --statfs-->  registry volume
```

Nothing has to be deployed or configured. The Helm chart already runs `registryctl` next to the registry with the same volume, the Compose install already bind-mounts `/data/registry` into it, and core already knows `REGISTRY_CONTROLLER_URL` and the secret registryctl accepts.

## Scope

- Supported: the `filesystem` storage driver (a PVC, hostPath or bind mount).
- Not supported for now: object storage drivers (S3, GCS, Azure, Swift). They expose no capacity API. For them `/systeminfo/volumes` reports the real driver name with zero sizes and `measured: false`; the global storage quota keeps using Harbor's own blob accounting.

## Where the numbers show up

- `GET /api/v2.0/systeminfo/volumes` (system administrators): `total`, `free`, `used`, `driver`, `measured`, `measured_at`.
- The [global storage quota](global-quota.md): when a measurement is available, its `used` value replaces the accounted one and `used_source` becomes `measured`. Enforcement uses the measured value too.
- Prometheus (in-core exporter mode): `harbor_storage_volume_bytes{kind="total|free|used",driver="..."}` and `harbor_storage_volume_measured`.

## Behaviour details

- Core caches the measurement for one minute, shared across core replicas through Redis, so the global quota gate on the push path does not call registryctl per push.
- registryctl unreachable or not answering within five seconds: core logs a warning and falls back to its local disk (`total`, `free` and `used` of the core container) with `measured: false`. Nothing fails, and the fallback numbers are never published as Prometheus volume bytes.
- registryctl configured with a `rootdirectory` that does not exist: the endpoint answers with an error instead of a zero-sized "measurement", and core falls back as above.
- Several registry replicas on a shared (RWX) volume: every registryctl reports the same filesystem; core uses whichever replica the Service answers with. The numbers are not summed.
- `used` is `total - free` as seen by the filesystem, which includes upload temp directories and blobs waiting for garbage collection. That is intended: it is the physical usage.

## registryctl endpoint

`GET /api/registry/storage`, authenticated with the same secret header as the other registryctl APIs:

```json
{
  "driver": "filesystem",
  "supported": true,
  "path": "/storage",
  "total": 107374182400,
  "free": 32212254720,
  "used": 75161927680,
  "measured_at": "2026-09-08T12:00:00Z"
}
```

For an object store the response is `{"driver": "s3", "supported": false, ...}` with zero sizes.

## Development

The harbor-next dev environment starts registryctl only with `docker compose --profile registryctl up`. Without it, core reports `measured: false`.
