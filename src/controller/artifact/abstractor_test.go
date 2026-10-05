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

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact/manifest"
	"github.com/goharbor/harbor/src/controller/artifact/processor"
	"github.com/goharbor/harbor/src/pkg/artifact"
	"github.com/goharbor/harbor/src/testing/mock"
	tart "github.com/goharbor/harbor/src/testing/pkg/artifact"
	tblob "github.com/goharbor/harbor/src/testing/pkg/blob"
	tpro "github.com/goharbor/harbor/src/testing/pkg/processor"
	"github.com/goharbor/harbor/src/testing/pkg/registry"
)

var (
	v2Manifest = `{
  "schemaVersion": 2,
  "mediaType": "application/vnd.docker.distribution.manifest.v2+json",
  "config": {
    "mediaType": "application/vnd.docker.container.image.v1+json",
    "size": 1510,
    "digest": "sha256:fce289e99eb9bca977dae136fbe2a82b6b7d4c372474c9235adc1741675f587e"
  },
  "layers": [
    {
      "mediaType": "application/vnd.docker.image.rootfs.diff.tar.gzip",
      "size": 977,
      "digest": "sha256:1b930d010525941c1d56ec53b97bd057a67ae1865eebf042686d2a2d18271ced"
    }
  ],
  "annotations": {
    "com.example.key1": "value1"
  }
}`
	OCIManifest = `{ 
   "schemaVersion": 2,
   "mediaType": "application/vnd.oci.image.manifest.v1+json",
   "config": {
      "mediaType": "application/vnd.example.config.v1+json",
      "digest": "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
      "size": 123
   },
   "layers": [
      {
         "mediaType": "application/vnd.example.data.v1.tar+gzip",
         "digest": "sha256:e258d248fda94c63753607f7c4494ee0fcbe92f1a76bfdac795c9d84101eb317",
         "size": 1234
      }
   ],
   "annotations": {
      "com.example.key1": "value1"
   }
}`
	OCIManifestWithArtifactType = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.oci.image.manifest.v1+json",
   "artifactType": "application/vnd.example+type",
   "config": {
      "mediaType": "application/vnd.example.config.v1+json",
      "digest": "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
      "size": 123
   },
   "layers": [
      {
         "mediaType": "application/vnd.example.data.v1.tar+gzip",
         "digest": "sha256:e258d248fda94c63753607f7c4494ee0fcbe92f1a76bfdac795c9d84101eb317",
         "size": 1234
      }
   ],
   "annotations": {
      "com.example.key1": "value1"
   }
}`
	OCIManifestWithEmptyConfig = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.oci.image.manifest.v1+json",
   "artifactType": "application/vnd.example+type",
   "config": {
     "mediaType": "application/vnd.oci.empty.v1+json",
     "digest": "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a",
     "size": 2
   },
   "layers": [
     {
       "mediaType": "application/vnd.example+type",
       "digest": "sha256:e258d248fda94c63753607f7c4494ee0fcbe92f1a76bfdac795c9d84101eb317",
       "size": 1234
     }
   ],
   "annotations": {
     "oci.opencontainers.image.created": "2023-01-02T03:04:05Z",
     "com.example.data": "payload"
   }
}`
	index = `{
  "schemaVersion": 2,
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 7143,
      "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
      "platform": {
        "architecture": "ppc64le",
        "os": "linux"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "size": 7682,
      "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270",
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      }
    }
  ],
  "annotations": {
    "com.example.key1": "value1"
  }
}`
	indexWithArtifactType = `{
   "schemaVersion": 2,
   "mediaType": "application/vnd.oci.image.index.v1+json",
   "artifactType": "application/vnd.food.stand",
   "manifests": [
     {
       "mediaType": "application/vnd.oci.image.manifest.v1+json",
       "size": 7143,
       "digest": "sha256:e692418e4cbaf90ca69d05a66403747baa33ee08806650b51fab815ad7fc331f",
       "platform": {
         "architecture": "ppc64le",
         "os": "linux"
       }
     },
     {
       "mediaType": "application/vnd.oci.image.manifest.v1+json",
       "size": 7682,
       "digest": "sha256:5b0bcabd1ed22e9fb1310cf6c2dec7cdef19f0ad69efa1f392e94a4333501270",
       "platform": {
         "architecture": "amd64",
         "os": "linux"
       }
     }
   ],
   "annotations": {
     "com.example.key1": "value1"
   }
 }`
)

type abstractorTestSuite struct {
	suite.Suite
	argMgr     *tart.Manager
	blobMgr    *tblob.Manager
	regCli     *registry.Client
	abstractor *abstractor
	processor  *tpro.Processor
}

func (a *abstractorTestSuite) SetupTest() {
	a.regCli = &registry.Client{}
	a.argMgr = &tart.Manager{}
	a.blobMgr = &tblob.Manager{}
	a.abstractor = &abstractor{
		regCli: a.regCli,
	}
	a.processor = &tpro.Processor{}
	// clear all registered processors
	processor.Registry = map[string]processor.Processor{}
	processor.Registry[schema2.MediaTypeImageConfig] = a.processor
	// replace the registered manifest abstractors with mock-backed ones so the
	// suite exercises them against its own managers instead of the globals
	// wired up by the manifest package's init()
	manifest.Registry = map[string]manifest.Abstractor{}
	a.Require().NoError(manifest.Register(manifest.NewV2(),
		v1.MediaTypeImageManifest, schema2.MediaTypeManifest))
	a.Require().NoError(manifest.Register(manifest.NewIndex(a.argMgr, a.regCli),
		v1.MediaTypeImageIndex, manifestlist.MediaTypeManifestList))
}

// docker manifest v2
func (a *abstractorTestSuite) TestAbstractMetadataOfV2Manifest() {
	manifest, _, err := distribution.UnmarshalManifest(schema2.MediaTypeManifest, []byte(v2Manifest))
	a.Require().Nil(err)
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(manifest, "", nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	a.processor.On("AbstractMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = a.abstractor.AbstractMetadata(nil, artifact)
	a.Require().Nil(err)
	a.Assert().Equal(int64(1), artifact.ID)
	a.Assert().Equal(schema2.MediaTypeManifest, artifact.ManifestMediaType)
	a.Assert().Equal(schema2.MediaTypeImageConfig, artifact.MediaType)
	a.Assert().Equal(int64(3043), artifact.Size)
	a.Require().Len(artifact.Annotations, 1)
	a.Equal("value1", artifact.Annotations["com.example.key1"])
}

// oci-spec v1
func (a *abstractorTestSuite) TestAbstractMetadataOfOCIManifest() {
	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(OCIManifest))
	a.Require().Nil(err)
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(manifest, "", nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	a.processor.On("AbstractMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = a.abstractor.AbstractMetadata(context.TODO(), artifact)
	a.Require().Nil(err)
	a.Assert().Equal(int64(1), artifact.ID)
	a.Assert().Equal(v1.MediaTypeImageManifest, artifact.ManifestMediaType)
	a.Assert().Equal("application/vnd.example.config.v1+json", artifact.MediaType)
	a.Assert().Equal("application/vnd.example.config.v1+json", artifact.ArtifactType)
	a.Assert().Equal(int64(1916), artifact.Size)
	a.Require().Len(artifact.Annotations, 1)
	a.Equal("value1", artifact.Annotations["com.example.key1"])
}

// oci-spec v1.1.0 with artifactType
func (a *abstractorTestSuite) TestAbstractMetadataOfOCIManifestWithArtifactType() {
	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(OCIManifestWithArtifactType))
	a.Require().Nil(err)
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(manifest, "", nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	a.processor.On("AbstractMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = a.abstractor.AbstractMetadata(context.TODO(), artifact)
	a.Require().Nil(err)
	a.Assert().Equal(int64(1), artifact.ID)
	a.Assert().Equal(v1.MediaTypeImageManifest, artifact.ManifestMediaType)
	a.Assert().Equal("application/vnd.example.config.v1+json", artifact.MediaType)
	a.Assert().Equal("application/vnd.example+type", artifact.ArtifactType)
	a.Assert().Equal(int64(1966), artifact.Size)
	a.Require().Len(artifact.Annotations, 1)
	a.Equal("value1", artifact.Annotations["com.example.key1"])
}

// empty config with artifactType
func (a *abstractorTestSuite) TestAbstractMetadataOfV2ManifestWithEmptyConfig() {
	// v1.MediaTypeImageManifest
	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(OCIManifestWithEmptyConfig))
	a.Require().Nil(err)
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(manifest, "", nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	a.processor.On("AbstractMetadata", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	err = a.abstractor.AbstractMetadata(context.TODO(), artifact)
	a.Require().Nil(err)
	a.Assert().Equal(int64(1), artifact.ID)
	a.Assert().Equal(v1.MediaTypeImageManifest, artifact.ManifestMediaType)
	a.Assert().Equal(v1.MediaTypeEmptyJSON, artifact.MediaType)
	a.Assert().Equal("application/vnd.example+type", artifact.ArtifactType)
	a.Assert().Equal(int64(1880), artifact.Size)
	a.Require().Len(artifact.Annotations, 2)
	a.Equal("payload", artifact.Annotations["com.example.data"])
}

// OCI index
func (a *abstractorTestSuite) TestAbstractMetadataOfIndex() {
	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeImageIndex, []byte(index))
	a.Require().Nil(err)
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(manifest, "", nil)
	a.argMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID:   2,
		Size: 10,
	}, nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	err = a.abstractor.AbstractMetadata(context.TODO(), artifact)
	a.Require().Nil(err)
	a.Assert().Equal(int64(1), artifact.ID)
	a.Assert().Equal(v1.MediaTypeImageIndex, artifact.ManifestMediaType)
	a.Assert().Equal(v1.MediaTypeImageIndex, artifact.MediaType)
	a.Assert().Equal("", artifact.ArtifactType)
	a.Assert().Equal(int64(668), artifact.Size)
	a.Require().Len(artifact.Annotations, 1)
	a.Assert().Equal("value1", artifact.Annotations["com.example.key1"])
	a.Len(artifact.References, 2)
}

func (a *abstractorTestSuite) TestAbstractMetadataOfIndexWithArtifactType() {
	manifest, _, err := distribution.UnmarshalManifest(v1.MediaTypeImageIndex, []byte(indexWithArtifactType))
	a.Require().Nil(err)
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(manifest, "", nil)
	a.argMgr.On("GetByDigest", mock.Anything, mock.Anything, mock.Anything).Return(&artifact.Artifact{
		ID:   2,
		Size: 10,
	}, nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	err = a.abstractor.AbstractMetadata(context.TODO(), artifact)
	a.Require().Nil(err)
	a.Assert().Equal(int64(1), artifact.ID)
	a.Assert().Equal(v1.MediaTypeImageIndex, artifact.ManifestMediaType)
	a.Assert().Equal(v1.MediaTypeImageIndex, artifact.MediaType)
	a.Assert().Equal("application/vnd.food.stand", artifact.ArtifactType)
	a.Assert().Equal(int64(801), artifact.Size)
	a.Require().Len(artifact.Annotations, 1)
	a.Assert().Equal("value1", artifact.Annotations["com.example.key1"])
	a.Len(artifact.References, 2)
}

type unknownManifest struct{}

func (u *unknownManifest) References() []distribution.Descriptor {
	return nil
}
func (u *unknownManifest) Payload() (mediaType string, payload []byte, err error) {
	return "unknown-manifest", nil, nil
}

// unknown
func (a *abstractorTestSuite) TestAbstractMetadataOfUnsupported() {
	a.regCli.On("PullManifest", mock.Anything, mock.Anything).Return(&unknownManifest{}, "", nil)
	artifact := &artifact.Artifact{
		ID: 1,
	}
	err := a.abstractor.AbstractMetadata(nil, artifact)
	a.Require().NotNil(err)
}

func TestAbstractorTestSuite(t *testing.T) {
	suite.Run(t, &abstractorTestSuite{})
}
