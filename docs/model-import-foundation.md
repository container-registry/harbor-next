# Model Import Foundation

Shared backend for importing AI models (Hugging Face) into Harbor. It ports the `huggingface`
registry adapter of goharbor/harbor#24008 onto harbor-next and adds four generic capabilities
that a UI can build on: resolve/preview, file filter, revision tag, source-only. This file is
the contract for the two UI prototypes (Option A: separate Model Sync page/API, Option B:
extend the replication/registry UI). Neither public API for Resolve is designed here.

Bookmark `model-import/foundation`, on `main@harbor-next`:

| Commit | Content |
|---|---|
| `feat(replication): add Hugging Face registry adapter` | PR #24008 commit 1, plus `huggingface` in the proxy-cache and adapter allowlists of `devenv/`, `deploy/compose/` and the chart |
| `fix(huggingface): persist token credentials, ...` | PR #24008 commit 2 |
| `feat(replication): Add File Filter for Source Artifacts` | `file` filter, `SourceOptions`, `SourceConfigurer` |
| `feat(replication): Add Artifact Resolver for Copy Previews` | `ArtifactResolver`, HF implementation |
| `feat(replication): Add Revision Tag Option to Replication Policies` | `revision_tag` policy flag, DB column |
| `feat(replication): Declare Source-Only Registries in Registry Info` | `RegistryInfo.SourceOnly`, `AdapterPattern.SourceOnly`, generic validation |

## Contract

### Go: `src/pkg/reg/model`

```go
const FilterTypeFile = "file"

type FileFilter struct {
    Include []string `json:"include,omitempty"`
    Exclude []string `json:"exclude,omitempty"`
}
func ParseFileFilter(value any) (*FileFilter, error)   // object or string form, nil when empty
func FileFilterOf(filters []*Filter) (*FileFilter, error)
func (f *FileFilter) Match(path string) bool            // nil selects everything
func (f *FileFilter) Validate() error
func (f *FileFilter) IsEmpty() bool
func (f *FileFilter) Key() string                       // order/duplicate independent, "" when empty

type SourceOptions struct {
    FileFilter  *FileFilter `json:"file_filter,omitempty"`
    RevisionTag bool        `json:"revision_tag,omitempty"`
}
func (o *SourceOptions) IsEmpty() bool

const RevisionTagPrefix = "sha-"
func RevisionTag(revision string) string                // "sha-" + first 12 hex, "" if not hex
func IsRevisionTag(tag string) bool

type ResolveResult struct {
    Repository   string          // e.g. "huggingfacetb/smollm2-135m", before dest namespace rewrite
    SourceID     string          // e.g. "HuggingFaceTB/SmolLM2-135M"
    Reference    string          // as asked, "main" when empty
    Revision     string          // 40-hex commit
    RevisionTag  string          // "sha-<12>"
    SourceURL    string
    Tags         []string        // moving tags (sanitized refs) pointing at Revision
    Digest       string          // manifest digest a copy with this filter produces; "" if nothing matches
    Licenses     []string
    Task         string          // Hub pipeline_tag
    LastModified *time.Time
    TotalFiles, MatchedFiles int
    TotalSize,  MatchedSize  int64
    Files        []*ResolvedFile // sorted by path, .gitattributes omitted
}
type ResolvedFile struct {
    Path    string
    Size    int64
    Class   string // FileClassWeight | FileClassConfig | FileClassCode | FileClassDoc
    SHA256  string // LFS files always; non-LFS files when matched (hashed for the digest)
    Matched bool
}

// RegistryInfo gains:
SourceOnly           bool `json:"source_only,omitempty"`
SupportedRevisionTag bool `json:"supported_revision_tag,omitempty"`
// AdapterPattern gains (type level, must agree with RegistryInfo.SourceOnly):
SourceOnly bool `json:"source_only,omitempty"`
// Resource gains:
SourceOptions *SourceOptions `json:"source_options,omitempty"`
```

A source declares file filter support by listing `{Type: "file", Style: "input"}` in
`RegistryInfo.SupportedResourceFilters`.

### Go: `src/pkg/reg/adapter`

```go
// Optional. Call right after creating the adapter, before any other call.
type SourceConfigurer interface {
    ConfigureSource(opts *model.SourceOptions) error
}
// No-op for empty options; error when non-empty and the adapter is not a SourceConfigurer.
func ConfigureSource(a any, opts *model.SourceOptions) error

// Optional. Preview of a copy, no data copied.
type ArtifactResolver interface {
    Resolve(repository, reference string, filter *model.FileFilter) (*model.ResolveResult, error)
}
```

Calling Resolve from new code (Option A endpoint or Option B preview):

```go
registry, err := regctl.Ctl.Get(ctx, registryID)          // controller/registry
a, err := reg.Mgr.CreateAdapter(ctx, registry)             // pkg/reg
resolver, ok := a.(adapter.ArtifactResolver)
if !ok {
    return errors.BadRequestError(nil).WithMessage("the registry cannot preview a copy")
}
result, err := resolver.Resolve("HuggingFaceTB/SmolLM2-135M", "main", filter)
```

HF Resolve errors: `BAD_REQUEST` for a malformed model ID or filter, `NOT_FOUND` for an unknown
model, tag or commit, `CONFLICT` for an ambiguous tag. Cost: the refs call (cached; also made
for a 40-hex reference, to list the tags pointing at it), the revision listing, and one raw download per matched non-LFS file whose
hash is not cached yet. Results are cached in the same `lib/cache` keys the copy uses, so a
preview warms the following copy or proxy pull.

### Go: `src/controller/replication/model`

```go
type Policy struct {
    ...
    RevisionTag bool `json:"revision_tag"`
}
func (p *Policy) SourceOptions() (*model.SourceOptions, error) // file filter from Filters + RevisionTag
```

### REST (`api/v2.0/swagger.yaml`)

- `ReplicationPolicy.revision_tag` (boolean, nullable). On PUT, an omitted value is `false`,
  like `copy_by_chunk`.
- `ReplicationFilter` with `type: "file"`. `value` is either
  `{"include": ["*.json"], "exclude": ["onnx/**"]}` or a string such as `"*.json, !onnx/**"`
  (separators: comma, space, newline; leading `!` excludes). The string form is what the
  existing replication form's generic text input sends, so the current portal already shows a
  FILE input for Hugging Face sources and it works. At most one file filter per policy.
- `RegistryInfo.source_only`, `RegistryInfo.supported_revision_tag` (omitted when false), and
  `file` in `supported_resource_filters`.

### Database

`replication_policy.revision_tag BOOLEAN NOT NULL DEFAULT FALSE`, added idempotently in
`make/migrations/postgresql/harbor_next.sql` (no numbered migration).

## How the options travel

```
policy (filters[type=file], revision_tag)
  └─ Policy.SourceOptions()                        controller/replication/model
       ├─ copy flow: adapter.ConfigureSource(srcAdapter, opts)   flow/copy.go
       │    └─ FetchArtifacts: HF appends "sha-<12>" to each artifact's tags (after the tag filter)
       └─ srcResource.SourceOptions = opts  ──JSON job param "src_resource"──►  jobservice
            └─ transfer.initialize: adapter.ConfigureSource(srcReg, src.SourceOptions)
                 └─ PullManifest/PullBlob: HF builds the manifest with the file filter
```

The destination resource never carries options. Proxy cache never configures options, so it
always serves the full repository.

## Semantics

**File filter.** A path is selected when it matches any include pattern (or there is none) and
no exclude pattern. Patterns are doublestar. A pattern without `/` matches the base name at
any depth (`*.json` selects `onnx/config.json`); a pattern with `/` matches the full path
(`onnx/**`). `.gitattributes` is never packed.

**Determinism.** The manifest digest is a function of the commit and the set of selected files
only: the same (commit, filter) always gives the same digest, a filter that drops files gives a
different digest, and two filters that select the same files share a digest (`**` gives the
unfiltered digest). The filter is deliberately not written into the manifest or config, so
identical content dedupes. A filter that selects nothing fails the copy with `BAD_REQUEST`;
Resolve returns `matched_files: 0` and no digest instead.

**Revision tag.** `sha-` plus the first 12 hex of the commit, added next to the moving tags a
copy gets (it follows the tags the tag filter kept and is not matched by the tag filter itself).
A source resolves a revision tag back to the ref head it names; a commit no ref points at any
more cannot be looked up by revision tag (the Hub has no short-sha lookup), which only matters
for proxy pulls by revision tag.

**Validation** (`controller/replication/policy.go`):

- file filter syntax: `pkg/reg/model.Filter.Validate`; more than one file filter: rejected.
- non-empty file filter: the source's registry info must list `file`.
- `revision_tag`: the source's registry info must declare `supported_revision_tag`.
- any source option on a policy without a remote source: rejected.
- destination with `source_only`: rejected. Replaces the former Hugging Face type check. It
  reads the factory's `AdapterPattern`, so no destination adapter is created and no remote
  registry is contacted; a destination of an unknown type is accepted as before and fails at
  run time with the real cause.

Source capabilities are read with the adapter factory's `Create` + `Info`, only when the policy
has source options; for the HF adapter that is offline.

## Verification

Unit tests (all pass, run from `src/` after `task build:gen-apis`):

```
go test ./pkg/reg/... ./controller/replication/... ./controller/registry/ ./server/v2.0/handler/
golangci-lint run ./pkg/reg/model/... ./pkg/reg/adapter/ ./pkg/reg/adapter/huggingface/... \
  ./controller/replication/... ./server/v2.0/handler/ ./pkg/replication/...   # 0 issues
```

New tests: `pkg/reg/model/source_test.go`, `pkg/reg/adapter/source_test.go`,
`pkg/reg/adapter/huggingface/{filter,resolve,revision_tag}_test.go`,
`controller/replication/policy_source_test.go`, `controller/replication/flow/copy_source_test.go`,
`controller/registry/process_test.go`, additions in `controller/replication/model/model_test.go`.
The filter determinism test also runs the real transfer with the options JSON-roundtripped as
job parameters, and again after clearing the jobservice process cache.

End to end on the SLOT=1 dev stack, 2026-11-04, model `HuggingFaceTB/SmolLM2-135M`, `main` =
`93efa2f097d58c2a74874c7e644dbc9b0cee75a2`. `H=http://localhost:8180/api/v2.0`, `A=admin:Harbor12345`.

Registry and capabilities:

```
curl -u $A -X POST $H/registries -H 'Content-Type: application/json' \
  -d '{"name":"hf","type":"huggingface","url":"https://huggingface.co","insecure":false}'   # 201, id 1, healthy
curl -u $A $H/registries/1/info
{"source_only":true,"supported_copy_by_chunk":true,"supported_resource_filters":[{"style":"input","type":"name",...},
 {"style":"input","type":"tag",...},{"style":"input","type":"file",...},{"style":"radio","type":"resource","values":["artifact"]}],
 "supported_revision_tag":true,"supported_triggers":["manual","scheduled"],"type":"huggingface"}
```

Policies (projects `hf-repl`, `hf-json` created first):

```
curl -u $A -X POST $H/replication/policies -H 'Content-Type: application/json' -d '{"name":"smollm-full",
  "src_registry":{"id":1},"dest_namespace":"hf-repl","dest_namespace_replace_count":1,
  "filters":[{"type":"name","value":"huggingfacetb/smollm2-135m"},{"type":"tag","value":"main"}],
  "trigger":{"type":"manual"},"enabled":true,"override":true}'                                   # 201, id 1
curl -u $A -X POST $H/replication/policies -H 'Content-Type: application/json' -d '{"name":"smollm-json",
  "src_registry":{"id":1},"dest_namespace":"hf-json","dest_namespace_replace_count":1,
  "filters":[{"type":"name","value":"huggingfacetb/smollm2-135m"},{"type":"tag","value":"main"},{"type":"file","value":"*.json"}],
  "trigger":{"type":"manual"},"enabled":true,"override":true,"revision_tag":true}'               # 201, id 2
curl -u $A -X POST $H/replication/executions -H 'Content-Type: application/json' -d '{"policy_id":N}'
```

(i) Full copy, execution 3 `Succeed 1/1`:

```
GET $H/projects/hf-repl/repositories/smollm2-135m/artifacts?with_tag=true
sha256:2b6f786b1ea21fb04c8466d412b19b00aba263c23d094170886b623f432508e3 CNAI
  application/vnd.cncf.model.manifest.v1+json ['main'] 272447115
```

Same digest as recorded in PR #24008, so the port and the filter refactor keep unfiltered output
byte-identical.

(ii)+(iii) File filter `*.json` with revision tag, execution 4 `Succeed 1/1`:

```
task log: source options: {"file_filter":{"include":["*.json"]},"revision_tag":true}
          copying huggingfacetb/smollm2-135m:[main,sha-93efa2f097d5] to hf-json/smollm2-135m:[main,sha-93efa2f097d5]
GET $H/projects/hf-json/repositories/smollm2-135m/artifacts?with_tag=true
sha256:2dc8fb68cff37ff1f1ae1bcef28039072cb9a012d4e7c7c817cee9d3d1b3b18e CNAI ['sha-93efa2f097d5', 'main'] 2912950
GET /v2/hf-json/smollm2-135m/manifests/sha-93efa2f097d5 (Accept: OCI manifest), layers:
application/vnd.cncf.model.weight.config.v1.raw 704 config.json
application/vnd.cncf.model.weight.config.v1.raw 111 generation_config.json
application/vnd.cncf.model.weight.config.v1.raw 831 special_tokens_map.json
application/vnd.cncf.model.weight.config.v1.raw 2104556 tokenizer.json
application/vnd.cncf.model.weight.config.v1.raw 3658 tokenizer_config.json
application/vnd.cncf.model.weight.config.v1.raw 800662 vocab.json
```

Reruns: execution 5, and execution 6 after `docker restart harbor-1-jobservice-1` (process cache
lost), both `Succeed` with `the artifact hf-json/smollm2-135m:main already exists on the
destination registry, skip` and the same for `sha-93efa2f097d5`; the digest stays `2dc8fb68…`.

Validation through the API, all `400`:

```
dest_registry = HF          -> the huggingface registry hf can only be a replication source
file filter "[a"            -> invalid file filter: malformed pattern "[a"
revision_tag, local source  -> the source registry Local does not support the revision tag
```

Proxy cache unchanged: project `hf-proxy` on registry 1,
`GET /v2/hf-proxy/huggingfacetb/smollm2-135m/manifests/main` returns 200 and the body hashes
to `2b6f786b…`.

(iv) Resolve against the real Hub, via the live test:

```
HF_FILTER='*.json' go test -count=1 -tags hubintegration -run TestResolveLive -v ./pkg/reg/adapter/huggingface/
  "revision": "93efa2f097d58c2a74874c7e644dbc9b0cee75a2", "revision_tag": "sha-93efa2f097d5",
  "digest": "sha256:2dc8fb68cff37ff1f1ae1bcef28039072cb9a012d4e7c7c817cee9d3d1b3b18e",
  "task": "text-generation", "total_files": 9, "matched_files": 6, "matched_size": 2910522
```

Without `HF_FILTER` it returns `digest` `sha256:2b6f786b…`, license `apache-2.0`, tags
`["main"]` and the 9 files with class (`doc`, `config` x7, `weight`) and sha256. Both preview
digests equal the digests of the actual copies above. `HF_MODEL` and `HF_REF` select another
model or reference.

## Dev stack

Running from this workspace on SLOT=1 (started with `task dev:infra:up SLOT=1` and
`task dev:backend:up SLOT=1 SKIP_TRIVY=true`; `task dev:up` was not used because it tears the
stack down when its foreground portal process exits). Core and jobservice hot-reload from
`src/` with air.

| | |
|---|---|
| Core API | http://localhost:8180/api/v2.0, `admin` / `Harbor12345` |
| Postgres | localhost:5532, `postgres` / `root123`, db `registry` |
| Redis | localhost:6479 |
| Registry | localhost:50100 |
| Compose project | `harbor-1` |
| State | registry `hf` (id 1), projects `hf-repl`, `hf-json`, `hf-proxy`, policies 1 and 2 |

Portal: `task dev:frontend:native SLOT=1` (ng serve on port 4300, proxying to core 8180). This harbor-next base has no
`CSRF_PLAINTEXT_HTTP` support in core (`src/server/middleware/csrf/`) and `devenv/` does not
forward it, so a browser login over plain HTTP may need that added.

Stop: `task dev:backend:down SLOT=1 && task dev:infra:down SLOT=1` from this workspace (named
volumes survive; `task dev:infra:remove SLOT=1` deletes them).

## Known limitations

- The revision tag is added only to commits that a moving tag selects; replication still copies
  ref heads, not every commit. Proxy pulls by revision tag work only while a ref points at it.
- The filter is not recorded in the artifact; which filter produced a copy is known from the
  policy only.
- The existing portal shows the FILE input for HF sources (generic text input, string syntax),
  but has no revision tag checkbox and does not hide source-only registries from the
  destination picker. That is UI work for Option B.
- Resolve has no REST endpoint; Option A/B decide the API shape.
- Observed, not investigated: the proxied `GET /v2/hf-proxy/.../manifests/main` answered with an
  empty `Docker-Content-Digest` header on the first pull.
