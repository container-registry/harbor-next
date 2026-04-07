# Agent Test Workflow: BuildKit and Buildah Attestation Accessories

## Goal

Validate this feature end to end:

- start Harbor from a clean state
- push multi-arch images that produce attestations
- verify Harbor stores those attestations as accessories
- verify they do not show up as normal `unknown/unknown` child artifacts

This is an agentic/manual validation workflow. It is not a Robot test plan.

## Non-Negotiable Run Order

Run the workflow in this order every time:

1. `task clean`
2. `task dev:up` in the background
3. wait for Harbor health
4. push the BuildKit sample
5. push the Podman/Buildah sample
6. verify Harbor via API first, UI second

Do not skip the clean step. This feature touches artifact ingestion and migration behavior, so stale DB state is a bad test input.

## Success Criteria

For each tested repo:

1. The top-level OCI index keeps only real platform children in `references`.
2. No attestation manifest appears as a normal `unknown/unknown` child reference.
3. Each platform child manifest exposes one or more accessories with type `attestation.intoto`.
4. In the UI, attestations appear under accessories rather than as separate artifact rows.

## Prerequisites

- `task`
- `curl`
- `jq`
- `docker` with a real Docker Engine and BuildKit/buildx for the BuildKit scenario
- `podman` for the Podman/Buildah scenario

Optional but helpful:

- `docker buildx imagetools`

## Tooling Preflight

### BuildKit host check

This must report Docker Buildx, not a Podman compatibility layer:

```bash
docker buildx version
```

If it prints `buildah ...`, move the BuildKit scenario to a real Docker machine.

### Podman/Buildah capability check

Make sure the local Podman path exposes the build flags used below:

```bash
podman buildx build --help | rg -- '--manifest|--platform|--sbom'
podman manifest push --help | rg -- '--all'
```

## Start Harbor From Scratch

```bash
task clean
mkdir -p tmp/agent-test-buildkit-buildah-attestation-accessories
task dev:up > tmp/agent-test-buildkit-buildah-attestation-accessories/harbor.log 2>&1 &
export HARBOR_DEV_PID=$!
```

Wait for Harbor:

```bash
until curl -sf http://localhost:8080/api/v2.0/ping >/dev/null; do
  sleep 5
done
```

If you want to stop the stack when done:

```bash
kill "$HARBOR_DEV_PID" 2>/dev/null || true
```

## Shared Setup

```bash
export HARBOR=http://localhost:8080
export AUTH=admin:Harbor12345
export PROJECT=attestation-e2e
export TMPROOT=$(mktemp -d)
```

Create the project once:

```bash
curl -su "$AUTH" -H 'Content-Type: application/json' \
  -X POST "$HARBOR/api/v2.0/projects" \
  -d "{\"project_name\":\"$PROJECT\",\"public\":true}" || true
```

Log in with both clients:

```bash
echo 'Harbor12345' | docker login localhost:8080 -u admin --password-stdin
echo 'Harbor12345' | podman login --tls-verify=false localhost:8080 -u admin --password-stdin
```

Create a minimal build context:

```bash
cat >"$TMPROOT/Dockerfile" <<'EOF'
FROM alpine:3.20
RUN echo hello > /hello.txt
CMD ["cat", "/hello.txt"]
EOF
```

## Harbor Assertion Helper

Run the same Harbor-side assertions for each repo/tag pair:

```bash
verify_attestation_accessories() {
  local repo="$1"
  local ref="$2"
  local outdir="$TMPROOT/$repo"

  mkdir -p "$outdir"

  curl -su "$AUTH" \
    "$HARBOR/api/v2.0/projects/$PROJECT/repositories/$repo/artifacts/$ref?with_accessory=true" \
    | tee "$outdir/index.json"

  jq -e '.manifest_media_type == "application/vnd.oci.image.index.v1+json"' "$outdir/index.json" >/dev/null
  jq -e '.references | length >= 2' "$outdir/index.json" >/dev/null
  jq -e 'all(.references[]; .platform.architecture != "unknown" and .platform.os != "unknown")' "$outdir/index.json" >/dev/null
  jq -e '[.references[] | select(.annotations["vnd.docker.reference.type"] == "attestation-manifest")] | length == 0' "$outdir/index.json" >/dev/null

  for dgst in $(jq -r '.references[].child_digest' "$outdir/index.json"); do
    local child_out="$outdir/${dgst//:/_}.accessories.json"
    curl -su "$AUTH" \
      "$HARBOR/api/v2.0/projects/$PROJECT/repositories/$repo/artifacts/$dgst/accessories" \
      | tee "$child_out"

    jq -e 'map(select(.type == "attestation.intoto")) | length >= 1' "$child_out" >/dev/null
  done
}
```

If `verify_attestation_accessories` fails, the feature is not working as expected.

## Scenario 1: BuildKit Multi-Arch Image

Use a dedicated repo:

```bash
export BK_REPO=demo-buildkit
export BK_TAG=bk-$(date -u +%Y%m%d%H%M%S)
export BK_REF=localhost:8080/$PROJECT/$BK_REPO:$BK_TAG
```

Create or select a builder:

```bash
docker buildx create --name harbor-attest --use >/dev/null 2>&1 || docker buildx use harbor-attest
```

Build and push:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --provenance=true \
  --sbom=true \
  --push \
  -t "$BK_REF" \
  "$TMPROOT"
```

Capture registry-side evidence:

```bash
docker buildx imagetools inspect "$BK_REF" \
  | tee "$TMPROOT/buildkit-imagetools.txt"
```

Validate Harbor:

```bash
verify_attestation_accessories "$BK_REPO" "$BK_TAG"
```

## Scenario 2: Podman / Buildah Multi-Arch Image

Use a different repo:

```bash
export POD_REPO=demo-podman
export POD_TAG=pd-$(date -u +%Y%m%d%H%M%S)
export POD_REF=localhost:8080/$PROJECT/$POD_REPO:$POD_TAG
export POD_LIST=$PROJECT-$POD_REPO-$POD_TAG
```

Build both architectures into one local manifest list with SBOM generation enabled:

```bash
podman manifest rm "$POD_LIST" 2>/dev/null || true

podman buildx build \
  --platform linux/amd64 \
  --manifest "$POD_LIST" \
  --sbom syft \
  -t "$POD_REPO:amd64-$POD_TAG" \
  "$TMPROOT"

podman buildx build \
  --platform linux/arm64 \
  --manifest "$POD_LIST" \
  --sbom syft \
  -t "$POD_REPO:arm64-$POD_TAG" \
  "$TMPROOT"
```

Capture the local manifest metadata before push:

```bash
podman manifest inspect "$POD_LIST" \
  | tee "$TMPROOT/podman-manifest-inspect.json"
```

Push the full manifest list:

```bash
podman manifest push --all --tls-verify=false "$POD_LIST" "docker://$POD_REF"
```

Validate Harbor:

```bash
verify_attestation_accessories "$POD_REPO" "$POD_TAG"
```

### Important Note For This Scenario

This workflow assumes the local Podman/Buildah version emits the attestation artifacts needed to exercise this Harbor feature.

If the local client produces a multi-arch image but no attestation artifacts, that run does not count as feature validation. In that case:

1. keep the Harbor evidence
2. keep `podman manifest inspect` output
3. record the client-side limitation explicitly

## Optional UI Verification

Use the UI only after the API assertions pass.

1. Open `http://localhost:4200`
2. Log in as `admin` / `Harbor12345`
3. Open `$PROJECT/$BK_REPO`
4. Open tag `$BK_TAG`
5. Confirm the index expands to real platform manifests only
6. Confirm attestation entries appear under accessories
7. Confirm there are no standalone `unknown/unknown` rows for the attestations
8. Repeat for `$POD_REPO:$POD_TAG`

## Evidence To Save

Keep these files:

- `tmp/agent-test-buildkit-buildah-attestation-accessories/harbor.log`
- `$TMPROOT/buildkit-imagetools.txt`
- `$TMPROOT/podman-manifest-inspect.json`
- `$TMPROOT/demo-buildkit/index.json`
- `$TMPROOT/demo-podman/index.json`
- every child accessory JSON captured by `verify_attestation_accessories`

## Final Agent Report Format

The final report should include:

1. whether `task clean` was run first
2. whether `task dev:up` was run in the background
3. the BuildKit image ref
4. the Podman/Buildah image ref
5. the child manifest digests for each repo
6. the accessory digests returned by Harbor for each child manifest
7. whether any `unknown/unknown` child references appeared
8. final pass/fail for BuildKit
9. final pass/fail for Podman/Buildah
