# Deploying Harbor on OpenShift

## Configuration Steps

### Set the External URL
Replace the `externalURL` with the public URL provided by OpenShift:

```yaml
externalURL: "https://alex-container-regis-dev.apps.rm2.thpm.p1.openshiftapps.com"
```

### Configure Ingress
OpenShift requires specific annotations and host configurations for routing to work.

1. Add the `route.openshift.io/termination` annotation.
2. Set the `core` and `hosts` fields to the provided URL.

```yaml
ingress:
  annotations:
    route.openshift.io/termination: edge
  core: "alex-container-regis-dev.apps.rm2.thpm.p1.openshiftapps.com"
  hosts:
    - host: alex-container-regis-dev.apps.rm2.thpm.p1.openshiftapps.com
      paths:
        - path: /
          pathType: Prefix
```

### Manage Security Contexts
Because OpenShift automatically sets security contexts during deployment, you must initialize the main settings to empty objects. For the `valkey` section, set specific fields to `null`.

- Main configuration:
  ```yaml
  securityContext: {}
  podSecurityContext: {}
  ```

- Valkey configuration:
  ```yaml
  securityContext:
    readOnlyRootFilesystem: null
    runAsNonRoot: null
    runAsUser: null

  podSecurityContext:
    fsGroup: null
    runAsUser: null
    runAsGroup: null
  ```

### Set Storage Class
Configure the `storageClass` to `gp3` (the default on OpenShift) in every persistence section:

```yaml
persistence:
  enabled: true
  storageClass: "gp3"
```

## Database Deployment

To ensure a working Harbor deployment, install a compatible PostgreSQL database and reference it in your configuration values.

## Accessing Harbor

After deployment, Harbor will be available at the configured `externalURL`. If access fails, check the events in the OpenShift dashboard for troubleshooting.
