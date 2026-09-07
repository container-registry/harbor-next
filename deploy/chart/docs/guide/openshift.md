# Deploying Harbor on OpenShift

A complete, working values file for this scenario lives at
[`example/openshift/values.yaml`](../../example/openshift/values.yaml);
the step-by-step walkthrough (build images, install, verify push/pull and
scanning, troubleshoot) is in
[`example/openshift/README.md`](../../example/openshift/README.md).
This guide explains the OpenShift-specific decisions behind that file —
what you must change compared to a vanilla Kubernetes install, and why.

## Routing: Ingress → Route with edge termination

OpenShift's router admits Ingress resources and converts them to Routes,
but only if the Ingress carries the termination annotation and does **not**
name an `ingressClassName`. TLS is terminated at the router edge, so the
chart's own TLS stays off and the Ingress `tls` list stays empty:

```yaml
externalURL: "https://harbor.apps.<cluster-domain>"

ingress:
  enabled: true
  className: ""                            # OpenShift router — no IngressClass
  annotations:
    route.openshift.io/termination: edge
  core: harbor.apps.<cluster-domain>
  hosts:
    - host: harbor.apps.<cluster-domain>
      paths:
        - path: /
          pathType: Prefix
  tls: []                                  # router terminates TLS

tls:
  enabled: false                           # edge termination — no in-cluster TLS
```

## Security contexts: let SCCs assign UIDs

OpenShift's restricted SCC assigns each namespace a UID range and rejects
pods that hardcode UIDs outside it. The chart's defaults pin
`runAsUser`/`runAsGroup`/`fsGroup`, so every component's security context
fields must be nulled out to let the SCC inject its own values.

There is **no chart-wide `securityContext` value** — the settings live per
component (`core`, `registry`, `portal`, `jobservice`, `exporter`,
`trivy`), and `values.schema.json` rejects unknown top-level keys at
install time. The pattern, repeated for each component:

```yaml
core:
  securityContext:
    runAsNonRoot: null
    runAsUser: null
    runAsGroup: null
    readOnlyRootFilesystem: null
    allowPrivilegeEscalation: null
    capabilities: null
    seccompProfile: null
  podSecurityContext:
    fsGroup: null
```

The Valkey subchart needs the same treatment under its own keys
(`valkey.securityContext` / `valkey.podSecurityContext`). The
[example values file](../../example/openshift/values.yaml) carries the
complete set for all components.

## Storage

Persistence is configured per component, not via a global `persistence`
block. Set your cluster's storage class on each one you enable:

```yaml
registry:
  persistence:
    enabled: true
    storageClass: "gp3"
    size: 10Gi

trivy:
  persistence:
    enabled: true
    storageClass: "gp3"
    size: 5Gi
```

## Database

The chart requires an external PostgreSQL. The example deploys one
alongside Harbor via `extraManifests` using
`registry.redhat.io/rhel9/postgresql-16` — the Red Hat image runs under
the restricted SCC without any tweaks, which stock `postgres` images do
not. Point `database.host` at the resulting Service
(`postgres.<namespace>.svc.cluster.local`).

## Verifying access

Harbor is reachable at `externalURL` once all pods are Ready. If the route
does not answer, check `oc get route` / `oc describe route` and the
namespace events — the [example README's troubleshooting
section](../../example/openshift/README.md#troubleshooting) covers SCC
violations, route admission, and database connectivity.
