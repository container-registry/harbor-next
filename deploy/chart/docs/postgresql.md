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

## Major upgrades

An init container runs [pgautoupgrade](https://github.com/pgautoupgrade/docker-pgautoupgrade)
with `PGAUTO_ONESHOT=yes` before PostgreSQL starts. If the data is from an
older major, it runs `pg_upgrade --link` in place and exits. If the versions
match, it exits at once. So a new major is one `helm upgrade` with a chart that
ships newer image tags.

From PostgreSQL 18 the subchart keeps data in `18/docker` on the volume;
before 18 it was `pgdata`. The init container moves `pgdata` there first.

There is no downgrade, and pgautoupgrade deletes the old cluster once it
succeeds, so take a backup first. If the upgrade fails, the pod loops in
`Init` and `$PGDATA/upgrade_in_progress.lock`, `old/` and `new/` remain on the
volume. If `pg_upgrade` stopped before linking files, `old/` still holds the
previous cluster.

If you pin `postgresql.image.tag` to another major, pin the init container
image and its `PGDATA` to the same major, or set `postgresql.initContainers: []`.
If you set `postgresql.auth.username`, add `POSTGRES_USER` with the same value
and `POSTGRES_DB` with the database name to the init container.
If you set `postgresql.persistence.subPath` or `volumeName`, give the init
container's data mount the same values. The chart refuses to render otherwise,
since the upgrader would miss the old data and PostgreSQL would start empty.

## Moving from goharbor/harbor-helm with the internal database

The old chart cannot be upgraded in place (Deployment selectors differ), so
uninstall it and install this chart under the same release name. The data
stays on the old `database-data-<release>-database-0` PVC. The init container
moves the old data directory to `18/docker` and upgrades it to PostgreSQL 18
when it is older.

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

4. The old image used another glibc, so PostgreSQL warns `database "registry"
   has a collation version mismatch`. Fix it once:

   ```sh
   kubectl -n harbor exec postgresql-0 -c postgresql -- sh -ec '
     reindexdb -h 127.0.0.1 -U postgres --all
     for d in postgres template1 registry; do
       psql -h 127.0.0.1 -U postgres -d $d -c "ALTER DATABASE $d REFRESH COLLATION VERSION"; done'
   ```

## Going back to goharbor/harbor-helm

Uninstall this chart first: `helm uninstall harbor -n harbor`.

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
