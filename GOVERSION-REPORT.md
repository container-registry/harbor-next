# Go 1.27.2 -> 1.27.3 (GO-2026-6809)
- harbor-next: https://github.com/container-registry/harbor-next/pull/1285 (branch chore/bump-go-1.27.3, pushed to next). Files: src/go.mod, versions.env, .oss-scanner/Dockerfile. Prefix chore(deps) follows #1231 precedent (hidden from notes).
- upstream: bump-go-1.27.3 pushed to bupd, PR to goharbor/harbor (see link below). Files: src/go.mod, Makefile, CI.yml, build-package.yml, tests/ci/distro_installer.sh.
- Verified on next: go1.27.3 build OK, govulncheck 0 called vulns. Upstream: build not run (gen_apis needs docker/perm), same go.mod change.
- Not merged.
