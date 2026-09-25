package p2p

import (
	"context"
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

// TestHandleImageScanned covers resolving the scanned children of an image index to the index
func TestHandleImageScanned(t *testing.T) {
	const (
		childID       int64 = 11
		indexID       int64 = 10
		repoName            = "library/busybox"
		childDigest         = "sha256:bbbb"
		siblingDigest       = "sha256:aaaa"
		nestedDigest        = "sha256:nested"
		grandDigest         = "sha256:cccc"
	)
	newArtifact := func(id int64, digest string, tags ...string) *artifact.Artifact {
		art := &artifact.Artifact{Artifact: pkg_artifact.Artifact{ID: id, Digest: digest, RepositoryName: repoName, Type: "IMAGE"}}
		for _, n := range tags {
			art.Tags = append(art.Tags, &tag.Tag{Tag: pkg_tag.Tag{Name: n}})
		}
		return art
	}
	newIndex := func(id int64, digest string, tags ...string) *artifact.Artifact {
		art := newArtifact(id, digest, tags...)
		art.ManifestMediaType = ocispec.MediaTypeImageIndex
		return art
	}
	now := time.Now()
	report := func(digest string, status job.Status, end time.Time) []*scan_dao.Report {
		return []*scan_dao.Report{{Digest: digest, Status: status.String(), EndTime: end}}
	}
	untaggedChild := newArtifact(childID, childDigest)
	sibling := newArtifact(12, siblingDigest)
	taggedIndex := newIndex(indexID, "sha256:index", "v1")
	references := []*pkg_artifact.Reference{
		{ParentID: indexID, ChildID: childID},
		{ParentID: indexID, ChildID: childID},
	}

	tests := []struct {
		name       string
		scanType   string
		scanned    *artifact.Artifact
		references []*pkg_artifact.Reference
		listErr    error
		parent     *artifact.Artifact
		parentErr  error
		walked     []*artifact.Artifact
		reports    map[string][]*scan_dao.Report
		reportErrs map[string]error
		preheatErr error
		preheated  []int64
		wantErr    bool
	}{
		{
			name:      "tagged artifact is preheated itself",
			scanned:   newArtifact(childID, childDigest, "v1"),
			preheated: []int64{childID},
		},
		{
			name:    "untagged artifact without parent is ignored",
			scanned: untaggedChild,
		},
		{
			name:       "child finishing last preheats its tagged index",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				siblingDigest: report(siblingDigest, job.ErrorStatus, now.Add(-time.Minute)),
				childDigest:   report(childDigest, job.SuccessStatus, now),
			},
			preheated: []int64{indexID},
		},
		{
			name:       "explicit vulnerability scan type is handled",
			scanType:   v1.ScanTypeVulnerability,
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				siblingDigest: report(siblingDigest, job.StoppedStatus, now.Add(-time.Minute)),
				childDigest:   report(childDigest, job.SuccessStatus, now),
			},
			preheated: []int64{indexID},
		},
		{
			name:       "child not finishing last leaves the index to its sibling",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				siblingDigest: report(siblingDigest, job.SuccessStatus, now),
				childDigest:   report(childDigest, job.SuccessStatus, now.Add(-time.Minute)),
			},
		},
		{
			name:       "children finishing at the same time are ordered by digest",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				childDigest:   report(childDigest, job.SuccessStatus, now),
				siblingDigest: report(siblingDigest, job.SuccessStatus, now),
			},
			preheated: []int64{indexID},
		},
		{
			name:       "index is not preheated while a sibling is still being scanned",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				siblingDigest: report(siblingDigest, job.RunningStatus, time.Time{}),
				childDigest:   report(childDigest, job.SuccessStatus, now),
			},
		},
		{
			name:       "index is not preheated while a sibling scan is pending",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				siblingDigest: report(siblingDigest, job.PendingStatus, time.Time{}),
				childDigest:   report(childDigest, job.SuccessStatus, now),
			},
		},
		{
			name:       "index is not preheated when a sibling has no report",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				childDigest: report(childDigest, job.SuccessStatus, now),
			},
		},
		{
			name:       "sibling not scannable by the scanner is ignored",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				childDigest: report(childDigest, job.SuccessStatus, now),
			},
			reportErrs: map[string]error{siblingDigest: errors.NotFoundError(nil)},
			preheated:  []int64{indexID},
		},
		{
			name:       "children of a nested index are walked",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			walked:     []*artifact.Artifact{untaggedChild, newIndex(13, nestedDigest), newArtifact(14, grandDigest)},
			reports: map[string][]*scan_dao.Report{
				grandDigest: report(grandDigest, job.SuccessStatus, now.Add(time.Minute)),
				childDigest: report(childDigest, job.SuccessStatus, now),
			},
		},
		{
			name:       "untagged index is ignored",
			scanned:    untaggedChild,
			references: references,
			parent:     newIndex(indexID, "sha256:index"),
		},
		{
			name:       "deleted index is ignored",
			scanned:    untaggedChild,
			references: references,
			parentErr:  errors.NotFoundError(nil),
		},
		{
			name:       "index without scanner is ignored",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reportErrs: map[string]error{
				childDigest:   errors.NotFoundError(nil),
				siblingDigest: errors.NotFoundError(nil),
			},
		},
		{
			name:     "sbom scan of untagged child is ignored",
			scanType: v1.ScanTypeSbom,
			scanned:  untaggedChild,
		},
		{
			name:    "listing references fails",
			scanned: untaggedChild,
			listErr: errors.New("db error"),
			wantErr: true,
		},
		{
			name:       "getting index fails",
			scanned:    untaggedChild,
			references: references,
			parentErr:  errors.New("db error"),
			wantErr:    true,
		},
		{
			name:       "getting reports fails",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reportErrs: map[string]error{childDigest: errors.New("scanner error")},
			wantErr:    true,
		},
		{
			name:       "preheating index fails",
			scanned:    untaggedChild,
			references: references,
			parent:     taggedIndex,
			reports: map[string][]*scan_dao.Report{
				childDigest:   report(childDigest, job.SuccessStatus, now),
				siblingDigest: report(siblingDigest, job.SuccessStatus, now.Add(-time.Minute)),
			},
			preheatErr: errors.New("enforce error"),
			preheated:  []int64{indexID},
			wantErr:    true,
		},
	}

	originalCtl, originalEnf := artifact.Ctl, preheat.Enf
	defer func() {
		artifact.Ctl, preheat.Enf = originalCtl, originalEnf
	}()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.TODO()

			walked := tt.walked
			if walked == nil {
				walked = []*artifact.Artifact{untaggedChild, sibling}
			}

			artCtl := &test_artifact.Controller{}
			artCtl.On("GetByReference", ctx, repoName, childDigest, mock.Anything).Return(tt.scanned, nil)
			artCtl.On("Get", ctx, indexID, mock.Anything).Return(tt.parent, tt.parentErr)
			artCtl.On("Walk", ctx, tt.parent, mock.Anything, mock.Anything).Return(
				func(_ context.Context, root *artifact.Artifact, walkFn func(*artifact.Artifact) error, _ *artifact.Option) error {
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

			var preheated []int64
			enf := &test_preheat.FakeEnforcer{}
			enf.On("PreheatArtifact", ctx, mock.Anything).Run(func(args mock.Arguments) {
				preheated = append(preheated, args.Get(1).(*artifact.Artifact).ID)
			}).Return(nil, tt.preheatErr)
			preheat.Enf = enf

			artMgr := &test_pkg_artifact.Manager{}
			artMgr.On("ListReferences", ctx, mock.Anything).Return(tt.references, tt.listErr)
			scanCtl := &test_scan.Controller{}
			scanCtl.On("GetReport", ctx, mock.Anything, []string(nil)).Return(
				func(_ context.Context, a *artifact.Artifact, _ []string) ([]*scan_dao.Report, error) {
					return tt.reports[a.Digest], tt.reportErrs[a.Digest]
				})

			handler := &Handler{artMgr: artMgr, scanCtl: scanCtl}
			err := handler.Handle(ctx, &event.ScanImageEvent{
				OccurAt:  time.Now(),
				ScanType: tt.scanType,
				Artifact: &v1.Artifact{Repository: repoName, Digest: childDigest},
			})
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.preheated, preheated)
		})
	}
}
