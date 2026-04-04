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
	"fmt"
	"testing"

	"github.com/docker/distribution"
	"github.com/docker/distribution/manifest"
	"github.com/docker/distribution/manifest/manifestlist"
	"github.com/docker/distribution/manifest/schema2"
	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact"
	"github.com/goharbor/harbor/src/lib"

	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

const ociManifest = `{
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

type CacheTestSuite struct {
	suite.Suite
	mCache     *ManifestCache
	mListCache *ManifestListCache
	local      localInterfaceMock
}

func (suite *CacheTestSuite) SetupSuite() {
	// Use short intervals so tests don't sleep for minutes
	sleepIntervalSec = 0
	maxManifestListWait = 2
	maxManifestWait = 2

	suite.local = localInterfaceMock{}
	suite.mListCache = &ManifestListCache{local: &suite.local}
	suite.mCache = &ManifestCache{local: &suite.local}
}

func (suite *CacheTestSuite) TearDownSuite() {
}
func (suite *CacheTestSuite) TestUpdateManifestList() {
	ctx := context.Background()
	amdDig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	armDig := "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"
	manifestList := manifestlist.ManifestList{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     manifestlist.MediaTypeManifestList,
		},
		Manifests: []manifestlist.ManifestDescriptor{
			{
				Descriptor: distribution.Descriptor{
					Digest:    digest.Digest(amdDig),
					Size:      3253,
					MediaType: schema2.MediaTypeManifest,
				},
				Platform: manifestlist.PlatformSpec{
					Architecture: "amd64",
					OS:           "linux",
				},
			}, {
				Descriptor: distribution.Descriptor{
					Digest:    digest.Digest(armDig),
					Size:      3253,
					MediaType: schema2.MediaTypeManifest,
				},
				Platform: manifestlist.PlatformSpec{
					Architecture: "arm",
					OS:           "linux",
				},
			},
		},
	}
	manList := &manifestlist.DeserializedManifestList{
		ManifestList: manifestList,
	}
	ar := &artifact.Artifact{}
	suite.local.GetManifestFunc = func(_ context.Context, art lib.ArtifactInfo) (*artifact.Artifact, error) {
		if art.Digest == amdDig {
			return ar, nil
		}
		return nil, nil
	}

	newMan, err := suite.mListCache.updateManifestList(ctx, "library/hello-world", manList)
	suite.Require().Nil(err)
	suite.Assert().Equal(len(newMan.References()), 1)
}

func (suite *CacheTestSuite) TestPushManifestList() {
	ctx := context.Background()
	amdDig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	armDig := "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"
	manifestList := manifestlist.ManifestList{
		Versioned: manifest.Versioned{
			SchemaVersion: 2,
			MediaType:     manifestlist.MediaTypeManifestList,
		},
		Manifests: []manifestlist.ManifestDescriptor{
			{
				Descriptor: distribution.Descriptor{
					Digest:    digest.Digest(amdDig),
					Size:      3253,
					MediaType: schema2.MediaTypeManifest,
				},
				Platform: manifestlist.PlatformSpec{
					Architecture: "amd64",
					OS:           "linux",
				},
			}, {
				Descriptor: distribution.Descriptor{
					Digest:    digest.Digest(armDig),
					Size:      3253,
					MediaType: schema2.MediaTypeManifest,
				},
				Platform: manifestlist.PlatformSpec{
					Architecture: "arm",
					OS:           "linux",
				},
			},
		},
	}
	manList := &manifestlist.DeserializedManifestList{
		ManifestList: manifestList,
	}
	_, payload, err := manList.Payload()
	suite.Nil(err)
	originDigest := digest.FromBytes(payload)

	ar := &artifact.Artifact{}
	suite.local.GetManifestFunc = func(_ context.Context, art lib.ArtifactInfo) (*artifact.Artifact, error) {
		if art.Digest == amdDig {
			return ar, nil
		}
		return nil, nil
	}

	suite.local.PushManifestFunc = func(_ string, tag string, _ distribution.Manifest) error {
		if tag == string(originDigest) {
			return fmt.Errorf("wrong digest")
		}
		return nil
	}
	suite.local.UpdatePullTimeFunc = func(_ context.Context, _ lib.ArtifactInfo) error {
		return nil
	}

	err = suite.mListCache.push(ctx, "library/hello-world", string(originDigest), manList)
	suite.Require().Nil(err)
}

func (suite *CacheTestSuite) TestManifestCache_CacheContent() {
	manifest := ociManifest
	man, desc, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(manifest))
	suite.Require().NoError(err)

	ctx := context.Background()
	repo := "library/hello-world"

	artInfo := lib.ArtifactInfo{
		Repository: repo,
		Digest:     string(desc.Digest),
		Tag:        "latest",
	}

	suite.local.CheckDependenciesFunc = func(_ context.Context, _ string, _ distribution.Manifest) []distribution.Descriptor {
		return []distribution.Descriptor{}
	}
	suite.local.PushManifestFunc = func(_ string, _ string, _ distribution.Manifest) error {
		return nil
	}

	suite.mCache.CacheContent(ctx, repo, man, artInfo, nil, "")
}

func (suite *CacheTestSuite) TestManifestCache_push_succeeds() {
	manifest := ociManifest
	man, desc, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(manifest))
	suite.Require().NoError(err)

	repo := "library/hello-world"

	artInfo := lib.ArtifactInfo{
		Repository: repo,
		Digest:     string(desc.Digest),
		Tag:        "latest",
	}

	suite.local.PushManifestFunc = func(_ string, _ string, _ distribution.Manifest) error {
		return nil
	}

	err = suite.mCache.push(artInfo, man)
	suite.Assert().NoError(err)
}

func (suite *CacheTestSuite) TestManifestCache_push_fails() {
	manifest := ociManifest
	man, desc, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(manifest))
	suite.Require().NoError(err)

	repo := "library/hello-world"

	artInfo := lib.ArtifactInfo{
		Repository: repo,
		Digest:     string(desc.Digest),
		Tag:        "latest",
	}

	digestErr := fmt.Errorf("error during manifest push referencing digest")
	tagErr := fmt.Errorf("error during manifest push referencing tag")
	suite.local.PushManifestFunc = func(_ string, ref string, _ distribution.Manifest) error {
		if ref == artInfo.Digest {
			return digestErr
		}
		if ref == artInfo.Tag {
			return tagErr
		}
		return nil
	}

	err = suite.mCache.push(artInfo, man)
	suite.Assert().Error(err)
	wrappedErr, isWrappedErr := err.(interface{ Unwrap() []error })
	suite.Assert().True(isWrappedErr)
	errs := wrappedErr.Unwrap()
	suite.Assert().Len(errs, 2)
	suite.Assert().Contains(errs, digestErr)
	suite.Assert().Contains(errs, tagErr)
}

func TestCacheTestSuite(t *testing.T) {
	suite.Run(t, &CacheTestSuite{})
}
