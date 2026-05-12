// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package artifact

import (
	"context"
	"testing"
	"time"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact/processor/chart"
	"github.com/goharbor/harbor/src/controller/artifact/processor/cnab"
	"github.com/goharbor/harbor/src/controller/artifact/processor/image"
	"github.com/goharbor/harbor/src/controller/tag"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/icon"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	accessorymodel "github.com/goharbor/harbor/src/pkg/accessory/model"
	basemodel "github.com/goharbor/harbor/src/pkg/accessory/model/base"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/pkg/blob/models"
	"github.com/goharbor/harbor/src/pkg/label/model"
	projectModel "github.com/goharbor/harbor/src/pkg/project/models"
	repomodel "github.com/goharbor/harbor/src/pkg/repository/model"
	model_tag "github.com/goharbor/harbor/src/pkg/tag/model/tag"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	tagtesting "github.com/goharbor/harbor/src/testing/controller/tag"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
	accessorytesting "github.com/goharbor/harbor/src/testing/pkg/accessory"
	arttesting "github.com/goharbor/harbor/src/testing/pkg/artifact"
	artrashtesting "github.com/goharbor/harbor/src/testing/pkg/artifactrash"
	"github.com/goharbor/harbor/src/testing/pkg/blob"
	"github.com/goharbor/harbor/src/testing/pkg/immutable"
	"github.com/goharbor/harbor/src/testing/pkg/label"
	"github.com/goharbor/harbor/src/testing/pkg/registry"
	repotesting "github.com/goharbor/harbor/src/testing/pkg/repository"
)

// TODO find another way to test artifact controller, it's hard to maintain currently

type fakeAbstractor struct {
	mock.Mock
}

func (f *fakeAbstractor) AbstractMetadata(ctx context.Context, artifact *artifact.Artifact) error {
	args := f.Called()
	return args.Error(0)
}

type stubAbstractor struct {
	fn func(context.Context, *artifact.Artifact) error
}

func (s stubAbstractor) AbstractMetadata(ctx context.Context, art *artifact.Artifact) error {
	return s.fn(ctx, art)
}

type controllerTestSuite struct {
	suite.Suite
	ctl          *controller
	repoMgr      *repotesting.Manager
	artMgr       *arttesting.Manager
	artrashMgr   *artrashtesting.Manager
	blobMgr      *blob.Manager
	tagCtl       *tagtesting.FakeController
	labelMgr     *label.Manager
	abstractor   *fakeAbstractor
	immutableMtr *immutable.FakeMatcher
	regCli       *registry.Client
	accMgr       *accessorytesting.Manager
	proCtl       *projecttesting.Controller
}

// SetupTest resets all mock fields and creates a bare controller.
// Each test must call the setupXxx helpers for the mocks it needs,
// keeping the total mock.Mock instance count (and race-detector
// overhead) proportional to actual usage instead of O(mocks×tests).
func (c *controllerTestSuite) SetupTest() {
	c.repoMgr = nil
	c.artMgr = nil
	c.artrashMgr = nil
	c.blobMgr = nil
	c.tagCtl = nil
	c.labelMgr = nil
	c.abstractor = nil
	c.immutableMtr = nil
	c.regCli = nil
	c.accMgr = nil
	c.proCtl = nil
	c.ctl = &controller{}
}

func (c *controllerTestSuite) setupRepoMgr() {
	c.repoMgr = &repotesting.Manager{}
	c.ctl.repoMgr = c.repoMgr
}

func (c *controllerTestSuite) setupArtMgr() {
	c.artMgr = &arttesting.Manager{}
	c.ctl.artMgr = c.artMgr
}

func (c *controllerTestSuite) setupArtrashMgr() {
	c.artrashMgr = &artrashtesting.Manager{}
	c.ctl.artrashMgr = c.artrashMgr
}

func (c *controllerTestSuite) setupBlobMgr() {
	c.blobMgr = &blob.Manager{}
	c.ctl.blobMgr = c.blobMgr
}

func (c *controllerTestSuite) setupTagCtl() {
	c.tagCtl = &tagtesting.FakeController{}
	c.ctl.tagCtl = c.tagCtl
}

func (c *controllerTestSuite) setupLabelMgr() {
	c.labelMgr = &label.Manager{}
	c.ctl.labelMgr = c.labelMgr
}

func (c *controllerTestSuite) setupAbstractor() {
	c.abstractor = &fakeAbstractor{}
	c.ctl.abstractor = c.abstractor
}

func (c *controllerTestSuite) setupImmutableMtr() {
	c.immutableMtr = &immutable.FakeMatcher{}
	c.ctl.immutableMtr = c.immutableMtr
}

func (c *controllerTestSuite) setupRegCli() {
	c.regCli = &registry.Client{}
	c.ctl.regCli = c.regCli
}

func (c *controllerTestSuite) setupAccMgr() {
	c.accMgr = &accessorytesting.Manager{}
	c.ctl.accessoryMgr = c.accMgr
}

func (c *controllerTestSuite) setupProCtl() {
	c.proCtl = &projecttesting.Controller{}
	c.ctl.proCtl = c.proCtl
}

func (c *controllerTestSuite) TestAssembleArtifact() {
	c.setupTagCtl()
	c.setupLabelMgr()
	c.setupAccMgr()

	art := &artifact.Artifact{
		ID:             1,
		Digest:         "sha256:123",
		RepositoryName: "library/hello-world",
	}
	option := &Option{
		WithTag: true,
		TagOption: &tag.Option{
			WithImmutableStatus: false,
		},
		WithLabel:     true,
		WithAccessory: true,
	}
	tg := &tag.Tag{
		Tag: model_tag.Tag{
			ID:           1,
			RepositoryID: 1,
			ArtifactID:   1,
			Name:         "latest",
			PushTime:     time.Now(),
			PullTime:     time.Now(),
		},
	}
	c.tagCtl.On("List").Return([]*tag.Tag{tg}, nil)
	ctx := lib.WithAPIVersion(nil, "2.0")
	lb := &model.Label{
		ID:   1,
		Name: "label",
	}
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{
		lb,
	}, nil)
	acc := &basemodel.Default{
		Data: accessorymodel.AccessoryData{
			ID:                1,
			ArtifactID:        2,
			SubArtifactDigest: "sha256:123",
			Type:              accessorymodel.TypeCosignSignature,
		},
	}
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{
		acc,
	}, nil)
	artifact := c.ctl.assembleArtifact(ctx, art, option)
	c.Require().NotNil(artifact)
	c.Equal(art.ID, artifact.ID)
	c.Equal(icon.DigestOfIconDefault, artifact.Icon)
	c.Contains(artifact.Tags, tg)
	c.Contains(artifact.Labels, lb)
	c.Contains(artifact.Accessories, acc)
	// TODO check other fields of option
}

func (c *controllerTestSuite) TestPopulateIcon() {
	cases := []struct {
		art *artifact.Artifact
		ico string
	}{
		{
			art: &artifact.Artifact{
				ID:     1,
				Digest: "sha256:123",
				Type:   image.ArtifactTypeImage,
			},
			ico: icon.DigestOfIconImage,
		},
		{
			art: &artifact.Artifact{
				ID:     2,
				Digest: "sha256:456",
				Type:   cnab.ArtifactTypeCNAB,
			},
			ico: icon.DigestOfIconCNAB,
		},
		{
			art: &artifact.Artifact{
				ID:     3,
				Digest: "sha256:1234",
				Type:   chart.ArtifactTypeChart,
			},
			ico: icon.DigestOfIconChart,
		},
		{
			art: &artifact.Artifact{
				ID:     4,
				Digest: "sha256:1234",
				Type:   "other",
			},
			ico: icon.DigestOfIconDefault,
		},
		{
			art: &artifact.Artifact{
				ID:     5,
				Digest: "sha256:2345",
				Type:   image.ArtifactTypeImage,
				Icon:   "sha256:abcd",
			},
			ico: "sha256:abcd",
		},
	}
	for _, cs := range cases {
		a := &Artifact{
			Artifact: *cs.art,
		}
		c.ctl.populateIcon(a)
		c.Equal(cs.ico, a.Icon)
	}
}

func (c *controllerTestSuite) TestEnsureArtifact() {
	digest := "sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180"

	// the artifact already exists
	c.setupArtMgr()
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID: 1,
	}, nil)
	created, art, err := c.ctl.ensureArtifact(orm.NewContext(nil, &ormtesting.FakeOrmer{}), "library/hello-world", digest)
	c.Require().Nil(err)
	c.False(created)
	c.Equal(int64(1), art.ID)

	// reset the mock
	c.SetupTest()
	c.setupRepoMgr()
	c.setupArtMgr()
	c.setupAbstractor()

	// the artifact doesn't exist
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		ProjectID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.artMgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	c.abstractor.On("AbstractMetadata").Return(nil)
	created, art, err = c.ctl.ensureArtifact(orm.NewContext(nil, &ormtesting.FakeOrmer{}), "library/hello-world", digest)
	c.Require().Nil(err)
	c.True(created)
	c.Equal(int64(1), art.ID)

	// reset the mock
	c.SetupTest()
	c.setupRepoMgr()
	c.setupArtMgr()
	c.setupAbstractor()

	// the artifact doesn't exist and get a conflict error on creating the artifact and fail to get again
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		ProjectID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.artMgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), errors.ConflictError(nil))
	c.abstractor.On("AbstractMetadata").Return(nil)
	created, art, err = c.ctl.ensureArtifact(orm.NewContext(nil, &ormtesting.FakeOrmer{}), "library/hello-world", digest)
	c.Require().Error(err, errors.NotFoundError(nil))
	c.False(created)
	c.Require().Nil(art)

	// reset the mock
	c.SetupTest()
	c.setupRepoMgr()
	c.setupArtMgr()
	c.setupAccMgr()
	c.setupAbstractor()

	// the artifact doesn't exist and includes a pending attestation accessory candidate
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		ProjectID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.artMgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	c.accMgr.On("Ensure", mock.Anything,
		"sha256:cad250bb95ea402adf4f687cc7d6747ecf0de875e6d6117f74437893964903df",
		"library/hello-world",
		int64(2),
		int64(3),
		int64(4),
		"sha256:44401ce7f2bf39029d0d56f095374b7f344e1986c8b4970ef4f4fdb98e3f7220",
		accessorymodel.TypeInTotoAttestation,
	).Return(nil).Once()
	c.ctl.abstractor = stubAbstractor{
		fn: func(_ context.Context, art *artifact.Artifact) error {
			art.AccessoryCandidates = []*artifact.AccessoryCandidate{{
				ArtifactID:        3,
				SubArtifactID:     2,
				SubArtifactRepo:   "library/hello-world",
				SubArtifactDigest: "sha256:cad250bb95ea402adf4f687cc7d6747ecf0de875e6d6117f74437893964903df",
				Digest:            "sha256:44401ce7f2bf39029d0d56f095374b7f344e1986c8b4970ef4f4fdb98e3f7220",
				Size:              4,
				Type:              accessorymodel.TypeInTotoAttestation,
			}}
			return nil
		},
	}
	created, art, err = c.ctl.ensureArtifact(orm.NewContext(nil, &ormtesting.FakeOrmer{}), "library/hello-world", digest)
	c.Require().Nil(err)
	c.True(created)
	c.Equal(int64(1), art.ID)
	c.accMgr.AssertExpectations(c.T())
}

func (c *controllerTestSuite) TestEnsure() {
	c.setupRepoMgr()
	c.setupArtMgr()
	c.setupAbstractor()
	c.setupTagCtl()
	c.setupAccMgr()
	c.setupProCtl()

	digest := "sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180"

	// both the artifact and the tag don't exist
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		ProjectID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.artMgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	c.abstractor.On("AbstractMetadata").Return(nil)
	c.tagCtl.On("Ensure").Return(int64(1), nil)
	c.accMgr.On("Ensure").Return(nil)
	c.proCtl.On("GetByName", mock.Anything, mock.Anything).Return(&projectModel.Project{ProjectID: 1, Name: "library", RegistryID: 0}, nil)
	_, id, err := c.ctl.Ensure(orm.NewContext(nil, &ormtesting.FakeOrmer{}), "library/hello-world", digest, &ArtOption{
		Tags: []string{"latest"},
	})
	c.Require().Nil(err)
	c.repoMgr.AssertExpectations(c.T())
	c.artMgr.AssertExpectations(c.T())
	c.tagCtl.AssertExpectations(c.T())
	c.abstractor.AssertExpectations(c.T())
	c.Equal(int64(1), id)
}

func (c *controllerTestSuite) TestCount() {
	c.setupArtMgr()
	c.artMgr.On("Count", mock.Anything, mock.Anything).Return(int64(1), nil)
	total, err := c.ctl.Count(nil, nil)
	c.Require().Nil(err)
	c.Equal(int64(1), total)
}

func (c *controllerTestSuite) TestList() {
	c.setupArtMgr()
	c.setupTagCtl()
	c.setupRepoMgr()
	c.setupAccMgr()

	query := &q.Query{}
	option := &Option{
		WithTag:       true,
		WithAccessory: true,
	}
	c.artMgr.On("List", mock.Anything, mock.Anything).Return([]*artifact.Artifact{
		{
			ID:           1,
			RepositoryID: 1,
		},
	}, nil)
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID:           1,
				RepositoryID: 1,
				ArtifactID:   1,
				Name:         "latest",
			},
		},
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		Name: "library/hello-world",
	}, nil)
	c.repoMgr.On("List", mock.Anything, mock.Anything).Return([]*repomodel.RepoRecord{
		{RepositoryID: 1, Name: "library/hello-world"},
	}, nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{}, nil)
	artifacts, err := c.ctl.List(nil, query, option)
	c.Require().Nil(err)
	c.Require().Len(artifacts, 1)
	c.Equal(int64(1), artifacts[0].ID)
	c.Require().Len(artifacts[0].Tags, 1)
	c.Equal(int64(1), artifacts[0].Tags[0].ID)
	c.Equal(0, len(artifacts[0].Accessories))
}

func (c *controllerTestSuite) TestListWithLatest() {
	c.setupArtMgr()
	c.setupTagCtl()
	c.setupRepoMgr()
	c.setupAccMgr()

	query := &q.Query{}
	option := &Option{
		WithTag:       true,
		WithAccessory: true,
	}
	c.artMgr.On("ListWithLatest", mock.Anything, mock.Anything).Return([]*artifact.Artifact{
		{
			ID:           1,
			RepositoryID: 1,
		},
	}, nil)
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID:           1,
				RepositoryID: 1,
				ArtifactID:   1,
				Name:         "latest",
			},
		},
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		Name: "library/hello-world",
	}, nil)
	c.repoMgr.On("List", mock.Anything, mock.Anything).Return([]*repomodel.RepoRecord{
		{RepositoryID: 1, Name: "library/hello-world"},
	}, nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{}, nil)
	artifacts, err := c.ctl.ListWithLatest(nil, query, option)
	c.Require().Nil(err)
	c.Require().Len(artifacts, 1)
	c.Equal(int64(1), artifacts[0].ID)
	c.Require().Len(artifacts[0].Tags, 1)
	c.Equal(int64(1), artifacts[0].Tags[0].ID)
	c.Equal(0, len(artifacts[0].Accessories))
}

func (c *controllerTestSuite) TestGet() {
	c.setupArtMgr()
	c.setupRepoMgr()
	c.artMgr.On("Get", mock.Anything, mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID:           1,
		RepositoryID: 1,
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	art, err := c.ctl.Get(nil, 1, nil)
	c.Require().Nil(err)
	c.Require().NotNil(art)
	c.Equal(int64(1), art.ID)
}

func (c *controllerTestSuite) TestGetByDigest() {
	c.setupRepoMgr()
	c.setupArtMgr()

	// not found
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	art, err := c.ctl.getByDigest(nil, "library/hello-world",
		"sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180", nil)
	c.Require().NotNil(err)
	c.True(errors.IsErr(err, errors.NotFoundCode))

	// reset the mock
	c.SetupTest()
	c.setupRepoMgr()
	c.setupArtMgr()

	// success
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID:           1,
		RepositoryID: 1,
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	art, err = c.ctl.getByDigest(nil, "library/hello-world",
		"sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180", nil)
	c.Require().Nil(err)
	c.Require().NotNil(art)
	c.Equal(int64(1), art.ID)
}

func (c *controllerTestSuite) TestGetByTag() {
	c.setupRepoMgr()
	c.setupTagCtl()

	// not found
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
	}, nil)
	c.tagCtl.On("List").Return(nil, nil)
	art, err := c.ctl.getByTag(nil, "library/hello-world", "latest", nil)
	c.Require().NotNil(err)
	c.True(errors.IsErr(err, errors.NotFoundCode))

	// reset the mock
	c.SetupTest()
	c.setupRepoMgr()
	c.setupTagCtl()
	c.setupArtMgr()

	// success
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
	}, nil)
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID:           1,
				RepositoryID: 1,
				Name:         "latest",
				ArtifactID:   1,
			},
		},
	}, nil)
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID: 1,
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	art, err = c.ctl.getByTag(nil, "library/hello-world", "latest", nil)
	c.Require().Nil(err)
	c.Require().NotNil(art)
	c.Equal(int64(1), art.ID)
}

func (c *controllerTestSuite) TestGetByReference() {
	c.setupRepoMgr()
	c.setupArtMgr()

	// reference is digest
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
	}, nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID:           1,
		RepositoryID: 1,
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	art, err := c.ctl.GetByReference(nil, "library/hello-world",
		"sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180", nil)
	c.Require().Nil(err)
	c.Require().NotNil(art)
	c.Equal(int64(1), art.ID)

	// reset the mock
	c.SetupTest()
	c.setupRepoMgr()
	c.setupTagCtl()
	c.setupArtMgr()

	// reference is tag
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
	}, nil)
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID:           1,
				RepositoryID: 1,
				Name:         "latest",
				ArtifactID:   1,
			},
		},
	}, nil)
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID: 1,
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	art, err = c.ctl.GetByReference(nil, "library/hello-world", "latest", nil)
	c.Require().Nil(err)
	c.Require().NotNil(art)
	c.Equal(int64(1), art.ID)
}

func (c *controllerTestSuite) TestDeleteDeeply() {
	// root artifact and doesn't exist
	c.setupArtMgr()
	c.setupAccMgr()
	c.setupLabelMgr()
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{}, nil)
	err := c.ctl.deleteDeeply(orm.NewContext(nil, &ormtesting.FakeOrmer{}), 1, true, false)
	c.Require().NotNil(err)
	c.Assert().True(errors.IsErr(err, errors.NotFoundCode))

	// reset the mock
	c.SetupTest()
	c.setupArtMgr()
	c.setupAccMgr()
	c.setupLabelMgr()

	// child artifact and doesn't exist
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{}, nil)
	err = c.ctl.deleteDeeply(orm.NewContext(nil, &ormtesting.FakeOrmer{}), 1, false, false)
	c.Require().Nil(err)

	// reset the mock
	c.SetupTest()
	c.setupArtMgr()
	c.setupTagCtl()
	c.setupRepoMgr()
	c.setupArtrashMgr()
	c.setupAccMgr()
	c.setupLabelMgr()

	// child artifact and contains tags
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{ID: 1}, nil)
	c.artMgr.On("Delete", mock.Anything, mock.Anything).Return(nil)
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID: 1,
			},
		},
	}, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	c.artrashMgr.On("Create", mock.Anything, mock.Anything).Return(int64(0), nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{}, nil)
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{}, nil)
	err = c.ctl.deleteDeeply(orm.NewContext(nil, &ormtesting.FakeOrmer{}), 1, false, false)
	c.Require().Nil(err)

	// reset the mock
	c.SetupTest()
	c.setupArtMgr()
	c.setupTagCtl()
	c.setupRepoMgr()
	c.setupAccMgr()
	c.setupLabelMgr()

	// root artifact is referenced by other artifacts
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{ID: 1}, nil)
	c.tagCtl.On("List").Return(nil, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{
		{
			ID: 1,
		},
	}, nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{}, nil)
	err = c.ctl.deleteDeeply(orm.NewContext(nil, &ormtesting.FakeOrmer{}), 1, true, false)
	c.Require().NotNil(err)

	// reset the mock
	c.SetupTest()
	c.setupArtMgr()
	c.setupTagCtl()
	c.setupRepoMgr()
	c.setupAccMgr()
	c.setupLabelMgr()

	// child artifact contains no tag but referenced by other artifacts
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{ID: 1}, nil)
	c.tagCtl.On("List").Return(nil, nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{
		{
			ID: 1,
		},
	}, nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{}, nil)
	err = c.ctl.deleteDeeply(nil, 1, false, false)
	c.Require().Nil(err)

	// reset the mock
	c.SetupTest()
	c.setupArtMgr()
	c.setupTagCtl()
	c.setupLabelMgr()
	c.setupAccMgr()
	c.setupBlobMgr()
	c.setupRepoMgr()
	c.setupArtrashMgr()

	// accessory contains tag
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{ID: 1}, nil)
	c.artMgr.On("Delete", mock.Anything, mock.Anything).Return(nil)
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID: 1,
			},
		},
	}, nil)
	c.tagCtl.On("DeleteTags", mock.Anything, mock.Anything).Return(nil)
	c.labelMgr.On("RemoveAllFrom", mock.Anything, mock.Anything).Return(nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{}, nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.accMgr.On("DeleteAccessories", mock.Anything, mock.Anything).Return(nil)
	c.blobMgr.On("List", mock.Anything, mock.Anything).Return(nil, nil)
	c.blobMgr.On("CleanupAssociationsForProject", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{}, nil)
	c.artrashMgr.On("Create", mock.Anything, mock.Anything).Return(int64(0), nil)
	c.labelMgr.On("ListByArtifact", mock.Anything, mock.Anything).Return([]*model.Label{}, nil)
	err = c.ctl.deleteDeeply(orm.NewContext(nil, &ormtesting.FakeOrmer{}), 1, true, true)
	c.Require().Nil(err)

}

func (c *controllerTestSuite) TestCopy() {
	c.setupArtMgr()
	c.setupProCtl()
	c.setupRepoMgr()
	c.setupTagCtl()
	c.setupAccMgr()
	c.setupAbstractor()
	c.setupRegCli()

	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID:     1,
		Digest: "sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180",
	}, nil)
	c.proCtl.On("GetByName", mock.Anything, mock.Anything).Return(&projectModel.Project{ProjectID: 1, Name: "library", RegistryID: 0}, nil)
	c.repoMgr.On("GetByName", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
		Name:         "library/hello-world",
	}, nil)
	c.artMgr.On("Count", mock.Anything, mock.Anything).Return(int64(0), nil)
	c.artMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(nil, errors.NotFoundError(nil))
	c.tagCtl.On("List").Return([]*tag.Tag{
		{
			Tag: model_tag.Tag{
				ID:   1,
				Name: "latest",
			},
		},
	}, nil)
	acc := &basemodel.Default{
		Data: accessorymodel.AccessoryData{
			ID:                1,
			ArtifactID:        2,
			SubArtifactDigest: "sha256:418fb88ec412e340cdbef913b8ca1bbe8f9e8dc705f9617414c1f2c8db980180",
			Type:              accessorymodel.TypeCosignSignature,
		},
	}
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{
		acc,
	}, nil)
	c.tagCtl.On("Update").Return(nil)
	c.repoMgr.On("Get", mock.Anything, mock.Anything).Return(&repomodel.RepoRecord{
		RepositoryID: 1,
		Name:         "library/hello-world",
	}, nil)
	c.abstractor.On("AbstractMetadata").Return(nil)
	c.artMgr.On("Create", mock.Anything, mock.Anything).Return(int64(1), nil)
	c.regCli.On("Copy", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	c.tagCtl.On("Ensure").Return(int64(1), nil)
	c.accMgr.On("Ensure", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	_, err := c.ctl.Copy(orm.NewContext(nil, &ormtesting.FakeOrmer{}), "library/hello-world", "latest", "library/hello-world2")
	c.Require().Nil(err)
}

func (c *controllerTestSuite) TestUpdatePullTime() {
	c.setupTagCtl()
	c.setupArtMgr()

	// artifact ID and tag ID matches
	c.tagCtl.On("Get").Return(&tag.Tag{
		Tag: model_tag.Tag{
			ID:         1,
			ArtifactID: 1,
		},
	}, nil)
	c.tagCtl.On("Update").Return(nil)
	c.artMgr.On("UpdatePullTime", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := c.ctl.UpdatePullTime(nil, 1, 1, time.Now())
	c.Require().Nil(err)
	c.artMgr.AssertExpectations(c.T())
	c.tagCtl.AssertExpectations(c.T())

	// reset the mock
	c.SetupTest()
	c.setupTagCtl()
	c.setupArtMgr()

	// artifact ID and tag ID doesn't match
	c.tagCtl.On("Get").Return(&tag.Tag{
		Tag: model_tag.Tag{
			ID:         1,
			ArtifactID: 2,
		},
	}, nil)
	c.artMgr.On("UpdatePullTime", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = c.ctl.UpdatePullTime(nil, 1, 1, time.Now())
	c.Require().NotNil(err)
	c.tagCtl.AssertExpectations(c.T())

	// if no tag, should not update tag
	c.SetupTest()
	c.setupTagCtl()
	c.setupArtMgr()
	c.tagCtl.On("Update").Return(nil)
	c.artMgr.On("UpdatePullTime", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = c.ctl.UpdatePullTime(nil, 1, 0, time.Now())
	c.Require().Nil(err)
	c.artMgr.AssertExpectations(c.T())
	// should not call tag Update
	c.tagCtl.AssertNotCalled(c.T(), "Update")
}

func (c *controllerTestSuite) TestGetAddition() {
	c.setupArtMgr()
	c.artMgr.On("Get", mock.Anything, mock.Anything).Return(&artifact.Artifact{}, nil)
	_, err := c.ctl.GetAddition(nil, 1, "addition")
	c.Require().NotNil(err)
}

func (c *controllerTestSuite) TestAddTo() {
	c.setupLabelMgr()
	c.labelMgr.On("AddTo", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := c.ctl.AddLabel(context.Background(), 1, 1)
	c.Require().Nil(err)
}

func (c *controllerTestSuite) TestRemoveFrom() {
	c.setupLabelMgr()
	c.labelMgr.On("RemoveFrom", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err := c.ctl.RemoveLabel(nil, 1, 1)
	c.Require().Nil(err)
}

func (c *controllerTestSuite) TestWalk() {
	c.setupArtMgr()
	c.setupAccMgr()

	c.artMgr.On("List", mock.Anything, mock.Anything).Return([]*artifact.Artifact{
		{Digest: "d1", ManifestMediaType: v1.MediaTypeImageManifest},
		{Digest: "d2", ManifestMediaType: v1.MediaTypeImageManifest},
	}, nil)
	c.accMgr.On("List", mock.Anything, mock.Anything).Return([]accessorymodel.Accessory{}, nil)
	c.artMgr.On("ListReferences", mock.Anything, mock.Anything).Return([]*artifact.Reference{}, nil)

	{
		root := &Artifact{}

		var n int
		c.ctl.Walk(context.TODO(), root, func(a *Artifact) error {
			n++
			return nil
		}, nil)

		c.Equal(1, n)
	}

	{
		root := &Artifact{}
		root.References = []*artifact.Reference{
			{ParentID: 1, ChildID: 2},
			{ParentID: 1, ChildID: 3},
		}

		var n int
		c.ctl.Walk(context.TODO(), root, func(a *Artifact) error {
			n++
			return nil
		}, nil)

		c.Equal(3, n)
	}
}

func (c *controllerTestSuite) TestIsInto() {
	c.setupBlobMgr()

	blobs := []*models.Blob{
		{Digest: "sha256:00000", ContentType: "application/vnd.oci.image.manifest.v1+json"},
		{Digest: "sha256:22222", ContentType: "application/vnd.oci.image.config.v1+json"},
		{Digest: "sha256:11111", ContentType: "application/vnd.in-toto+json"},
	}
	c.blobMgr.On("GetByArt", mock.Anything, mock.Anything).Return(blobs, nil).Once()
	isInto, err := c.ctl.HasUnscannableLayer(context.Background(), "sha256: 77777")
	c.Nil(err)
	c.True(isInto)

	blobs2 := []*models.Blob{
		{Digest: "sha256:00000", ContentType: "application/vnd.oci.image.manifest.v1+json"},
		{Digest: "sha256:22222", ContentType: "application/vnd.oci.image.config.v1+json"},
		{Digest: "sha256:11111", ContentType: "application/vnd.oci.image.layer.v1.tar+gzip"},
	}

	c.blobMgr.On("GetByArt", mock.Anything, mock.Anything).Return(blobs2, nil).Once()
	isInto2, err := c.ctl.HasUnscannableLayer(context.Background(), "sha256: 8888")
	c.Nil(err)
	c.False(isInto2)
}

func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, &controllerTestSuite{})
}
