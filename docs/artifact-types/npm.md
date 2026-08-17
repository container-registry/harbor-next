# npm

Harbor can host your own npm packages natively and/or proxy-cache
registry.npmjs.org (or any npm-compatible upstream, e.g. a private npm
registry). Both live under the same `/npm/<project>/` URL prefix in the same
project.

## Native hosting

### 1. Point npm at your project

```bash
npm config set registry https://<harbor-host>/npm/<project>/
```

Or per-project in `.npmrc`:

```ini
registry=https://<harbor-host>/npm/<project>/
```

Scoped packages (`@myorg/pkg`) work as-is; Harbor doesn't require the scope
to match the project name.

### 2. Authenticate

Use HTTP Basic auth with a Harbor user or robot account:

```ini
# .npmrc
//<harbor-host>/npm/<project>/:_auth=<base64 of "username:password">
//<harbor-host>/npm/<project>/:always-auth=true
```

`npm login` against Harbor will authenticate and confirm your identity, but
the token it stores is **not yet honored on later requests** — keep using
`_auth`/Basic credentials in `.npmrc` for `publish`/`install`. This is a
known gap, not a design choice; see the source pointers below if you want to
pick it up.

### 3. Publish and install

```bash
npm publish                 # PUT, requires push permission on the project
npm install harbor-multi-format-demo   # GET, requires pull permission (or a public project)
```

Each `npm publish` uploads exactly one version (standard npm CLI behavior).
Versions are **immutable** once published — re-publishing the same
name+version is rejected with a 409, matching real npm registry semantics.

### dist-tags

Dist-tags (`latest`, `next`, custom channels) are tracked independently of
the version data and can be repointed or removed without republishing:

```bash
npm dist-tag ls harbor-multi-format-demo
npm dist-tag add harbor-multi-format-demo@1.2.0 next
npm dist-tag rm harbor-multi-format-demo next
```

If you never set any dist-tag, `latest` defaults to the highest published
semver version.

### What you get in the Portal

Native npm artifacts get their own artifact-type icon, a **Usage** tab with
the ready-to-copy `npm install` command, a **Versions** tab, and (for
packages with a README/packument) the standard additions surface. Search
matches the readable package name, not Harbor's internal storage path.

## Proxy cache (pull-through)

To have Harbor cache packages from an upstream npm registry instead of (or
alongside) hosting your own:

1. **Administration → Registries → New Endpoint** — choose **npmjs.org** for
   the editable `https://registry.npmjs.org` default, or **npm Registry** for
   another npm-compatible upstream. Save.
2. **New Project** (or edit an existing one) → enable **Proxy Cache** →
   select the registry endpoint you just created.
3. Point `npm` at the project the same way as native hosting (step 1 above).
   `npm install <anything on the upstream registry>` now flows through
   Harbor and gets cached; repeat installs are served from Harbor without
   another upstream round trip.

Proxy-cache projects are pull-only for the upstream ecosystem — publish into
a proxy-cache project and it's stored as a **native** package that then
shadows any upstream package of the same name/version (see
[the overview](./README.md#how-native--proxy-interact-npm-and-maven) for the
exact precedence rule).

## Troubleshooting

| Symptom | Cause |
|---|---|
| `401 Unauthorized` on publish/install | Check `_auth`/`always-auth` in `.npmrc`; confirm the account has push/pull on the project. |
| `409 Conflict` on publish | That exact name+version already exists — versions are immutable. Bump the version. |
| Installs never hit upstream | Confirm the project has Proxy Cache enabled and the registry endpoint is reachable; native storage is checked first, so a same-name/version native package silently wins. |
| `npm login` "works" but next request 401s | Expected today — use Basic auth (`_auth`) instead of the login-issued token. |

## Source pointers

`src/server/registry/npm/{route,handler}.go` (protocol + proxy fallback),
`src/controller/artifact/processor/npm/npm.go` (artifact-type recognition),
`src/pkg/multiformat` (native storage model), `docs/native-package-ux/` (design
record).
