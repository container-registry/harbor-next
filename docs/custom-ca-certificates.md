# Custom CA Certificates for Internal TLS

## Table of Contents
- [Overview](#overview)
- [Which Services This Applies To](#which-services-this-applies-to)
- [Mounting a Custom CA Certificate](#mounting-a-custom-ca-certificate)
- [Overriding the Mount Directory](#overriding-the-mount-directory)
- [The `harbor-registry` Exception](#the-harbor-registry-exception)
- [Troubleshooting](#troubleshooting)

## Overview

Harbor's Helm chart has long supported mounting an operator-supplied CA
certificate bundle into service containers (via `caBundleSecretName`), at the
conventional path `/harbor_cust_cert`. `core`, `jobservice`, `registryctl`,
and `exporter` are built `FROM scratch` with no shell and no
`update-ca-certificates`, so any certificate mounted there is merged into
those services' trust store automatically at startup, before any TLS
connection is attempted.

This matters whenever one of these services makes an outbound TLS connection
to an endpoint whose certificate isn't signed by a public CA — most notably,
`core` calling back into the registry's token realm (`.../service/token`)
when internal TLS is enabled with a private/internal CA.

## Which Services This Applies To

- `core`
- `jobservice`
- `registryctl`
- `exporter`

## Mounting a Custom CA Certificate

Mount one or more `.crt`/`.pem` files into `/harbor_cust_cert` (any other
file extension or subdirectory is ignored). At startup, each of the services
above merges every certificate found there into its baked-in system CA
bundle, and points its outbound TLS stack at the merged result. If the
directory doesn't exist or is empty — the common case for deployments that
don't use a private CA — this is a no-op.

## Overriding the Mount Directory

Set `CUSTOM_CA_CERT_DIR` on any of the four services above to mount the
custom CA bundle somewhere other than `/harbor_cust_cert`.

## The `harbor-registry` Exception

`harbor-registry` is not part of this mechanism. Unlike the other four
services, its running binary is unmodified upstream `distribution` source —
this repo doesn't own that code, so it can't call into the same
CA-merging logic. Go's standard library (`crypto/x509`) already reads the
`SSL_CERT_FILE` environment variable automatically with no application code
involvement, so if `harbor-registry` itself needs to trust a private CA
(for example, connecting to an S3-compatible storage backend over TLS),
set `SSL_CERT_FILE` directly on that container instead, pointing at the
mounted certificate file.

## Troubleshooting

If a service still logs `x509: certificate signed by unknown authority`
after mounting a custom CA certificate:
- Confirm the mounted file has a `.crt` or `.pem` extension.
- Confirm the mount directory matches `CUSTOM_CA_CERT_DIR` if set, or
  `/harbor_cust_cert` otherwise.
- Check the affected service's startup logs for
  `loaded N custom CA certificate(s) from ... into ...` — its absence means
  the directory was empty or unreadable.
- If the failing connection originates from `harbor-registry` itself, see
  [The `harbor-registry` Exception](#the-harbor-registry-exception) above.
