# Harbor Next on OpenShift

Deploy Harbor Next to an OpenShift cluster using ephemeral images from [ttl.sh](https://ttl.sh).

## Prerequisites

- OpenShift cluster with `oc` CLI configured
- `helm` v3.x
- `task` (taskfile.dev) for building images
- Docker or Podman for building/pushing images

## 1. Build and Push Images to ttl.sh

From the repository root:

```bash
task image:all-images IMAGE_REGISTRY=ttl.sh IMAGE_NAMESPACE=harbor-next
```

This pushes all Harbor images to `ttl.sh/harbor-next/harbor-<service>:v2.15.0`.

> **Note:** ttl.sh images expire after 1 hour for non-duration tags. Re-push if images expire before deployment completes.

## 2. Build Chart Dependencies and Install

PostgreSQL is included in the values file via `extraManifests` and will be deployed alongside Harbor automatically.

```bash
cd deploy/chart
helm dependency build
helm install harbor-next . -n vad1mo-dev -f example/openshift/values.yaml
```

## 3. Verify

```bash
# Check pods
oc get pods -n vad1mo-dev

# Check route
oc get route -n vad1mo-dev

# Wait for all pods to be ready
oc wait -n vad1mo-dev --for=condition=ready pod -l app.kubernetes.io/instance=harbor-next --timeout=300s
```

## 4. Test Push and Pull

```bash
HARBOR_URL=harbor-next-vad1mo-dev.apps.rm1.0a51.p1.openshiftapps.com

# Login
echo "${HARBOR_ADMIN_PASSWORD}" | docker login ${HARBOR_URL} -u admin --password-stdin

# Push a test image
docker pull alpine:latest
docker tag alpine:latest ${HARBOR_URL}/library/alpine:test
docker push ${HARBOR_URL}/library/alpine:test

# Pull it back
docker pull ${HARBOR_URL}/library/alpine:test
```

## 5. Verify Vulnerability Scanning

If Trivy is enabled, trigger and check a scan:

```bash
# Trigger scan via Harbor API
curl -s -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  -X POST "https://${HARBOR_URL}/api/v2.0/projects/library/repositories/alpine/artifacts/test/scan"

# Check scan result (wait a minute for scan to complete)
curl -s -u "admin:${HARBOR_ADMIN_PASSWORD}" \
  "https://${HARBOR_URL}/api/v2.0/projects/library/repositories/alpine/artifacts/test?with_scan_overview=true" | jq .scan_overview
```

## Cleanup

```bash
helm uninstall harbor-next -n vad1mo-dev
# PVCs are not deleted by helm uninstall — clean up manually
oc delete pvc -n vad1mo-dev postgres-data
oc delete pvc -l app.kubernetes.io/instance=harbor-next -n vad1mo-dev
```

## Troubleshooting

### SCC Violations

If pods fail to start with `SecurityContextConstraint` errors, verify that `securityContext: {}` and `podSecurityContext: {}` are set for all components in your values file. OpenShift's SCC system manages UIDs — hardcoded UIDs from the default chart values will be rejected.

### Route Not Accessible

Check that the route was created and has an admitted status:

```bash
oc get route -n vad1mo-dev
oc describe route -n vad1mo-dev
```

The ingress must use `route.openshift.io/termination: edge` annotation and must not specify an `ingressClassName`.

### ttl.sh Image Expiry

ttl.sh images expire after 1 hour for tags without a duration suffix. If pods show `ImagePullBackOff`, re-push images:

```bash
task image:all-images IMAGE_REGISTRY=ttl.sh IMAGE_NAMESPACE=harbor-next
oc delete pods -n vad1mo-dev -l app.kubernetes.io/instance=harbor-next
```

### Database Connection

Verify PostgreSQL is running and accessible:

```bash
oc get pods -n vad1mo-dev -l app=postgres
oc logs -n vad1mo-dev deployment/postgres
```

The `database.host` in values.yaml must match `postgres.<namespace>.svc.cluster.local`.
