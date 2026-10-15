package p2p

import (
	"context"
	"fmt"
	"testing"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact"
	"github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/p2p/preheat"
	"github.com/goharbor/harbor/src/controller/tag"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/q"
	pkg_artifact "github.com/goharbor/harbor/src/pkg/artifact"
	scan_dao "github.com/goharbor/harbor/src/pkg/scan/dao/scan"
	"github.com/goharbor/harbor/src/pkg/scan/rest/v1"
	pkg_tag "github.com/goharbor/harbor/src/pkg/tag/model/tag"
	test_artifact "github.com/goharbor/harbor/src/testing/controller/artifact"
	test_preheat "github.com/goharbor/harbor/src/testing/controller/p2p/preheat"
	test_scan "github.com/goharbor/harbor/src/testing/controller/scan"
	"github.com/goharbor/harbor/src/testing/mock"
	test_pkg_artifact "github.com/goharbor/harbor/src/testing/pkg/artifact"
)

// PreheatTestSuite is a test suite of testing preheat handler
type PreheatTestSuite struct {
	suite.Suite
	artifactCtl artifact.Controller
	preheatEnf  preheat.Enforcer

	handler *Handler
}

// TestPreheat is an entry method of running PreheatTestSuite
func TestPreheat(t *testing.T) {
	suite.Run(t, &PreheatTestSuite{})
}

// SetupSuite prepares env for running PreheatTestSuite
func (suite *PreheatTestSuite) SetupSuite() {
	fakeArtifactCtl := &test_artifact.Controller{}
	fakeArtifactCtl.On("GetByReference",
		context.TODO(),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("string"),
		mock.AnythingOfType("*artifact.Option"),
	).Return(&artifact.Artifact{}, nil)
	fakeArtifactCtl.On("Get",
		context.TODO(),
		mock.AnythingOfType("int64"),
		mock.AnythingOfType("*artifact.Option"),
	).Return(&artifact.Artifact{
		Artifact: pkg_artifact.Artifact{
			Type: "IMAGE",
		},
	}, nil)

	fakeEnforcer := &test_preheat.FakeEnforcer{}
	fakeEnforcer.On("PreheatArtifact",
		context.TODO(),
		mock.AnythingOfType("*artifact.Artifact"),
	).Return(nil, nil)

	suite.artifactCtl = artifact.Ctl
	artifact.Ctl = fakeArtifactCtl
	suite.preheatEnf = preheat.Enf
	preheat.Enf = fakeEnforcer
	fakeArtifactMgr := &test_pkg_artifact.Manager{}
	fakeArtifactMgr.On("ListReferences",
		context.TODO(),
		mock.Anything,
	).Return(nil, nil)

	suite.handler = &Handler{artMgr: fakeArtifactMgr}
}

// TearDownSuite cleans the testing env
func (suite *PreheatTestSuite) TearDownSuite() {
	artifact.Ctl = suite.artifactCtl
	preheat.Enf = suite.preheatEnf
}

// TestIsStateful ...
func (suite *PreheatTestSuite) TestIsStateful() {
	b := suite.handler.IsStateful()
	suite.False(b, "handler is stateful")
}

func (suite *PreheatTestSuite) TestName() {
	suite.Equal("P2PPreheat", suite.handler.Name())
}

// TestHandle ...
func (suite *PreheatTestSuite) TestHandle() {
	type args struct {
		data any
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "PreheatHandler String Error",
			args: args{
				data: "",
			},
			wantErr: true,
		},
		{
			name: "PreheatHandler 1",
			args: args{
				data: &event.PushArtifactEvent{
					ArtifactEvent: &event.ArtifactEvent{
						Artifact: &pkg_artifact.Artifact{
							Type:         "IMAGE",
							ID:           11,
							RepositoryID: 23,
						},
						Tags: []string{"v1.1", "v1.2"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "PreheatHandler 2",
			args: args{
				data: &event.ScanImageEvent{
					OccurAt: time.Now(),
					Artifact: &v1.Artifact{
						Repository: "library",
						Digest:     "sha256:1359608115b94599e5641638bac5aef1ddfaa79bb96057ebf41ebc8d33acf8a7",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "PreheatHandler 3",
			args: args{
				data: &event.ArtifactLabeledEvent{
					OccurAt:    time.Now(),
					ArtifactID: 1,
					LabelID:    2,
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		err := suite.handler.Handle(context.TODO(), tt.args.data)
		if tt.wantErr {
			suite.Error(err, tt.name)
		} else {
			suite.NoError(err, tt.name)
		}
	}
}

const (
	testChildID       int64 = 11
	testSiblingID     int64 = 12
	testIndexID       int64 = 10
	testRepoName            = "library/busybox"
	testChildDigest         = "sha256:bbbb"
	testSiblingDigest       = "sha256:aaaa"
)

func newTestArtifact(id int64, digest string, tags ...string) *artifact.Artifact {
	art := &artifact.Artifact{Artifact: pkg_artifact.Artifact{ID: id, Digest: digest, RepositoryName: testRepoName, Type: "IMAGE"}}
	for _, n := range tags {
		art.Tags = append(art.Tags, &tag.Tag{Tag: pkg_tag.Tag{Name: n}})
	}
	return art
}

func newTestIndex(id int64, digest string, tags ...string) *artifact.Artifact {
	art := newTestArtifact(id, digest, tags...)
	art.ManifestMediaType = ocispec.MediaTypeImageIndex
	return art
}

func testReport(digest string, status job.Status, end time.Time) []*scan_dao.Report {
	return []*scan_dao.Report{{Digest: digest, Status: status.String(), EndTime: end}}
}

func testClaimKey(end time.Time) string {
	return fmt.Sprintf("p2p:preheat:scanned_index:%d:%d", testIndexID, end.UnixNano())
}

type scannedFixture struct {
	scanned    *artifact.Artifact
	references []*pkg_artifact.Reference
	listErr    error
	parent     *artifact.Artifact
	parentErr  error
	walked     []*artifact.Artifact
	reports    map[string][]*scan_dao.Report
	reportErrs map[string]error
	claims     map[string]bool
	claimErr   error
	preheatErr error
	preheated  []int64
}

func (f *scannedFixture) mock(t *testing.T) *Handler {
	ctx := context.TODO()
	originalCtl, originalEnf := artifact.Ctl, preheat.Enf
	t.Cleanup(func() {
		artifact.Ctl, preheat.Enf = originalCtl, originalEnf
	})

	artCtl := &test_artifact.Controller{}
	artCtl.On("GetByReference", ctx, testRepoName, mock.Anything, mock.Anything).Return(
		func(context.Context, string, string, *artifact.Option) (*artifact.Artifact, error) {
			return f.scanned, nil
		})
	artCtl.On("Get", ctx, testIndexID, mock.Anything).Return(
		func(context.Context, int64, *artifact.Option) (*artifact.Artifact, error) {
			return f.parent, f.parentErr
		})
	artCtl.On("Walk", ctx, mock.Anything, mock.Anything, mock.Anything).Return(
		func(_ context.Context, root *artifact.Artifact, walkFn func(*artifact.Artifact) error, _ *artifact.Option) error {
			walked := f.walked
			if walked == nil {
				walked = []*artifact.Artifact{newTestArtifact(testChildID, testChildDigest), newTestArtifact(testSiblingID, testSiblingDigest)}
			}
			for _, a := range append([]*artifact.Artifact{root}, walked...) {
				if err := walkFn(a); err != nil {
					if err == artifact.ErrBreak {
						return nil
					}
					return err
				}
			}
			return nil
		})
	artifact.Ctl = artCtl

	enf := &test_preheat.FakeEnforcer{}
	enf.On("PreheatArtifact", ctx, mock.Anything).Run(func(args mock.Arguments) {
		f.preheated = append(f.preheated, args.Get(1).(*artifact.Artifact).ID)
	}).Return(nil, f.preheatErr)
	preheat.Enf = enf

	artMgr := &test_pkg_artifact.Manager{}
	artMgr.On("ListReferences", ctx, mock.Anything).Return(
		func(context.Context, *q.Query) ([]*pkg_artifact.Reference, error) {
			return f.references, f.listErr
		})
	scanCtl := &test_scan.Controller{}
	scanCtl.On("GetReport", ctx, mock.Anything, []string(nil)).Return(
		func(_ context.Context, a *artifact.Artifact, _ []string) ([]*scan_dao.Report, error) {
			return f.reports[a.Digest], f.reportErrs[a.Digest]
		})

	claim := func(_ context.Context, key string) (bool, error) {
		if f.claimErr != nil {
			return false, f.claimErr
		}
		if f.claims == nil {
			f.claims = map[string]bool{}
		}
		if f.claims[key] {
			return false, nil
		}
		f.claims[key] = true
		return true, nil
	}

	return &Handler{artMgr: artMgr, scanCtl: scanCtl, claim: claim}
}

func handleScanned(h *Handler, eventType, scanType, digest string) error {
	return h.Handle(context.TODO(), &event.ScanImageEvent{
		EventType: eventType,
		OccurAt:   time.Now(),
		ScanType:  scanType,
		Artifact:  &v1.Artifact{Repository: testRepoName, Digest: digest},
	})
}

func TestHandleImageScanned(t *testing.T) {
	now := time.Now()
	untaggedChild := newTestArtifact(testChildID, testChildDigest)
	taggedChild := newTestArtifact(testChildID, testChildDigest, "v1-amd64")
	taggedIndex := newTestIndex(testIndexID, "sha256:index", "v1")
	references := []*pkg_artifact.Reference{
		{ParentID: testIndexID, ChildID: testChildID},
		{ParentID: testIndexID, ChildID: testChildID},
	}
	finished := map[string][]*scan_dao.Report{
		testSiblingDigest: testReport(testSiblingDigest, job.ErrorStatus, now.Add(-time.Minute)),
		testChildDigest:   testReport(testChildDigest, job.SuccessStatus, now),
	}

	tests := []struct {
		name      string
		eventType string
		scanType  string
		fixture   scannedFixture
		preheated []int64
		wantErr   bool
	}{
		{
			name:      "tagged artifact is preheated itself",
			fixture:   scannedFixture{scanned: taggedChild},
			preheated: []int64{testChildID},
		},
		{
			name:      "tagged artifact is not preheated itself for a failed scan",
			eventType: event.TopicScanningFailed,
			fixture:   scannedFixture{scanned: taggedChild},
		},
		{
			name:    "untagged artifact without parent is ignored",
			fixture: scannedFixture{scanned: untaggedChild},
		},
		{
			name:      "index is preheated once the scans of all its children are finished",
			fixture:   scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: finished},
			preheated: []int64{testIndexID},
		},
		{
			name:      "explicit vulnerability scan type is handled",
			scanType:  v1.ScanTypeVulnerability,
			fixture:   scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: finished},
			preheated: []int64{testIndexID},
		},
		{
			name:      "index is preheated for a failed scan finishing last",
			eventType: event.TopicScanningFailed,
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: map[string][]*scan_dao.Report{
				testSiblingDigest: testReport(testSiblingDigest, job.SuccessStatus, now.Add(-time.Minute)),
				testChildDigest:   testReport(testChildDigest, job.ErrorStatus, now),
			}},
			preheated: []int64{testIndexID},
		},
		{
			name:      "index is preheated for a stopped scan finishing last",
			eventType: event.TopicScanningStopped,
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: map[string][]*scan_dao.Report{
				testSiblingDigest: testReport(testSiblingDigest, job.SuccessStatus, now.Add(-time.Minute)),
				testChildDigest:   testReport(testChildDigest, job.StoppedStatus, now),
			}},
			preheated: []int64{testIndexID},
		},
		{
			name:      "tagged child finishing last preheats itself and its index",
			fixture:   scannedFixture{scanned: taggedChild, references: references, parent: taggedIndex, reports: finished},
			preheated: []int64{testChildID, testIndexID},
		},
		{
			name:      "tagged child failing last preheats its index only",
			eventType: event.TopicScanningFailed,
			fixture: scannedFixture{scanned: taggedChild, references: references, parent: taggedIndex, reports: map[string][]*scan_dao.Report{
				testSiblingDigest: testReport(testSiblingDigest, job.SuccessStatus, now.Add(-time.Minute)),
				testChildDigest:   testReport(testChildDigest, job.ErrorStatus, now),
			}},
			preheated: []int64{testIndexID},
		},
		{
			name: "index is not preheated while a sibling is still being scanned",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: map[string][]*scan_dao.Report{
				testSiblingDigest: testReport(testSiblingDigest, job.RunningStatus, time.Time{}),
				testChildDigest:   testReport(testChildDigest, job.SuccessStatus, now),
			}},
		},
		{
			name: "index is not preheated while a sibling scan is pending",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: map[string][]*scan_dao.Report{
				testSiblingDigest: testReport(testSiblingDigest, job.PendingStatus, time.Time{}),
				testChildDigest:   testReport(testChildDigest, job.SuccessStatus, now),
			}},
		},
		{
			name: "index is not preheated when a sibling has no report",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: map[string][]*scan_dao.Report{
				testChildDigest: testReport(testChildDigest, job.SuccessStatus, now),
			}},
		},
		{
			name: "sibling not scannable by the scanner is ignored",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex,
				reports:    map[string][]*scan_dao.Report{testChildDigest: testReport(testChildDigest, job.SuccessStatus, now)},
				reportErrs: map[string]error{testSiblingDigest: errors.NotFoundError(nil)},
			},
			preheated: []int64{testIndexID},
		},
		{
			name: "children of a nested index are walked",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex,
				walked: []*artifact.Artifact{untaggedChild, newTestIndex(13, "sha256:nested"), newTestArtifact(14, "sha256:cccc")},
				reports: map[string][]*scan_dao.Report{
					"sha256:cccc":   testReport("sha256:cccc", job.RunningStatus, time.Time{}),
					testChildDigest: testReport(testChildDigest, job.SuccessStatus, now),
				},
			},
		},
		{
			name: "index preheated for the same scans already is skipped",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: finished,
				claims: map[string]bool{testClaimKey(now): true},
			},
		},
		{
			name:    "untagged index is ignored",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: newTestIndex(testIndexID, "sha256:index")},
		},
		{
			name:    "deleted index is ignored",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parentErr: errors.NotFoundError(nil)},
		},
		{
			name: "index without scanner is ignored",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reportErrs: map[string]error{
				testChildDigest:   errors.NotFoundError(nil),
				testSiblingDigest: errors.NotFoundError(nil),
			}},
		},
		{
			name:     "sbom scan of untagged child is ignored",
			scanType: v1.ScanTypeSbom,
			fixture:  scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: finished},
		},
		{
			name:      "sbom scan of tagged child preheats the child only",
			scanType:  v1.ScanTypeSbom,
			fixture:   scannedFixture{scanned: taggedChild, references: references, parent: taggedIndex, reports: finished},
			preheated: []int64{testChildID},
		},
		{
			name:    "listing references fails",
			fixture: scannedFixture{scanned: untaggedChild, listErr: errors.New("db error")},
			wantErr: true,
		},
		{
			name:    "getting index fails",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parentErr: errors.New("db error")},
			wantErr: true,
		},
		{
			name: "getting reports fails",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex,
				reportErrs: map[string]error{testChildDigest: errors.New("scanner error")},
			},
			wantErr: true,
		},
		{
			name: "claiming the preheat fails",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: finished,
				claimErr: errors.New("redis error"),
			},
			wantErr: true,
		},
		{
			name: "preheating index fails",
			fixture: scannedFixture{scanned: untaggedChild, references: references, parent: taggedIndex, reports: finished,
				preheatErr: errors.New("enforce error"),
			},
			preheated: []int64{testIndexID},
			wantErr:   true,
		},
		{
			name: "failing to preheat tagged child still preheats its index",
			fixture: scannedFixture{scanned: taggedChild, references: references, parent: taggedIndex, reports: finished,
				preheatErr: errors.New("enforce error"),
			},
			preheated: []int64{testChildID, testIndexID},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventType := tt.eventType
			if eventType == "" {
				eventType = event.TopicScanningCompleted
			}
			f := tt.fixture
			err := handleScanned(f.mock(t), eventType, tt.scanType, testChildDigest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.preheated, f.preheated)
		})
	}
}

func TestHandleImageScannedSuccessively(t *testing.T) {
	now := time.Now()
	child := newTestArtifact(testChildID, testChildDigest)
	sibling := newTestArtifact(testSiblingID, testSiblingDigest)
	f := &scannedFixture{
		references: []*pkg_artifact.Reference{{ParentID: testIndexID, ChildID: testChildID}},
		parent:     newTestIndex(testIndexID, "sha256:index", "v1"),
		reports: map[string][]*scan_dao.Report{
			testChildDigest:   testReport(testChildDigest, job.SuccessStatus, now),
			testSiblingDigest: testReport(testSiblingDigest, job.RunningStatus, time.Time{}),
		},
	}
	h := f.mock(t)

	f.scanned = child
	assert.NoError(t, handleScanned(h, event.TopicScanningCompleted, "", testChildDigest))
	assert.Empty(t, f.preheated)

	f.reports[testSiblingDigest] = testReport(testSiblingDigest, job.SuccessStatus, now.Add(-time.Second))
	f.scanned = sibling
	assert.NoError(t, handleScanned(h, event.TopicScanningCompleted, "", testSiblingDigest))
	assert.Equal(t, []int64{testIndexID}, f.preheated)

	f.scanned = child
	assert.NoError(t, handleScanned(h, event.TopicScanningCompleted, "", testChildDigest))
	assert.Equal(t, []int64{testIndexID}, f.preheated)

	f.reports[testChildDigest] = testReport(testChildDigest, job.ErrorStatus, now.Add(time.Minute))
	assert.NoError(t, handleScanned(h, event.TopicScanningFailed, "", testChildDigest))
	assert.Equal(t, []int64{testIndexID, testIndexID}, f.preheated)
}
