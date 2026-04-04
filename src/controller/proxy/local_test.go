//  Copyright Project Harbor Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package proxy

import (
	"context"
	"testing"
	"time"

	distribution2 "github.com/docker/distribution"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact"
	ctltag "github.com/goharbor/harbor/src/controller/tag"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	modeltag "github.com/goharbor/harbor/src/pkg/tag/model/tag"
	testregistry "github.com/goharbor/harbor/src/testing/moq/pkg/registry"
)

type mockManifest struct {
	ReferencesFunc func() []distribution2.Descriptor
	PayloadFunc    func() (mediaType string, payload []byte, err error)
}

func (m *mockManifest) References() []distribution2.Descriptor {
	if m.ReferencesFunc != nil {
		return m.ReferencesFunc()
	}
	return nil
}

func (m *mockManifest) Payload() (mediaType string, payload []byte, err error) {
	if m.PayloadFunc != nil {
		return m.PayloadFunc()
	}
	return "", nil, nil
}

type artifactControllerMock struct {
	GetByReferenceFunc func(ctx context.Context, repository, reference string, option *artifact.Option) (arti *artifact.Artifact, err error)
	UpdatePullTimeFunc func(ctx context.Context, artifactID int64, tagID int64, t time.Time) error

	getByReferenceCalls []struct{}
	updatePullTimeCalls []struct{}
}

func (a *artifactControllerMock) GetByReference(ctx context.Context, repository, reference string, option *artifact.Option) (arti *artifact.Artifact, err error) {
	a.getByReferenceCalls = append(a.getByReferenceCalls, struct{}{})
	if a.GetByReferenceFunc != nil {
		return a.GetByReferenceFunc(ctx, repository, reference, option)
	}
	return &artifact.Artifact{}, nil
}

func (a *artifactControllerMock) UpdatePullTime(ctx context.Context, artifactID int64, tagID int64, t time.Time) error {
	a.updatePullTimeCalls = append(a.updatePullTimeCalls, struct{}{})
	if a.UpdatePullTimeFunc != nil {
		return a.UpdatePullTimeFunc(ctx, artifactID, tagID, t)
	}
	return nil
}

func (a *artifactControllerMock) GetByReferenceCalls() []struct{} {
	return a.getByReferenceCalls
}

func (a *artifactControllerMock) UpdatePullTimeCalls() []struct{} {
	return a.updatePullTimeCalls
}

type localHelperTestSuite struct {
	suite.Suite
	registryClient *testregistry.Client
	local          *localHelper
	artCtl         *artifactControllerMock
}

func (lh *localHelperTestSuite) SetupTest() {
	lh.registryClient = &testregistry.Client{}
	lh.artCtl = &artifactControllerMock{}
	lh.local = &localHelper{registry: lh.registryClient, artifactCtl: lh.artCtl}

}

func (lh *localHelperTestSuite) TestBlobExist_False() {
	repo := "library/hello-world"
	dig := "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
	art := lib.ArtifactInfo{Repository: repo, Digest: dig}
	ctx := context.Background()
	lh.registryClient.BlobExistFunc = func(_ string, _ string) (bool, error) {
		return false, nil
	}
	exist, err := lh.local.BlobExist(ctx, art)
	lh.Require().Nil(err)
	lh.Assert().Equal(false, exist)
}
func (lh *localHelperTestSuite) TestBlobExist_True() {
	repo := "library/hello-world"
	dig := "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
	art := lib.ArtifactInfo{Repository: repo, Digest: dig}
	ctx := context.Background()
	lh.registryClient.BlobExistFunc = func(_ string, _ string) (bool, error) {
		return true, nil
	}
	exist, err := lh.local.BlobExist(ctx, art)
	lh.Require().Nil(err)
	lh.Assert().Equal(true, exist)
}

func (lh *localHelperTestSuite) TestPushManifest() {
	dig := "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f"
	lh.registryClient.PushManifestFunc = func(_ string, _ string, _ string, _ []byte) (string, error) {
		return dig, nil
	}
	manifest := &mockManifest{}
	manifest.PayloadFunc = func() (string, []byte, error) {
		return schema2.MediaTypeManifest, []byte("example"), nil
	}
	err := lh.local.PushManifest("library/hello-world", "", manifest)
	lh.Require().Nil(err)
}

func (lh *localHelperTestSuite) TestCheckDependencies_Fail() {
	ctx := context.Background()
	manifest := &mockManifest{}
	refs := []distribution2.Descriptor{
		{Digest: "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"},
		{Digest: "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"},
	}
	manifest.ReferencesFunc = func() []distribution2.Descriptor {
		return refs
	}
	lh.registryClient.BlobExistFunc = func(_ string, _ string) (bool, error) {
		return false, nil
	}
	ret := lh.local.CheckDependencies(ctx, "library/hello-world", manifest)
	lh.Assert().Equal(len(ret), 2)
}

func (lh *localHelperTestSuite) TestCheckDependencies_Suc() {
	ctx := context.Background()
	manifest := &mockManifest{}
	refs := []distribution2.Descriptor{
		{Digest: "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"},
		{Digest: "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"},
	}
	manifest.ReferencesFunc = func() []distribution2.Descriptor {
		return refs
	}
	lh.registryClient.BlobExistFunc = func(_ string, _ string) (bool, error) {
		return true, nil
	}
	ret := lh.local.CheckDependencies(ctx, "library/hello-world", manifest)
	lh.Assert().Equal(len(ret), 0)
}

func (lh *localHelperTestSuite) TestManifestExist() {
	ctx := context.Background()
	dig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	ar := &artifact.Artifact{}
	lh.artCtl.GetByReferenceFunc = func(_ context.Context, _ string, _ string, _ *artifact.Option) (*artifact.Artifact, error) {
		return ar, nil
	}
	art := lib.ArtifactInfo{Repository: "library/hello-world", Digest: dig}
	a, err := lh.local.GetManifest(ctx, art)
	lh.Assert().Nil(err)
	lh.Assert().NotNil(a)
}

func (lh *localHelperTestSuite) TestUpdatePullTime_EmptyReference() {
	ctx := context.Background()
	// both Digest and Tag are empty — should return nil without calling any controller method
	art := lib.ArtifactInfo{Repository: "library/hello-world"}
	err := lh.local.UpdatePullTime(ctx, art)
	lh.Require().Nil(err)
	assert.Empty(lh.T(), lh.artCtl.GetByReferenceCalls())
	assert.Empty(lh.T(), lh.artCtl.UpdatePullTimeCalls())
}

func (lh *localHelperTestSuite) TestUpdatePullTime_ArtifactNotFound() {
	ctx := context.Background()
	dig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	art := lib.ArtifactInfo{Repository: "library/hello-world", Digest: dig}
	lh.artCtl.GetByReferenceFunc = func(_ context.Context, _ string, _ string, _ *artifact.Option) (*artifact.Artifact, error) {
		return nil, errors.NotFoundError(nil)
	}
	// Not-found is expected during proxy-cache races and should be treated as a no-op.
	err := lh.local.UpdatePullTime(ctx, art)
	lh.Require().Nil(err)
	assert.Empty(lh.T(), lh.artCtl.UpdatePullTimeCalls())
}

func (lh *localHelperTestSuite) TestUpdatePullTime_ByDigestNoTag() {
	ctx := context.Background()
	dig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	art := lib.ArtifactInfo{Repository: "library/hello-world", Digest: dig}
	a := &artifact.Artifact{}
	a.ID = 100
	lh.artCtl.GetByReferenceFunc = func(_ context.Context, _ string, _ string, _ *artifact.Option) (*artifact.Artifact, error) {
		return a, nil
	}
	lh.artCtl.UpdatePullTimeFunc = func(_ context.Context, artifactID int64, tagID int64, _ time.Time) error {
		assert.Equal(lh.T(), int64(100), artifactID)
		assert.Equal(lh.T(), int64(0), tagID)
		return nil
	}
	err := lh.local.UpdatePullTime(ctx, art)
	lh.Require().Nil(err)
	assert.NotEmpty(lh.T(), lh.artCtl.UpdatePullTimeCalls())
}

func (lh *localHelperTestSuite) TestUpdatePullTime_ByTagFound() {
	ctx := context.Background()
	tag := "latest"
	art := lib.ArtifactInfo{Repository: "library/hello-world", Tag: tag}
	a := &artifact.Artifact{}
	a.ID = 200
	a.Tags = []*ctltag.Tag{
		{Tag: modeltag.Tag{ID: 42, ArtifactID: 200, Name: tag}},
	}
	lh.artCtl.GetByReferenceFunc = func(_ context.Context, _ string, _ string, _ *artifact.Option) (*artifact.Artifact, error) {
		return a, nil
	}
	lh.artCtl.UpdatePullTimeFunc = func(_ context.Context, artifactID int64, tagID int64, _ time.Time) error {
		assert.Equal(lh.T(), int64(200), artifactID)
		assert.Equal(lh.T(), int64(42), tagID)
		return nil
	}
	err := lh.local.UpdatePullTime(ctx, art)
	lh.Require().Nil(err)
	assert.NotEmpty(lh.T(), lh.artCtl.UpdatePullTimeCalls())
}

func (lh *localHelperTestSuite) TestUpdatePullTime_ByTagNotFound() {
	ctx := context.Background()
	art := lib.ArtifactInfo{Repository: "library/hello-world", Tag: "v1.0"}
	a := &artifact.Artifact{}
	a.ID = 300
	a.Tags = []*ctltag.Tag{
		{Tag: modeltag.Tag{ID: 99, ArtifactID: 300, Name: "latest"}},
	}
	lh.artCtl.GetByReferenceFunc = func(_ context.Context, _ string, _ string, _ *artifact.Option) (*artifact.Artifact, error) {
		return a, nil
	}
	// tagID should be 0 because "v1.0" was not found in a.Tags
	lh.artCtl.UpdatePullTimeFunc = func(_ context.Context, artifactID int64, tagID int64, _ time.Time) error {
		assert.Equal(lh.T(), int64(300), artifactID)
		assert.Equal(lh.T(), int64(0), tagID)
		return nil
	}
	err := lh.local.UpdatePullTime(ctx, art)
	lh.Require().Nil(err)
	assert.NotEmpty(lh.T(), lh.artCtl.UpdatePullTimeCalls())
}

func TestLocalHelperTestSuite(t *testing.T) {
	suite.Run(t, &localHelperTestSuite{})
}
