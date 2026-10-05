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

package gc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/artifactrash/model"
	blobModels "github.com/goharbor/harbor/src/pkg/blob/models"
	"github.com/goharbor/harbor/src/pkg/registry"
	"github.com/goharbor/harbor/src/pkg/registry/interceptor/readonly"
	mockjobservice "github.com/goharbor/harbor/src/testing/jobservice"
	"github.com/goharbor/harbor/src/testing/mock"
	trashtesting "github.com/goharbor/harbor/src/testing/pkg/artifactrash"
	blobtesting "github.com/goharbor/harbor/src/testing/pkg/blob"
	registrytesting "github.com/goharbor/harbor/src/testing/pkg/registry"
	regctltesting "github.com/goharbor/harbor/src/testing/registryctl"
)

const (
	sweepRepo   = "library/app"
	sweepDigest = "sha256:aa11111111111111111111111111111111111111111111111111111111111111"
)

type sweepMocks struct {
	ctx     *mockjobservice.MockJobContext
	blobMgr *blobtesting.Manager
	trash   *trashtesting.Manager
	regctl  *regctltesting.Client
	regCli  *registrytesting.Client
}

func newSweep(t *testing.T, contentType string, trashed, deleteTag bool) (*GarbageCollector, *sweepMocks) {
	t.Helper()
	m := &sweepMocks{
		ctx:     &mockjobservice.MockJobContext{},
		blobMgr: &blobtesting.Manager{},
		trash:   &trashtesting.Manager{},
		regctl:  &regctltesting.Client{},
		regCli:  &registrytesting.Client{},
	}
	m.ctx.On("GetLogger").Return(&mockjobservice.MockJobLogger{})
	m.ctx.On("OPCommand").Return(job.NilCommand, false)
	mock.OnAnything(m.ctx, "Checkin").Return(nil)
	mock.OnAnything(m.blobMgr, "UpdateBlobStatus").Return(int64(1), nil)
	mock.OnAnything(m.blobMgr, "Delete").Return(nil)
	mock.OnAnything(m.blobMgr, "CleanupAssociationsForArtifact").Return(nil)
	mock.OnAnything(m.trash, "Delete").Return(nil)

	orig := registry.Cli
	registry.Cli = m.regCli
	t.Cleanup(func() { registry.Cli = orig })

	gc := &GarbageCollector{
		blobMgr:           m.blobMgr,
		artrashMgr:        m.trash,
		registryCtlClient: m.regctl,
		deleteTag:         deleteTag,
		workers:           1,
		trashedArts:       map[string][]model.ArtifactTrash{},
		deleteSet: []*blobModels.Blob{
			{ID: 1, Digest: sweepDigest, ContentType: contentType, Size: 10},
		},
	}
	if trashed {
		gc.trashedArts[sweepDigest] = []model.ArtifactTrash{
			{ID: 7, RepositoryName: sweepRepo, Digest: sweepDigest},
		}
	}
	return gc, m
}

// Which storage calls sweep makes per media type. Schema1 rows can predate the
// v3 registry, so they must keep going down the manifest path.
func TestSweepRoutesByMediaType(t *testing.T) {
	manifestTypes := []string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.docker.distribution.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v1+prettyjws",
	}
	for _, ct := range manifestTypes {
		t.Run("manifest/"+ct, func(t *testing.T) {
			gc, m := newSweep(t, ct, true, false)
			m.regctl.On("DeleteManifest", sweepRepo, sweepDigest).Return(nil).Once()
			m.regctl.On("DeleteBlob", sweepDigest).Return(nil).Once()

			require.NoError(t, gc.sweep(m.ctx))
			m.regctl.AssertExpectations(t)
			m.blobMgr.AssertCalled(t, "CleanupAssociationsForArtifact", mock.Anything, sweepDigest)
			m.trash.AssertCalled(t, "Delete", mock.Anything, int64(7))
			m.blobMgr.AssertCalled(t, "Delete", mock.Anything, int64(1))
		})
		t.Run("manifest+delete_tag/"+ct, func(t *testing.T) {
			gc, m := newSweep(t, ct, true, true)
			m.regCli.On("ManifestExist", sweepRepo, sweepDigest).Return(true, nil, nil).Once()
			m.regCli.On("DeleteManifest", sweepRepo, sweepDigest).Return(nil).Once()
			m.regctl.On("DeleteManifest", sweepRepo, sweepDigest).Return(nil).Once()
			m.regctl.On("DeleteBlob", sweepDigest).Return(nil).Once()

			require.NoError(t, gc.sweep(m.ctx))
			m.regCli.AssertExpectations(t)
			m.regctl.AssertExpectations(t)
		})
	}

	blobTypes := []string{
		"application/vnd.docker.image.rootfs.diff.tar.gzip",
		"application/vnd.docker.container.image.v1+json",
		"application/vnd.oci.image.layer.v1.tar+gzip",
		"application/vnd.oci.image.config.v1+json",
		"application/octet-stream",
		"",
	}
	for _, ct := range blobTypes {
		t.Run("blob/"+ct, func(t *testing.T) {
			// even if an artifact with this digest sits in the trash, a non-manifest
			// blob must never trigger a manifest/tag deletion
			gc, m := newSweep(t, ct, true, true)
			m.regctl.On("DeleteBlob", sweepDigest).Return(nil).Once()

			require.NoError(t, gc.sweep(m.ctx))
			m.regctl.AssertExpectations(t)
			m.regctl.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything)
			m.regCli.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything)
			m.blobMgr.AssertCalled(t, "Delete", mock.Anything, int64(1))
		})
	}

	t.Run("foreign layer only drops the DB row", func(t *testing.T) {
		gc, m := newSweep(t, "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip", false, true)

		require.NoError(t, gc.sweep(m.ctx))
		m.regctl.AssertNotCalled(t, "DeleteBlob", mock.Anything)
		m.regctl.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything)
		m.blobMgr.AssertCalled(t, "Delete", mock.Anything, int64(1))
	})

	t.Run("untrashed manifest is swept as a plain blob", func(t *testing.T) {
		gc, m := newSweep(t, "application/vnd.oci.image.manifest.v1+json", false, true)
		m.regctl.On("DeleteBlob", sweepDigest).Return(nil).Once()

		require.NoError(t, gc.sweep(m.ctx))
		m.regctl.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything)
		m.regctl.AssertExpectations(t)
	})
}

// A failed revision delete must leave the blob data and DB row in place.
func TestSweepKeepsBlobWhenManifestDeleteFails(t *testing.T) {
	gc, m := newSweep(t, "application/vnd.docker.distribution.manifest.v2+json", true, false)
	m.regctl.On("DeleteManifest", sweepRepo, sweepDigest).Return(readonly.Err).Once()

	assert.ErrorIs(t, gc.sweep(m.ctx), readonly.Err)
	m.regctl.AssertNotCalled(t, "DeleteBlob", mock.Anything)
	m.blobMgr.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	m.blobMgr.AssertNotCalled(t, "CleanupAssociationsForArtifact", mock.Anything, mock.Anything)
}

func TestSweepKeepsBlobWhenV2DeleteFails(t *testing.T) {
	gc, m := newSweep(t, "application/vnd.oci.image.manifest.v1+json", true, true)
	m.regCli.On("ManifestExist", sweepRepo, sweepDigest).Return(true, nil, nil).Once()
	m.regCli.On("DeleteManifest", sweepRepo, sweepDigest).Return(readonly.Err).Once()

	assert.ErrorIs(t, gc.sweep(m.ctx), readonly.Err)
	m.regctl.AssertNotCalled(t, "DeleteManifest", mock.Anything, mock.Anything)
	m.regctl.AssertNotCalled(t, "DeleteBlob", mock.Anything)
	m.blobMgr.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

// registryctl answers 404 for content already gone; sweep must treat that as done.
func TestSweepTreatsNotFoundAsDeleted(t *testing.T) {
	notFound := errors.New(nil).WithCode(errors.NotFoundCode)
	gc, m := newSweep(t, "application/vnd.oci.image.manifest.v1+json", true, false)
	m.regctl.On("DeleteManifest", sweepRepo, sweepDigest).Return(notFound).Once()
	m.regctl.On("DeleteBlob", sweepDigest).Return(notFound).Once()

	require.NoError(t, gc.sweep(m.ctx))
	m.regctl.AssertExpectations(t)
	m.blobMgr.AssertCalled(t, "Delete", mock.Anything, int64(1))
}
