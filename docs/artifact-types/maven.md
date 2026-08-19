# Maven

Harbor can host your own Maven artifacts natively and/or proxy-cache Maven
Central (or any Maven-compatible repository, e.g. a corporate Nexus/Artifactory
mirror). Both live under the same `/maven/<project>/` URL prefix in the same
project.

## Native hosting

### 1. Point Maven at your project

`~/.m2/settings.xml`:

```xml
<settings>
  <servers>
    <server>
      <id>harbor</id>
      <username>YOUR_USERNAME</username>
      <password>YOUR_PASSWORD_OR_ROBOT_SECRET</password>
    </server>
  </servers>
</settings>
```

`pom.xml`:

```xml
<distributionManagement>
  <repository>
    <id>harbor</id>
    <url>https://<harbor-host>/maven/<project></url>
  </repository>
</distributionManagement>
<repositories>
  <repository>
    <id>harbor</id>
    <url>https://<harbor-host>/maven/<project></url>
  </repository>
</repositories>
```

### 2. Deploy and resolve

```bash
mvn deploy                                    # PUT, requires push permission
mvn dependency:get -Dartifact=com.acme:widget:1.0.0   # GET, requires pull permission
```

There is no publish-body protocol the way npm has one: the Maven client PUTs
each file individually (jar, pom, sources/javadoc classifiers, checksums)
and computes `maven-metadata.xml` itself. Harbor's behavior:

- Real files (jar/pom/classifiers) are stored exactly as uploaded.
- `.sha1`/`.md5`/`.sha256`/`.sha512` sidecars you upload are **accepted and
  discarded** — Harbor always derives checksums from the exact bytes it
  serves, so a checksum never drifts from the artifact.
- `maven-metadata.xml` you upload is likewise accepted and discarded — Harbor
  **always synthesizes it from what's actually stored**, both at the
  GroupId:ArtifactId level (`<versions>`, `<latest>`, `<release>`) and, for
  `-SNAPSHOT` versions, at the GAV level (`<snapshotVersions>` with the
  timestamp/build-number scheme Maven clients expect).

### SNAPSHOT versions

SNAPSHOT deploys work the normal Maven way — Harbor tracks each timestamped
build (`1.0-SNAPSHOT` → `1.0-20260101.120000-3`) and synthesizes the GAV
`maven-metadata.xml` so clients resolve the latest build automatically.

### What you get in the Portal

Native Maven artifacts get a **Files** tab listing every GAV member file
(classifier, extension, checksum, release vs. snapshot), a **Versions** tab,
and a **Usage** tab with the ready-to-copy
`mvn dependency:get -Dartifact=...` snippet.

## Proxy cache (pull-through)

1. **Administration → Registries → New Endpoint** — Provider **Maven**, URL
   `https://repo1.maven.org/maven2` (or your upstream). Save.
2. **New Project** (or edit an existing one) → enable **Proxy Cache** →
   select the registry endpoint you just created.
3. Point Maven at the project the same way as native hosting (step 1 above).
   Any GAV your build resolves that isn't already stored natively is fetched
   from upstream, streamed back immediately, and cached in the background for
   next time.

`maven-metadata.xml` is **never cached as truth** even in a proxy-cache
project — every metadata request goes to upstream fresh (or is synthesized
from what's natively stored), so proxied projects always see the true
upstream version list rather than a stale snapshot. Only real files
(jar/pom/classifiers) are cached.

Proxy-cache projects are pull-only for the upstream repository — `mvn
deploy` into a proxy-cache project stores a **native** artifact that then
shadows the same GAV from upstream (see
[the overview](./README.md#how-native--proxy-interact-npm-and-maven) for the
exact precedence rule).

## Troubleshooting

| Symptom | Cause |
|---|---|
| `401 Unauthorized` on deploy/resolve | Check `<server>` credentials in `settings.xml` match the `<id>` referenced by `pom.xml`; confirm push/pull permission on the project. |
| `409 Conflict` on deploy | That exact GAV file already exists for a release version, or a SNAPSHOT publish raced another with the same timestamp — retry the SNAPSHOT deploy. |
| Old versions still show up after upstream removed them (proxy cache) | Expected — once cached, a file isn't re-validated against upstream. Only `maven-metadata.xml` (the version list) is always fresh. |
| `mvn dependency:get` 404s on something that exists upstream | Confirm the project has Proxy Cache enabled and the registry endpoint's URL is correct (no trailing content past the Maven repo root). |

## Source pointers

`src/server/registry/maven/{route,handler}.go` (protocol + metadata
synthesis + proxy fallback), `src/controller/artifact/processor/maven/maven.go`
(artifact-type recognition), `src/pkg/multiformat` (native storage model),
`docs/native-package-ux/` (design record).
