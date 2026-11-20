# In-cluster PostgreSQL

For installs without CloudNativePG or a managed database, the chart can run
PostgreSQL itself with the
[cloudpirates/postgres](https://github.com/CloudPirates-io/helm-charts/tree/main/charts/postgres)
subchart and the official `postgres` image:

```yaml
postgresql:
  enabled: true
```

Harbor connects to the `postgresql` Service as `postgres`, database
`registry`, with `postgres-password` from the `postgresql` Secret. The subchart
generates that password unless you set `postgresql.auth.password` or
`postgresql.auth.existingSecret`. `database.*` is ignored. Any other
cloudpirates/postgres value can be set under `postgresql:`.

## Upgrading from a 2.15 chart

A release installed from a harbor-next 2.15 chart stores values with no
`postgresql.enabled`. With `helm upgrade --reuse-values`, Helm keeps those old
values and skips the new chart defaults, so Helm would deploy the subchart
nobody asked for. The chart refuses that upgrade with an error. Pass your
values file (`helm upgrade -f values.yaml`) or use `--reset-then-reuse-values`
(Helm 3.14+); both pick up the new defaults, including
`postgresql.enabled: false`.

## Major upgrades

The chart runs PostgreSQL 19, with the data in `19/docker` on the volume. An
init container, `pgdata-guard`, checks the volume before PostgreSQL starts. It
passes when the volume is empty or already holds PostgreSQL 19 data. When it
finds data of an older major (`18/docker`, or `pgdata` and `pgdata/pgNN` from
older layouts), it logs where and exits with an error, and the pod stays in
`Init`. Without the guard, PostgreSQL 19 would start an empty database next to
data in the `pgdata` layouts.

Charts up to 2.16 ran [pgautoupgrade](https://github.com/pgautoupgrade/docker-pgautoupgrade)
there instead, which upgraded older data in place. pgautoupgrade has no
PostgreSQL 19 image yet, so the upgrade from PostgreSQL 18 is a dump and
restore. Take it before you upgrade the chart; the release is called `harbor`
in namespace `harbor` here:

```sh
stop_writers() {
  kubectl -n harbor scale deploy harbor-core harbor-jobservice --replicas=0 &&
  kubectl -n harbor wait --for=delete pod --timeout=5m \
    -l 'app.kubernetes.io/instance=harbor,app.kubernetes.io/component in (core,jobservice)'
}

# 1. With the old chart: stop Harbor's writers, dump, check the dump
stop_writers
kubectl -n harbor exec postgresql-0 -c postgresql -- \
  sh -c 'pg_dump -U "${POSTGRES_USER:-postgres}" -d "$POSTGRES_DB"' > harbor.sql
grep -q -- '-- PostgreSQL database dump complete' harbor.sql && echo "dump OK"

# 2. Upgrade the chart; the PostgreSQL pod stops in Init at the guard.
#    The upgrade starts core again, and core would migrate the new empty
#    database before the restore, so stop the writers once more.
helm upgrade harbor <chart> -n harbor -f values.yaml
stop_writers

# 3. Only after "dump OK": replace the volume with an empty one
kubectl -n harbor delete pvc data-postgresql-0 --wait=false
kubectl -n harbor delete pod postgresql-0

# 4. Restore, and start Harbor only when the restore succeeded
kubectl -n harbor wait --for=condition=Ready pod/postgresql-0 --timeout=5m &&
kubectl -n harbor exec -i postgresql-0 -c postgresql -- \
  sh -c 'psql -v ON_ERROR_STOP=1 -q -U "${POSTGRES_USER:-postgres}" -d "$POSTGRES_DB"' < harbor.sql > /dev/null &&
echo "restore OK" &&
kubectl -n harbor scale deploy harbor-core harbor-jobservice --replicas=1
```

Keep `harbor.sql` and the chart's `secretKey` together until the upgraded
Harbor works. Step 3 deletes the old data, so do not run it without `dump OK`.
If step 4 does not print `restore OK`, repeat steps 3 and 4. Set the replicas
in step 4 to your values; with an HPA, `helm upgrade` restores them.

Step 3 assumes the claim the subchart creates, `data-postgresql-0`
(`<persistence.volumeName>-postgresql-0` if you renamed the volume). With
`postgresql.persistence.existingClaim`, the chart does not create the claim:
point `existingClaim` at a new, empty claim instead, and keep the old one
until the upgrade works.

To stay on PostgreSQL 18 instead, pin `postgresql.image.tag` to an 18 tag and
set `postgresql.initContainers` to the pgautoupgrade 18 init container from the
2.16 chart's `values.yaml`. Use `[]` only when the volume already holds
`18/docker`; the pgautoupgrade init container is what moves older layouts there.

If you set `postgresql.persistence.subPath` or `volumeName`, give the init
container's data mount the same values. The chart refuses to render otherwise,
since the guard would miss the old data and PostgreSQL would start empty.

## Moving from goharbor/harbor-helm with the internal database

The old chart cannot be upgraded in place (Deployment selectors differ), so
uninstall it and install this chart under the same release name. The data
stays on the old `database-data-<release>-database-0` PVC. Install the 2.16
chart for this move: its init container moves the old data directory to
`18/docker` and upgrades it to PostgreSQL 18 when it is older. The old image
used another glibc, so on the first start it also runs `reindexdb --all` and
`ALTER DATABASE ... REFRESH COLLATION VERSION`. Later starts find nothing to
do. Then go to PostgreSQL 19 as described in [Major upgrades](#major-upgrades).
This chart's guard refuses the old directory and does not move it.

| Harbor | harbor-helm | PostgreSQL | Data directory on the PVC |
|---|---|---|---|
| 2.8 | 1.12 | 13 | `pgdata/pg13` |
| 2.9 to 2.10 | 1.13 to 1.14 | 14 | `pgdata/pg14` |
| 2.11 to 2.15.1 | 1.15 to 1.19.1 | 15 | `pgdata/pg15` |
| 2.15.2 and newer | 1.19.2 and newer | 18 | `pgdata/pg18` |

Check yours with
`kubectl -n harbor exec harbor-database-0 -c database -- ls /var/lib/postgresql/data/pgdata`.

1. Read the old secrets and take a backup:

   ```sh
   kubectl -n harbor get secret harbor-database -o jsonpath='{.data.POSTGRES_PASSWORD}' | base64 -d
   kubectl -n harbor get secret harbor-core -o jsonpath='{.data.secretKey}' | base64 -d
   kubectl -n harbor exec harbor-database-0 -c database -- pg_dumpall -U postgres > all.sql
   ```

2. `helm uninstall harbor -n harbor`. The database PVC and `harbor-registry`
   stay.

3. Install with:

   ```yaml
   secretKey: <secretKey>
   registry:
     persistence:
       enabled: true
       existingClaim: harbor-registry
   postgresql:
     enabled: true
     auth:
       password: <POSTGRES_PASSWORD>
     persistence:
       existingClaim: database-data-harbor-database-0
   ```

## Going back to goharbor/harbor-helm

Uninstall this chart first: `helm uninstall harbor -n harbor`.

Once the data is PostgreSQL 19, only the dump from step 1 brings you back.
While it is still on the 2.16 chart:

If the old data was PostgreSQL 15 or older, it is PostgreSQL 18 now and the
old image cannot read it. Restore the dump from step 1 into a fresh old
install. Anything written after the move is lost. The harbor-db image has
no `tar` before 2.15.2, so the dump goes through `kubectl exec`, not
`kubectl cp`.

```sh
kubectl -n harbor delete pvc database-data-harbor-database-0
helm install harbor harbor/harbor --version <old chart> -n harbor -f old-values.yaml --wait
kubectl -n harbor scale deploy harbor-core harbor-jobservice --replicas=0
kubectl -n harbor exec harbor-database-0 -c database -- psql -U postgres -c 'DROP DATABASE registry WITH (FORCE)'
kubectl -n harbor exec -i harbor-database-0 -c database -- psql -U postgres -q < all.sql
kubectl -n harbor scale deploy harbor-core harbor-jobservice --replicas=1
```

The restore prints `ERROR:  role "postgres" already exists`, which is
harmless.

If the old data was already PostgreSQL 18 (`pgdata/pg18`), move it back and
reinstall the old chart with the old values:

```sh
kubectl -n harbor apply -f - <<'YAML'
apiVersion: v1
kind: Pod
metadata:
  name: pg-move-back
spec:
  restartPolicy: Never
  securityContext:
    runAsUser: 999
    runAsGroup: 999
  containers:
    - name: mv
      image: docker.io/library/busybox:1.37
      command: ["sh", "-ec", "mkdir -p /data/pgdata && mv /data/18/docker /data/pgdata/pg18 && rmdir /data/18"]
      volumeMounts:
        - name: data
          mountPath: /data
  volumes:
    - name: data
      persistentVolumeClaim:
        claimName: database-data-harbor-database-0
YAML
kubectl -n harbor wait --for=jsonpath='{.status.phase}'=Succeeded pod/pg-move-back
kubectl -n harbor delete pod pg-move-back
helm install harbor harbor/harbor --version <old chart> -n harbor -f old-values.yaml --wait
```

Moving the directory back only works while the Harbor schema is still the
old release's. Check it with `SELECT version FROM schema_migrations` before
you uninstall this chart. Harbor Next 2.15 keeps schema 181, the same as
upstream 2.15.3, and upstream core starts on it with `No change in schema,
skip.`. Harbor Next 2.16 migrates to 190, and upstream 2.15 core then exits
with `failed to migrate the database, error: no migration found for version
190`. Then the dump is the only way back. Tables that Harbor Next adds stay in the
database and upstream ignores them.
