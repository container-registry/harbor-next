# Multi-Format Artifact Feature Spec

## Goal

Make Harbor a multi-ecosystem artifact registry without turning each package
manager into a separate product silo.

The feature should support package-native client protocols at the edge while
storing package content through Harbor's existing OCI-backed artifact, blob,
repository, tag, quota, scan, and Portal systems.

Initial ecosystems:

- npm
- Maven
- Homebrew bottles

Later ecosystems should be able to register into the same foundation without
rewriting Harbor core.

## Architecture Principles

Generic code is foundational only.

Shared code may define:

- package ecosystem registration
- generic OCI package storage primitives
- generic package media types
- generic config and layer descriptors
- shared registry/controller integration

Shared code must not define npm or Maven protocol semantics.

Ecosystem-specific code owns:

- route parsing
- request and response shapes
- auth header conventions beyond Harbor's security context
- package identity rules
- protocol metadata
- publish and resolution behavior
- compatibility quirks
- artifact additions that depend on ecosystem semantics

This means npm behavior belongs under `npm`, Maven behavior belongs under
`maven`, and Homebrew-specific behavior belongs under Homebrew or OCI image
classification code that is explicitly about Homebrew bottles.

## Package Registry Registration

Harbor should have one small central package registry surface.

Its job is only to register package ecosystems and let each ecosystem register
its own package route.

The central surface may know that ecosystems exist:

- npm
- Maven
- Homebrew

It should not know how npm packuments work, how Maven metadata is generated, or
how Homebrew bottle annotations are interpreted.

Package client routes should be explicit ecosystem routes:

- `/npm/<project>/...`
- `/maven/<project>/...`

Future package managers should follow the same pattern with their own top-level
ecosystem route. Harbor should not add native root-level package routes such as
`/<project>/...`, because those conflict with Harbor application, API, OCI, and
static routes and make ecosystem dispatch ambiguous.

## Package Proxy Projects

A proxy project is bound to exactly one upstream registry and therefore one
package ecosystem. For example, an npm proxy project serves `/npm/<project>/...`
and a Maven proxy project serves `/maven/<project>/...`; native clients must not
publish a different package type or OCI images into either project.

Proxy projects are read-only by default. At project creation, an administrator
may enable client publishing. When enabled, packages published by authorized
clients coexist with dependencies cached from the configured upstream registry,
so an install can serve locally stored content and proxy missing dependency
content through Harbor.

The upstream registry and client-publishing policy are creation-only project
identity. The Portal displays both values after creation but does not allow them
to be changed. The API likewise rejects attempts to change or delete the
publishing policy; changing either choice requires a new proxy project.

## Native Client URL Shape

Package URLs should feel native to the package manager, not like OCI registry
implementation details.

For Maven, repository managers commonly expose a repository URL like:

```xml
<distributionManagement>
  <snapshotRepository>
    <id>NAME</id>
    <url>https://maven.example.com/OWNER/REPOSITORY/</url>
  </snapshotRepository>
  <repository>
    <id>NAME</id>
    <url>https://maven.example.com/OWNER/REPOSITORY/</url>
  </repository>
</distributionManagement>
```

Harbor can expose the same Maven repository layout without requiring an
implementation-oriented `/repository/` prefix in the client-facing URL. A
project-scoped Maven URL should use the Maven ecosystem prefix:

```text
https://registry.goharbor.io/maven/<project>/
```

Example:

```text
https://registry.goharbor.io/maven/library/
```

With that base URL, Maven would publish and resolve paths such as:

```text
https://registry.goharbor.io/maven/library/com/acme/demo/1.0.0/demo-1.0.0.jar
https://registry.goharbor.io/maven/library/com/acme/demo/maven-metadata.xml
```

This is cleaner than:

```text
https://registry.goharbor.io/repository/library/
```

The public package client routes are not API routes. Use `/npm/...` and
`/maven/...`, not `/api/npm/...` or `/api/maven/...`.

## Generic Package Storage

The generic package store is a foundational storage adapter over Harbor's OCI
registry and controllers.

It may provide:

- repository name construction from project, ecosystem prefix, and package name
- config blob creation
- manifest creation
- blob push and pull helpers
- artifact ensure
- blob sync and associations
- quota integration
- tag attachment
- layer lookup by digest or annotation
- upsert support for manifests made from multiple package files

It should not provide:

- npm packument generation
- npm dist-tag semantics
- Maven path parsing
- Maven metadata generation
- Maven release and snapshot policy
- Homebrew bottle interpretation

Those are ecosystem-owned concerns.

## Generic Media Types

Generic package media types are intentional and preferred for common package
storage primitives.

The current generic primitives are:

- config: `application/vnd.harbor.package.config.v1+json`
- file layer: `application/vnd.harbor.package.file.v1`
- file-list layer: `application/vnd.harbor.package.files.v1+json`
- documentation layer: `application/vnd.harbor.package.doc.v1.raw`

Ecosystem identity should be carried by the artifact type and annotations.

Examples:

- Maven artifact type: `application/vnd.harbor.maven.package.v1`
- npm artifact type: `application/vnd.harbor.npm.package.v1`
- Maven file role annotation: `io.goharbor.maven.role`
- layer title/path annotation: `org.opencontainers.image.title`

Do not create one-off media types for every Maven file role such as POM,
metadata, checksum, signature, classifier, README, or license. Use generic file
or doc layers and role annotations.

## Streaming Requirement

Package uploads and downloads must be streaming.

Handlers must not buffer full package files or archives in memory before
pushing them into Harbor storage.

This is especially important for Maven because Maven repositories commonly hold
large JARs, WARs, ZIPs, native archives, Android artifacts, generated SDKs, and
vendor binaries.

Required direction:

- request bodies should stream into digesting and storage paths
- checksum calculation should happen while streaming or from stored blobs
- metadata extraction should read only the files needed, and only when safe
- download responses should stream from registry storage to the client
- size limits should protect the service, but limits are not a substitute for
  streaming

Any temporary buffering should be limited to small metadata payloads such as
POM XML, npm packument JSON, or generated file-list JSON.

## npm Scope

npm owns npm protocol behavior under the npm package implementation.

npm-specific concerns include:

- package name parsing, including scoped names
- packument responses
- abbreviated metadata responses
- tarball routes
- publish payload decoding
- `dist-tags`
- `whoami`
- npm auth token compatibility
- npm-specific semantic layers and additions

The generic package foundation should only provide storage primitives used by
the npm implementation.

## Maven Scope

Maven owns Maven protocol behavior under the Maven package implementation.

Maven-specific concerns include:

- Maven repository path parsing
- `GET`, `HEAD`, and `PUT`
- release and snapshot paths
- generated and uploaded `maven-metadata.xml`
- checksum sidecars
- signature sidecars
- POM parsing
- classifiers and attached artifacts
- release overwrite policy
- snapshot overwrite policy
- plugin metadata
- hosted, proxy, and group repository behavior

Maven files should be stored using generic package file/doc layers with
Maven-specific role annotations.

## Homebrew Scope

Homebrew bottles are already OCI artifacts.

The Homebrew work should focus on:

- recognizing Homebrew bottle image indexes
- presenting them as Homebrew artifacts in Harbor UI/API
- validating that `brew` can install bottles served from Harbor

Homebrew should not force a new storage path unless a Homebrew-native protocol
requires it later.

## Open Decisions

- Whether generic package storage should stay under `src/server/registry/pkgstore`
  or move to a more explicitly foundational package namespace.
- How to expose package ecosystem registration to future plugins or modules.
- How to reconcile partial writes after storage succeeds but metadata or Harbor
  controller updates fail.
- How to make streaming upload paths fit existing quota and blob association
  flows cleanly.

## Non-Goals For The Generic Layer

- Reimplementing each package manager in shared code.
- Defining per-ecosystem metadata schemas in shared code.
- Buffering complete package payloads in memory.
- Creating package-manager-specific database schemas before the generic storage
  model proves insufficient.
- Treating Homebrew bottles as a separate non-OCI storage model.
