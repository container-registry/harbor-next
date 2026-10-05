# Distribution patches

Patches applied to [distribution/distribution](https://github.com/distribution/distribution)
before `task build:binary:registry:<platform>` compiles the registry binary.
The list and order live in `DISTRIBUTION_PATCHES` in the root `Taskfile.yml`;
a file in this directory that is not listed there is not applied.

Patched builds report their version as `<DISTRIBUTION_VERSION>+patched.<N>`
(e.g. `3.1.1+patched.2`), so `registry --version` and the startup log show
that the binary is not stock upstream.

| Patch | Upstream | Effect |
| ----- | -------- | ------ |
| `0001-s3-bound-pooled-part-buffer-growth-to-chunk-size.patch` | [distribution#4919](https://github.com/distribution/distribution/pull/4919) | S3 part buffer capped at `chunksize` instead of settling near 2x |
| `0002-s3-add-spooldir-to-buffer-upload-parts-on-disk.patch` | [distribution#4920](https://github.com/distribution/distribution/pull/4920) | Optional `storage.s3.spooldir` keeps pending upload parts on disk instead of in memory |

Each patch carries an `Upstream:` trailer. Drop a patch once the upstream PR
is merged and released in the `DISTRIBUTION_VERSION` we build.

## Adding or refreshing a patch

Patches must apply with plain `git apply` to the `DISTRIBUTION_VERSION` tag.
Upstream PRs usually target `main`, so backport them onto the tag:

```bash
git clone https://github.com/distribution/distribution.git && cd distribution
git checkout -b backport "$(grep ^DISTRIBUTION_VERSION= ../harbor-next/versions.env | cut -d= -f2)"
git fetch origin pull/<N>/head:pr<N>
git cherry-pick pr<N>          # resolve conflicts against the tag
git commit --amend --no-edit --trailer "Upstream: https://github.com/distribution/distribution/pull/<N>"
git format-patch --zero-commit --no-signature --start-number <next free number> -o ../harbor-next/patches/distribution <tag>..backport
```

Then add the file to `DISTRIBUTION_PATCHES`. When Renovate bumps
`DISTRIBUTION_VERSION`, the registry build fails until every listed patch is
rebased onto the new tag or removed.
