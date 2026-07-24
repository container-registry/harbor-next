# Issue 478 public-image workload

`issue478-images.csv` is a 597-image input manifest generated on 2026-07-24.
Each row is a currently resolvable tag returned by the Docker Hub tag API for
the listed upstream repository. The pool covers released Linux tags from Go,
Alpine, BusyBox, Node, Nginx, PostgreSQL, and Valkey.

The CSV deliberately includes several generations and base-image variants
(Alpine, Bookworm, Bullseye, Trixie, slim, and so on). That gives the scan
benchmark real operating-system and language-runtime CVE distributions instead
of the zero-vulnerability synthetic scratch image baseline.

Regenerate this snapshot with:

```sh
task --taskfile bench/Taskfile.yml images:list
```

The generator samples two Docker Hub result pages ordered by recency and in
both lexical directions, filters out Windows, prerelease, test, and digest-like
names, and selects evenly across the version-sorted candidates. It emits
between 500 and 800 references or fails. The image reference and the Docker Hub
source URL are retained for each row so that the workload is auditable before
any pull or push is started.

This is an input manifest only: no image has been pulled, pushed to Harbor, or
scanned as part of creating it.

## Lock and mirror

The scan workload must use immutable Linux/amd64 content. Resolve the tag
snapshot, then stream the resulting manifests and layers directly into the
isolated `issue478` Harbor project:

```sh
task --taskfile bench/Taskfile.yml images:lock
task --taskfile bench/Taskfile.yml images:mirror
```

The lock file records the digest and the source used for every reference. The
primary source is `mirror.gcr.io`, with Docker Hub as the one fallback. Copies
are sequential and use `crane copy`; layers are never stored in the local
container engine. If neither source resolves or copies an image, the task stops
instead of silently changing the workload.
