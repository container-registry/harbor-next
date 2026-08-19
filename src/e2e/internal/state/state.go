//go:build e2e

// Package state holds per-scenario state carried on a typed context key.
package state

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/goharbor/harbor/src/e2e/internal/fakeidp"
	"github.com/goharbor/harbor/src/e2e/internal/harborclient"
	"github.com/goharbor/harbor/src/e2e/internal/otlpreceiver"
)

type stateKey struct{}

// Robot is a project-scoped robot credential captured for later use.
type Robot struct {
	ID      int64
	Name    string
	Secret  string
	Project string
}

// UserCred identifies a provisioned test user and its credentials.
type UserCred struct {
	ID       int64
	Name     string
	Password string
	Email    string
}

// WebhookEvent is a captured notification payload delivered to the in-process listener.
type WebhookEvent struct {
	Type   string
	Body   []byte
	Header http.Header
}

// Scenario is the mutable state shared between steps of a single scenario.
type Scenario struct {
	Mu sync.Mutex

	// Suffix is the per-scenario unique token appended to every created resource name.
	Suffix string

	// Client is the admin API client.
	Client *harborclient.Client

	// RegistryHost is the host:port portion of the Harbor endpoint (for image refs).
	RegistryHost string

	// Created resources — walked in reverse in sc.After for best-effort teardown.
	CreatedProjects       []string
	CreatedUsers          []UserCred
	UsersByAlias          map[string]UserCred
	CreatedRobots         []Robot
	RobotsByAlias         map[string]Robot
	CreatedLabels         []int64
	CreatedWebhookIDs     map[string][]int64 // project → policy ids
	CreatedRegistries     []int64
	CreatedReplicationIDs []int64
	CreatedImmutability   map[string][]int64 // project → rule ids
	TempDirs              []string
	HTTPMock              *httptest.Server
	WebhookEvents         chan WebhookEvent
	OTLPReceiver          *otlpreceiver.Receiver
	AuditOTLPRestore      func() error

	// FedIDP state.
	CreatedIdPs          []int64                 // IDP IDs to delete in teardown (robots must be deleted first)
	FakeIdPs             map[string]*fakeidp.IdP // label → fake IdP instance (for JWT issuance in steps)
	LastIdPID            int64                   // most recently created IdP ID
	LastRobotIdPID       int64                   // robot ID for the last FedIDP robot
	LastJWT              string                  // last issued JWT (for auth step assertions)
	ProjectFedIDPRestore func() error            // restores enable_project_federated_idp after gate-mutation scenarios

	// Cosign key material paths per label (empty label = default).
	CosignKeys map[string]string // label → dir containing cosign.key/cosign.pub

	// Multi-arch build fixture (path to Dockerfile).
	BuildFixtureDir string

	// SBOM fixture path.
	SBOMPredicatePath string

	// Image round-trip state.
	LastImageRef  string
	PushedDigest  string
	PulledDigest  string
	LastArtifacts []map[string]any

	// HTTP capture of last API call for assertions.
	LastResp *http.Response
	LastBody []byte
	LastErr  error

	// Last audit operation requested by the audit-log steps.
	LastAuditOperation string

	// Last CLI shellout (cosign / buildx / docker).
	LastCLIStdout []byte
	LastCLIStderr []byte
	LastCLIErr    error

	// Last image push/pull error (crane).
	LastImageErr error

	// Captured replication / scan state.
	LastReplicationID    int64
	SFTPRegistryID       int64
	SFTPEventPolicyID    int64
	SFTPBasePath         string
	SFTPBackupRepository string
	SFTPBackupTag        string
	LastScanArtifact     string

	// Captured Prometheus metrics by Harbor service name.
	MetricsByService map[string][]byte

	// Branding tests.
	LastBrandingConfig map[string]any
	BrandingRestore    func() error

	// Native package (npm / Maven) scenario state.
	//
	// PkgBaseName is the name as written in the feature file; PkgName is that
	// name with the scenario suffix appended so concurrent runs — and repeat
	// runs against a long-lived registry — never collide. Steps address the
	// package by its base name and resolve through PkgName.
	PkgBaseName     string
	PkgName         string
	PkgVersion      string
	PkgProject      string // Harbor project name (already suffix-resolved)
	PkgDir          string // working directory holding the package sources
	PkgFormat       string // "npm" or "maven"
	MavenGroupID    string
	MavenArtifactID string

	// Result of the most recent consume (npm install / Maven resolve).
	InstalledVersion string
	InstalledContent []byte
}

// NewSuffix returns a short random token suitable for resource-name suffixes.
func NewSuffix() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "xxxxxxxx"
	}
	return hex.EncodeToString(buf)
}

// NewScenario constructs an empty scenario with a fresh suffix and client.
func NewScenario(c *harborclient.Client) *Scenario {
	return &Scenario{
		Suffix:              NewSuffix(),
		Client:              c,
		RegistryHost:        c.Host(),
		UsersByAlias:        map[string]UserCred{},
		RobotsByAlias:       map[string]Robot{},
		CreatedWebhookIDs:   map[string][]int64{},
		CreatedImmutability: map[string][]int64{},
		CosignKeys:          map[string]string{},
		FakeIdPs:            map[string]*fakeidp.IdP{},
		MetricsByService:    map[string][]byte{},
	}
}

// Put stores the scenario on a context.
func Put(ctx context.Context, s *Scenario) context.Context {
	return context.WithValue(ctx, stateKey{}, s)
}

// Get retrieves the scenario from a context. Panics if absent — every step must
// run after hooks.BeforeScenario has populated it.
func Get(ctx context.Context) *Scenario {
	s, ok := ctx.Value(stateKey{}).(*Scenario)
	if !ok || s == nil {
		panic("e2e: scenario state missing — ensure BeforeScenario hook ran")
	}
	return s
}
