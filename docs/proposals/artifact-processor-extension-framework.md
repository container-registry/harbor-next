# Proposal: Artifact Processor Extension Framework

## Summary

This proposal introduces an **Artifact Processor Extension Framework** for Harbor that enables
pluggable post-push image processing capabilities. The framework provides a generic, registry-based
architecture where processors can be registered, configured per-project, and triggered
automatically when artifacts are pushed. The initial processors target **SOCI index generation**
and **eStargz conversion** (via the existing acceleration-service), but the framework is designed
to support any artifact transformation or enrichment operation.

## Motivation

Container image lazy-loading technologies like SOCI (Seekable OCI) and eStargz significantly
improve container startup times by enabling on-demand layer fetching. Currently, Harbor has no
built-in mechanism to automatically generate these artifacts when images are pushed. The
[goharbor/acceleration-service](https://github.com/goharbor/acceleration-service) exists as an
external webhook-driven service, but it operates outside Harbor's management plane with no
visibility into processing status, no per-project configuration, and no unified API.

Beyond image acceleration, there are many potential post-push processing needs:
- SBOM generation
- Signature verification/re-signing
- Format conversion (OCI <-> Docker)
- Custom metadata extraction
- Compliance scanning triggers

A generic framework allows Harbor to support all these use cases through a single extensible
architecture.

## Design Goals

1. **Extensibility**: New processors can be added by implementing a single interface and
   self-registering via `init()`, following Harbor's established processor registry pattern
2. **Per-project configuration**: Processors are enabled/disabled per project with processor-
   specific settings
3. **Async processing**: All processing runs asynchronously through Harbor's existing task/
   execution framework
4. **Status visibility**: Processing status is tracked and queryable per artifact
5. **Dual execution model**: Support both internal processing (job service) and external service
   delegation (HTTP callbacks to services like acceleration-service)
6. **Minimal core changes**: Integrate via Harbor's existing event system with a single new
   event handler subscription

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Harbor Core                              │
│                                                                 │
│  ┌──────────┐    TopicPushArtifact    ┌──────────────────────┐  │
│  │ Registry │ ─── event published ──> │ ArtifactProcessor    │  │
│  │ (push)   │                         │ EventHandler         │  │
│  └──────────┘                         └─────────┬────────────┘  │
│                                                 │               │
│                                    ┌────────────▼────────────┐  │
│                                    │ Processor Controller    │  │
│                                    │                         │  │
│                                    │ - List project policies │  │
│                                    │ - Match artifact type   │  │
│                                    │ - Create executions     │  │
│                                    └────────────┬────────────┘  │
│                                                 │               │
│                              ┌──────────────────▼─────────┐    │
│                              │   Processor Registry        │    │
│                              │                             │    │
│                              │  ┌─────────┐ ┌───────────┐ │    │
│                              │  │  SOCI   │ │  eStargz  │ │    │
│                              │  │Processor│ │ Processor │ │    │
│                              │  └────┬────┘ └─────┬─────┘ │    │
│                              │       │            │        │    │
│                              └───────┼────────────┼────────┘    │
│                                      │            │             │
│              ┌───────────────────────▼──┐  ┌──────▼──────────┐  │
│              │  Job Service (internal)  │  │ HTTP Delegator  │  │
│              │  - SOCI index generation │  │ (external svc)  │  │
│              └──────────────────────────┘  └────────┬────────┘  │
│                                                     │           │
└─────────────────────────────────────────────────────┼───────────┘
                                                      │
                                         ┌────────────▼─────────┐
                                         │ acceleration-service │
                                         │ (estargz/nydus/      │
                                         │  zstd:chunked)       │
                                         └──────────────────────┘
```

## Detailed Design

### 1. Processor Interface

The core abstraction follows Harbor's existing `processor.Processor` registry pattern
(see `src/controller/artifact/processor/processor.go`).

```go
// src/pkg/artifactprocessor/processor.go

package artifactprocessor

// Processor defines the interface for artifact post-push processors.
// Implementations handle specific transformation or enrichment operations
// on artifacts after they are pushed to the registry.
type Processor interface {
    // Info returns metadata about this processor.
    Info() *Info

    // Process initiates processing for the given artifact.
    // It receives the artifact details and processor-specific configuration.
    // Returns a vendor-specific execution ID for status tracking.
    Process(ctx context.Context, artifact *artifact.Artifact, config map[string]interface{}) error

    // ShouldProcess determines whether this processor should handle the given artifact.
    // This allows processors to filter by media type, annotations, labels, etc.
    ShouldProcess(ctx context.Context, artifact *artifact.Artifact) (bool, error)
}

// Info contains metadata about a processor.
type Info struct {
    // Type is the unique identifier (e.g., "soci", "estargz", "nydus")
    Type string

    // Name is the human-readable display name
    Name string

    // Description describes what the processor does
    Description string

    // Version is the processor version
    Version string

    // ExecutionMode indicates whether the processor runs internally
    // via Harbor's job service or delegates to an external service.
    ExecutionMode ExecutionMode

    // OutputType describes what the processor produces
    OutputType OutputType
}

// ExecutionMode specifies how the processor executes its work
type ExecutionMode string

const (
    // ExecutionModeInternal runs processing within Harbor's job service
    ExecutionModeInternal ExecutionMode = "internal"

    // ExecutionModeExternal delegates processing to an external service via HTTP
    ExecutionModeExternal ExecutionMode = "external"
)

// OutputType describes what a processor produces
type OutputType string

const (
    // OutputTypeCompanionArtifact creates a new artifact linked to the original
    // (e.g., SOCI index stored as OCI referrer)
    OutputTypeCompanionArtifact OutputType = "companion_artifact"

    // OutputTypeConvertedArtifact creates a converted copy of the original
    // (e.g., eStargz version tagged alongside the original)
    OutputTypeConvertedArtifact OutputType = "converted_artifact"

    // OutputTypeMetadata adds metadata/annotations to the existing artifact
    OutputTypeMetadata OutputType = "metadata"
)
```

### 2. Processor Registry

Following Harbor's proven pattern from `src/controller/artifact/processor/processor.go`:

```go
// src/pkg/artifactprocessor/registry.go

package artifactprocessor

var (
    registry = map[string]Processor{}
    mu       sync.RWMutex
)

// Register registers a processor for a given type.
// Processors self-register via init() in their packages.
func Register(p Processor) error {
    mu.Lock()
    defer mu.Unlock()

    info := p.Info()
    if _, exists := registry[info.Type]; exists {
        return fmt.Errorf("processor type %q already registered", info.Type)
    }
    registry[info.Type] = p
    log.Infof("artifact processor registered: %s (%s)", info.Name, info.Type)
    return nil
}

// Get returns the processor for the given type
func Get(processorType string) (Processor, bool) {
    mu.RLock()
    defer mu.RUnlock()
    p, ok := registry[processorType]
    return p, ok
}

// List returns all registered processors
func List() map[string]Processor {
    mu.RLock()
    defer mu.RUnlock()
    result := make(map[string]Processor, len(registry))
    for k, v := range registry {
        result[k] = v
    }
    return result
}
```

### 3. Per-Project Policy Model

Processor policies control which processors are active per project. This follows the same
pattern as P2P preheat policies (`src/pkg/p2p/preheat/models/policy/`).

```go
// src/pkg/artifactprocessor/model/policy.go

package model

// Policy defines a per-project artifact processor policy
type Policy struct {
    ID             int64                  `json:"id" orm:"column(id);pk;auto"`
    ProjectID      int64                  `json:"project_id"`
    ProcessorType  string                 `json:"processor_type"`
    Enabled        bool                   `json:"enabled"`
    Configuration  map[string]interface{} `json:"configuration"`
    // Filters control which artifacts this policy applies to.
    // Supports repository name patterns, tag patterns, and media types.
    Filters        []*Filter              `json:"filters"`
    CreationTime   time.Time              `json:"creation_time"`
    UpdateTime     time.Time              `json:"update_time"`
}

// Filter defines criteria for matching artifacts
type Filter struct {
    Type  string `json:"type"`   // "repository", "tag", "media_type", "label"
    Value string `json:"value"`  // glob pattern or exact match
}
```

#### Database Schema

```sql
-- New table: artifact_processor_policy
CREATE TABLE artifact_processor_policy (
    id            SERIAL PRIMARY KEY,
    project_id    INTEGER NOT NULL,
    processor_type VARCHAR(64) NOT NULL,
    enabled       BOOLEAN NOT NULL DEFAULT true,
    configuration JSONB,
    filters       JSONB,
    creation_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    update_time   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (project_id, processor_type)
);

CREATE INDEX idx_app_project ON artifact_processor_policy (project_id);
```

No additional execution/task tables are needed — the framework uses Harbor's existing
`execution` and `task` tables via the `task.ExecutionManager` and `task.Manager` interfaces,
keyed by a new vendor type (`ARTIFACT_PROCESSOR`).

### 4. Event Integration

A new event handler subscribes to `TopicPushArtifact`, following the exact pattern of
`src/controller/event/handler/p2p/preheat.go`:

```go
// src/controller/event/handler/artifactprocessor/handler.go

package artifactprocessor

// Handler handles push artifact events for artifact processing
type Handler struct{}

func (h *Handler) Name() string { return "ArtifactProcessor" }

func (h *Handler) IsStateful() bool { return false }

func (h *Handler) Handle(ctx context.Context, value any) error {
    e, ok := value.(*event.PushArtifactEvent)
    if !ok {
        return errors.New("unsupported event type")
    }

    // Only process image artifacts (not signatures, SBOMs, etc.)
    if e.Artifact.Type != image.ArtifactTypeImage {
        return nil
    }

    // Delegate to the processor controller which handles:
    // 1. Looking up active policies for the project
    // 2. Matching against filters
    // 3. Creating executions for each matched processor
    return processorctl.Ctl.ProcessArtifact(ctx, e.Artifact)
}
```

Registration in `src/controller/event/handler/init.go`:

```go
// artifact processor
_ = notifier.Subscribe(event.TopicPushArtifact, &artifactprocessor.Handler{})
```

### 5. Processor Controller

The controller orchestrates policy lookup, processor matching, and execution creation:

```go
// src/controller/artifactprocessor/controller.go

package artifactprocessor

// Controller manages artifact processor operations
type Controller interface {
    // ProcessArtifact triggers all matching processors for an artifact
    ProcessArtifact(ctx context.Context, art *artifact.Artifact) error

    // ListPolicies returns processor policies for a project
    ListPolicies(ctx context.Context, projectID int64) ([]*model.Policy, error)

    // CreatePolicy creates a new processor policy
    CreatePolicy(ctx context.Context, policy *model.Policy) (int64, error)

    // UpdatePolicy updates a processor policy
    UpdatePolicy(ctx context.Context, policy *model.Policy) error

    // DeletePolicy deletes a processor policy
    DeletePolicy(ctx context.Context, id int64) error

    // GetPolicy retrieves a single policy by ID
    GetPolicy(ctx context.Context, id int64) (*model.Policy, error)

    // ListProcessors returns all registered processor types
    ListProcessors(ctx context.Context) []*Info

    // GetExecution returns the processing execution for a given execution ID
    GetExecution(ctx context.Context, executionID int64) (*task.Execution, error)

    // ListExecutions lists executions for an artifact or project
    ListExecutions(ctx context.Context, query *q.Query) ([]*task.Execution, error)
}
```

### 6. Concrete Processor Implementations

#### 6.1 SOCI Processor (Internal Execution)

SOCI creates companion index artifacts that are stored alongside the original image in the
registry using OCI Reference Types (referrers API). No image conversion is needed.

```go
// src/pkg/artifactprocessor/soci/processor.go

package soci

func init() {
    _ = artifactprocessor.Register(&Processor{})
}

type Processor struct{}

func (p *Processor) Info() *artifactprocessor.Info {
    return &artifactprocessor.Info{
        Type:          "soci",
        Name:          "SOCI Index Generator",
        Description:   "Generates Seekable OCI (SOCI) indices for lazy image pulling",
        Version:       "1.0.0",
        ExecutionMode: artifactprocessor.ExecutionModeInternal,
        OutputType:    artifactprocessor.OutputTypeCompanionArtifact,
    }
}

func (p *Processor) ShouldProcess(ctx context.Context, art *artifact.Artifact) (bool, error) {
    // Only process OCI/Docker v2 images (not indices, not signatures, etc.)
    return art.ManifestMediaType == v1.MediaTypeImageManifest ||
        art.ManifestMediaType == schema2.MediaTypeManifest, nil
}

func (p *Processor) Process(ctx context.Context, art *artifact.Artifact,
    config map[string]interface{}) error {
    // 1. Pull image manifest layers
    // 2. Generate zTOC for each layer (table of contents for seekable access)
    // 3. Build SOCI index manifest referencing the zTOCs
    // 4. Push SOCI index to registry as OCI referrer of the original image
    // 5. Record as accessory (companion artifact)
    return nil // stub — actual implementation requires SOCI library
}
```

**SOCI Output Structure:**
```
Original Image Manifest (sha256:abc...)
  └── [referrer] SOCI Index Manifest (application/vnd.amazon.soci.index.v1+json)
        ├── zTOC for layer 1
        ├── zTOC for layer 2
        └── zTOC for layer N
```

#### 6.2 eStargz Processor (External Delegation)

eStargz conversion delegates to the acceleration-service, which handles the actual image
recompression. Harbor orchestrates the request and tracks status.

```go
// src/pkg/artifactprocessor/estargz/processor.go

package estargz

func init() {
    _ = artifactprocessor.Register(&Processor{})
}

type Processor struct{}

func (p *Processor) Info() *artifactprocessor.Info {
    return &artifactprocessor.Info{
        Type:          "estargz",
        Name:          "eStargz Converter",
        Description:   "Converts images to eStargz format via acceleration-service",
        Version:       "1.0.0",
        ExecutionMode: artifactprocessor.ExecutionModeExternal,
        OutputType:    artifactprocessor.OutputTypeConvertedArtifact,
    }
}

func (p *Processor) ShouldProcess(ctx context.Context, art *artifact.Artifact) (bool, error) {
    // Skip if already an eStargz image (check annotations)
    if art.Annotations != nil {
        if _, ok := art.Annotations["containerd.io/snapshot/stargz/toc.digest"]; ok {
            return false, nil
        }
    }
    return art.ManifestMediaType == v1.MediaTypeImageManifest ||
        art.ManifestMediaType == schema2.MediaTypeManifest, nil
}

func (p *Processor) Process(ctx context.Context, art *artifact.Artifact,
    config map[string]interface{}) error {
    // 1. Read acceleration-service endpoint from config
    // 2. Build webhook-compatible payload with artifact reference
    // 3. POST to <endpoint>/api/v1/conversions
    // 4. Track conversion status (poll or callback)
    // 5. Resulting image appears in registry with tag suffix (e.g., "-estargz")
    return nil // stub — actual implementation calls acceleration-service
}
```

### 7. API Design

New REST API endpoints under the project scope, following Harbor's existing API patterns:

```
# Processor type discovery (global)
GET    /api/v2.0/processors                              # List all registered processor types

# Per-project processor policies
GET    /api/v2.0/projects/{project_id}/processors/policies         # List policies
POST   /api/v2.0/projects/{project_id}/processors/policies         # Create policy
GET    /api/v2.0/projects/{project_id}/processors/policies/{id}    # Get policy
PUT    /api/v2.0/projects/{project_id}/processors/policies/{id}    # Update policy
DELETE /api/v2.0/projects/{project_id}/processors/policies/{id}    # Delete policy

# Processing executions (per project)
GET    /api/v2.0/projects/{project_id}/processors/executions                 # List executions
GET    /api/v2.0/projects/{project_id}/processors/executions/{exec_id}       # Get execution
GET    /api/v2.0/projects/{project_id}/processors/executions/{exec_id}/tasks # List tasks
GET    /api/v2.0/projects/{project_id}/processors/executions/{exec_id}/tasks/{task_id}/log # Get task log

# Manual trigger
POST   /api/v2.0/projects/{project_id}/processors/executions       # Manually trigger processing
```

### 8. File Structure

New files to create (following Harbor's package conventions):

```
src/
├── pkg/artifactprocessor/
│   ├── processor.go              # Processor interface and types
│   ├── registry.go               # Processor registry (Register/Get/List)
│   ├── model/
│   │   └── policy.go             # Policy model
│   ├── dao/
│   │   ├── dao.go                # DAO interface
│   │   └── dao_impl.go           # Database operations
│   ├── manager.go                # Policy manager (business logic)
│   ├── soci/
│   │   └── processor.go          # SOCI processor implementation
│   └── estargz/
│       └── processor.go          # eStargz processor implementation
│
├── controller/artifactprocessor/
│   └── controller.go             # Processor controller (orchestration)
│
├── controller/event/handler/artifactprocessor/
│   └── handler.go                # Event handler for TopicPushArtifact
│
├── server/v2.0/handler/
│   └── processor.go              # API handler
│
└── jobservice/job/
    └── known_jobs.go             # Add ARTIFACT_PROCESSOR vendor type

Files to modify:
├── src/controller/event/handler/init.go   # Register new event handler
└── src/jobservice/job/known_jobs.go       # Add vendor type constant
```

### 9. Execution Flow

#### Automatic Processing (Push Event)

```
1. User pushes image to Harbor registry
2. Harbor publishes TopicPushArtifact event
3. ArtifactProcessor EventHandler receives event
4. Handler calls ProcessorController.ProcessArtifact(ctx, artifact)
5. Controller queries artifact_processor_policy for project
6. For each enabled policy:
   a. Get processor from registry by type
   b. Check processor.ShouldProcess(artifact) — skip if false
   c. Check filters (repository pattern, tag pattern, etc.)
   d. Create execution via task.ExecutionManager (vendor=ARTIFACT_PROCESSOR)
   e. Call processor.Process(ctx, artifact, config)
      - Internal: submits job to job service
      - External: sends HTTP request to external service
7. Execution status updates tracked via task framework
8. On completion:
   - Companion artifacts recorded via accessory model
   - Converted artifacts appear with configured tag suffix
```

#### Manual Trigger

```
1. User calls POST /api/v2.0/projects/{id}/processors/executions
   with body: {"artifact_id": 123, "processor_type": "soci"}
2. Same flow as steps 5-8 above, but triggered via API
```

### 10. Integration with Existing Harbor Systems

| System | Integration Point | Details |
|--------|------------------|---------|
| Event System | `notifier.Subscribe` | Subscribe to `TopicPushArtifact` |
| Task/Execution | `task.ExecutionManager` | Track processing status with vendor type `ARTIFACT_PROCESSOR` |
| Accessory Model | `accessory/model` | Register SOCI index as new accessory type (`accelerator.soci`) |
| Job Service | `job.Interface` | Internal processors run as jobs |
| RBAC | Existing project permissions | Policy management requires project admin |
| Quota | Existing quota system | Converted/companion artifacts count toward project quota |

### 11. SOCI vs eStargz Comparison

| Aspect | SOCI | eStargz |
|--------|------|---------|
| Approach | Creates companion index artifact | Converts image to new format |
| Original image | Unchanged | Unchanged (new artifact created) |
| Storage | Index stored as OCI referrer | Full converted image stored alongside |
| Storage overhead | Small (index + zTOCs) | Large (full image copy) |
| Execution | Internal (Harbor job service) | External (acceleration-service) |
| Output | Companion artifact (referrer) | New tagged artifact (e.g., `:tag-estargz`) |
| Runtime requirement | SOCI snapshotter | eStargz-aware snapshotter |

### 12. Migration Strategy

1. **Database**: Add migration in `make/migrations/postgresql/` for the
   `artifact_processor_policy` table
2. **Accessory type**: Register `accelerator.soci` in accessory model constants
3. **Vendor type**: Add `ArtifactProcessorVendorType` to known jobs
4. **Feature flag**: Initially behind a feature flag for opt-in enablement

### 13. Future Extensions

The framework naturally supports additional processors:

- **Nydus converter**: Similar to eStargz, delegates to acceleration-service with
  `driver=nydus`
- **zstd:chunked converter**: Another acceleration-service driver
- **OCI-to-Docker converter**: Internal processor for format compatibility
- **Custom webhook processor**: Generic HTTP POST to any endpoint on artifact push
- **SBOM generator**: Automatic SBOM generation on push (similar to scan-on-push)

Each new processor only needs to:
1. Implement the `Processor` interface
2. Call `Register()` in its `init()` function
3. Be imported in the appropriate place for side-effect registration
