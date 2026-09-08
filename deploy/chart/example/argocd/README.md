# Argo CD (GitOps)

Argo CD `Application` for the `harbor` chart: published OCI chart plus
pinned-secret values tracked in git.

## Why `autoGenSecrets: false` is mandatory here

Argo CD renders charts **client-side** with `helm template` on every
sync. Helm's `lookup` function returns nothing there, so with the
default `autoGenSecrets: true` every sync regenerates all generated
secret material (encryption key, component identities, CSRF key,
registry htpasswd) and rolls every workload through the
`checksum/secret` annotations — a perpetual sync loop that also breaks
registry authentication mid-flight.

With `autoGenSecrets: false` the chart instead **fails at render time**
naming any value that still needs pinning, and rendering becomes
byte-for-byte deterministic — no `ignoreDifferences` workarounds needed.

## Usage

Requires Argo CD **2.6+** (multi-source Applications / `$values`).

1. Create the namespace and the identity Secrets. Names, keys, and
   generation commands are documented in
   [`../flux/identity-secrets.yaml`](../flux/identity-secrets.yaml) —
   they are identical for Argo CD. Manage them with ExternalSecrets,
   SealedSecrets, or SOPS; never commit plaintext.
2. Adjust [`values.yaml`](values.yaml): `externalURL`, database
   coordinates, ingress host/issuer, storage size.
3. Point [`application.yaml`](application.yaml) at your fork/values
   location, register the OCI repository with Argo CD
   (`argocd repo add 8gears.container-registry.com/8gcr/charts --type helm --enable-oci`),
   and apply:

   ```bash
   kubectl apply -f application.yaml
   ```

## Notes

- Rotating a pinned Secret does not restart pods — trigger a rollout
  (`kubectl rollout restart`) or run [Reloader](https://github.com/stakater/Reloader).
- The chart's deterministic-rendering property is CI-enforced
  (`task helm:gitops-determinism`); see the README's GitOps section for
  the full pinning reference.
