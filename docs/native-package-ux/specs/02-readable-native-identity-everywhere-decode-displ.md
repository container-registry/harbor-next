# Spec #2 — Readable native identity via storage-tree naming + per-ecosystem icons

**REVISED (supersedes the earlier presentation-decode draft).** Product decision: make package
names readable at the STORAGE layer, not via UI-only decode. This is what is implemented.

## Goal / acceptance criteria
1. **Readable stored repo names.** `naming.EncodeRepo` lays the native identity out as a readable
   tree, so the stored OCI repository name is human-meaningful **everywhere** (registry catalog,
   `/api/v2.0/.../repositories`, CLI, logs) — not just in a decoded UI:
   - Maven `groupId:artifactId` → groupId dots become path segments + artifactId as the final
     segment: `org.springframework.boot:spring-boot-starter-test` →
     `library/maven/org/springframework/boot/spring-boot-starter-test`.
   - npm scoped `@scope/name` → `library/npm/<scope>/<name>` (drop the illegal `@`); plain names →
     `library/npm/<name>`.
   - A segment already grammar-legal lowercase is emitted verbatim; otherwise it is escaped
     (uppercase/odd chars) so the result stays OCI-grammar-legal and injective.
   - Coordinates remain stored authoritatively in `multi_format_package.native_name`; the repo name is never
     decoded back.
2. **No leftover escaped names** for freshly pushed packages (no `mcom_x2e…` / `morg_x2e…`).
3. **Per-ecosystem icons.** npm and Maven artifacts render their own icon (not the generic/unknown
   icon) in the artifact list and detail. Backend wires `DigestOfIconNPM`/`DigestOfIconMaven`
   (`src/lib/icon/const.go`) = the real sha256 of `icons/npm.png` / `icons/maven.png`; `builtInIcons`
   (`src/controller/icon/controller.go`) maps those digests to the PNG files; `defaultIcons`
   (`src/controller/artifact/controller.go`) maps artifact type NPM/MAVEN → those digests. The portal
   icon flow is digest-driven (`artifact.icon` → icon service), so no frontend type→icon map is needed.

## Files
- `src/pkg/multiformat/naming/naming.go` — `EncodeRepo` tree layout + `nameSegments` + `componentOrEscape`.
- `src/pkg/multiformat/naming/naming_test.go` — tree + grammar + escape-fallback assertions.
- `src/lib/icon/const.go`, `src/controller/icon/controller.go`, `src/controller/artifact/controller.go` — icons.
- `icons/npm.png`, `icons/maven.png` — assets.
- Frontend: `artifact.ts` ArtifactType enum gains NPM/MAVEN (type label correct); icon renders via existing digest-driven flow.

## Verification (dev env SLOT=1)
- `go test ./pkg/multiformat/naming/...` passes (tree cases: spring-boot-starter-test, com.acme:widget2,
  lodash, @types/node; uppercase/odd → grammar-legal escaped).
- Push a maven GAV and an npm package; `GET /api/v2.0/projects/library/repositories` shows readable
  names like `library/maven/org/springframework/.../spring-core` and `library/npm/lodash` (NO `_x`).
- Real sha256 of `icons/npm.png` == `DigestOfIconNPM` (and maven). In the portal, npm/maven rows show
  their ecosystem icon and the type label reads NPM / MAVEN.
