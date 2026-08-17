# Maven Compatibility Fixtures

These fixtures build a containerized Maven client and generate example Maven
projects inside the container. They are intended for testing whether Harbor can
accept Maven uploads and serve them back to Maven clients without installing
Java or Maven on the host.

## Build A Client Image

Build the default modern matrix image:

```bash
docker build \
  -f tests/maven/Containerfile \
  --build-arg JAVA_VERSION=21 \
  --build-arg MAVEN_VERSION=3.9.9 \
  -t harbor-maven-compat:jdk21-mvn3.9.9 \
  tests/maven
```

Build older clients by changing the build args:

```bash
docker build -f tests/maven/Containerfile --build-arg JAVA_VERSION=8  --build-arg MAVEN_VERSION=3.6.3 -t harbor-maven-compat:jdk8-mvn3.6.3  tests/maven
docker build -f tests/maven/Containerfile --build-arg JAVA_VERSION=11 --build-arg MAVEN_VERSION=3.8.8 -t harbor-maven-compat:jdk11-mvn3.8.8 tests/maven
docker build -f tests/maven/Containerfile --build-arg JAVA_VERSION=17 --build-arg MAVEN_VERSION=3.9.9 -t harbor-maven-compat:jdk17-mvn3.9.9 tests/maven
docker build -f tests/maven/Containerfile --build-arg JAVA_VERSION=21 --build-arg MAVEN_VERSION=3.9.9 -t harbor-maven-compat:jdk21-mvn3.9.9 tests/maven
```

The helper script `build-matrix.sh` runs the four builds above.

## Push And Pull The Client Image Through Harbor

Assuming Harbor is exposed on the host at `localhost:8080` and the Harbor
project is `library`:

```bash
docker login localhost:8080 -u admin -p Harbor12345

docker tag harbor-maven-compat:jdk21-mvn3.9.9 \
  localhost:8080/library/harbor-maven-compat:jdk21-mvn3.9.9

docker push localhost:8080/library/harbor-maven-compat:jdk21-mvn3.9.9
docker pull localhost:8080/library/harbor-maven-compat:jdk21-mvn3.9.9
```

## Run Against Harbor Maven On The Host

On Linux, pass `--add-host=host.docker.internal:host-gateway` so the container
can reach Harbor running on the host. Docker Desktop already provides
`host.docker.internal`, but the flag is harmless there.

Use the Portal dev proxy when testing the same public URL shape as
`localhost:4200`:

```bash
docker run --rm \
  --add-host=host.docker.internal:host-gateway \
  -e HARBOR_MAVEN_URL=http://host.docker.internal:4200/maven/library/ \
  -e HARBOR_USERNAME=admin \
  -e HARBOR_PASSWORD=Harbor12345 \
  localhost:8080/library/harbor-maven-compat:jdk21-mvn3.9.9
```

Direct Core URL:

```bash
HARBOR_MAVEN_URL=http://host.docker.internal:8080/maven/library/
```

## Open A Shell Inside The Image

```bash
docker run --rm -it \
  --add-host=host.docker.internal:host-gateway \
  -e HARBOR_MAVEN_URL=http://host.docker.internal:4200/maven/library/ \
  -e HARBOR_USERNAME=admin \
  -e HARBOR_PASSWORD=Harbor12345 \
  --entrypoint bash \
  localhost:8080/library/harbor-maven-compat:jdk21-mvn3.9.9
```

Inside the container:

```bash
run-maven-compat all
run-maven-compat basic
run-maven-compat multi
run-maven-compat resolve
```

## Suites

The entrypoint accepts one suite name. Default is `all`.

- `basic`: tiny JAR, medium JAR, configurable large JAR, snapshot, POM-only parent
- `classifiers`: sources, javadocs, assembly ZIP, platform classifier ZIP
- `web`: WAR packaging
- `multi`: multi-module reactor with parent POM, BOM, cross-module dependencies, external dependencies, and WAR module
- `plugin`: `maven-plugin` packaging plus plugin resolution/execution from Harbor
- `archetype`: `maven-archetype` packaging artifact resolution
- `manual`: `deploy:deploy-file` vendor ZIP with classifier and supplied POM
- `resolve`: clean-local-repository pullback checks from Harbor
- `all`: all suites above

## Useful Environment Variables

- `HARBOR_MAVEN_URL`: Maven repository URL, for example `http://host.docker.internal:8080/maven/library/`
- `HARBOR_USERNAME`: Harbor username
- `HARBOR_PASSWORD`: Harbor password
- `HARBOR_REPOSITORY_ID`: Maven server id, default `harbor`
- `HARBOR_MAVEN_GROUP`: generated groupId, default `com.harbor.maven.compat`
- `MAVEN_FIXTURE_VERSION`: release version. By default each run uses a timestamped unique version.
- `MAVEN_SNAPSHOT_VERSION`: snapshot version. By default each run uses a timestamped unique snapshot.
- `LARGE_MB`: large fixture resource size in MiB, default `30`
- `MIRROR_ALL_TO_HARBOR=true`: writes a `mirrorOf=*` settings.xml. Use this only when Harbor proxy/group Maven support exists, because Maven plugins and external dependencies will otherwise be forced through Harbor too.

## What This Exercises

The full run deploys and resolves:

- release and snapshot metadata
- generated `.pom`, `.jar`, `.war`, `.zip`, checksum, and metadata files
- sources and javadocs classifiers
- platform/native classifier artifacts
- POM-only packages, parent POMs, and BOM/import POMs
- multi-module reactor deploys with cross-module dependencies
- external dependencies from Maven Central during build
- Maven plugin packaging and plugin descriptor resolution
- Maven archetype packaging
- manual third-party binary publication with `deploy:deploy-file`

This is intentionally broader than the current minimal Maven route. Some suites
may expose missing Harbor behavior; keep those failures as compatibility gaps.
