#!/bin/sh
# Upgrades PostgreSQL 18 data in 18/docker to 19/docker with pg_upgrade --link.
# Runs in tianon/postgres-upgrade:18-to-19 with the data volume as the working
# directory. The chart's values.yaml carries it inline, minus the root handoff.
set -eu

[ -s 19/docker/PG_VERSION ] && exit 0
if [ ! -s 18/docker/PG_VERSION ]; then
  for d in . data [0-9]*/docker pgdata pgdata/pg[0-9]*; do
    [ -s "$d/PG_VERSION" ] || continue
    echo "found PostgreSQL $(cat "$d/PG_VERSION") data in $d; only 18/docker is upgraded automatically" >&2
    exit 1
  done
  exit 0
fi

if [ "$(id -u)" = 0 ]; then
  chown -R postgres:postgres .
  exec gosu postgres "$0"
fi

# Retry after a failed run: drop the half-built cluster and undo pg_upgrade's
# link-mode marker, which is safe while the new cluster has never started.
rm -rf 19/upgrade
if [ -f 18/docker/global/pg_control.old ] && [ ! -f 18/docker/global/pg_control ]; then
  mv 18/docker/global/pg_control.old 18/docker/global/pg_control
fi

# Single-user mode runs as the bootstrap superuser, so no role name or password is needed.
single() {
  "$1/postgres" --single -D "$2" -j template1 2>/dev/null | sed -n 's/^[[:space:]]*1: r = "\(.*\)"[[:space:]].*/\1/p'
}
r=$(single "$PGBINOLD" 18/docker <<'SQL'
select concat_ws('|', pg_encoding_to_char(d.encoding), d.datlocprovider::text, d.datcollate,
  d.datctype, coalesce(d.datlocale, ''), current_setting('data_checksums'), a.rolname) as r
  from pg_authid a, pg_database d where a.oid = 10 and d.datname = 'template0'
SQL
)
# pg_upgrade records the upgrader's libc collation versions; keep the old ones,
# which describe how the indexes were built (NULL for musl, as in alpine images).
collv=$(single "$PGBINOLD" 18/docker <<'SQL'
select 'update pg_database d set datcollversion = v.c from (values ' ||
  string_agg(format('(%L, %L::text)', datname, datcollversion), ', ') ||
  ') v(n, c) where d.datname = v.n' as r from pg_database
SQL
)
[ -n "$r" ] && [ -n "$collv" ] || { echo "cannot read the PostgreSQL 18 cluster in 18/docker" >&2; exit 1; }
# The role name goes last: read leaves any '|' in it to the last variable.
IFS='|' read -r enc prov coll ctype loc sums user <<EOF
$r
EOF

set -- --username="$user" --encoding="$enc" --lc-collate="$coll" --lc-ctype="$ctype"
case $prov in
  i) set -- "$@" --locale-provider=icu --icu-locale="$loc" ;;
  b) set -- "$@" --locale-provider=builtin --builtin-locale="$loc" ;;
esac
[ "$sums" = on ] || set -- "$@" --no-data-checksums

mkdir -p 19
"$PGBINNEW/initdb" -D 19/upgrade "$@"
"$PGBINNEW/pg_upgrade" --link -U "$user" -b "$PGBINOLD" -B "$PGBINNEW" -d 18/docker -D 19/upgrade
echo "$collv" | "$PGBINNEW/postgres" --single -D 19/upgrade -j template1 >/dev/null
cp 18/docker/pg_hba.conf 18/docker/pg_ident.conf 18/docker/postgresql.auto.conf 19/upgrade/
mv -T 19/upgrade 19/docker
rm -f delete_old_cluster.sh
echo "upgraded 18/docker to 19/docker; remove 18 once PostgreSQL 19 runs"
