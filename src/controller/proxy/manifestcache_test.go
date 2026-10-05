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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/manifestlist"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/controller/artifact"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/testing/mock"

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
	mCache      *ManifestCache
	mListCache  *ManifestListCache
	local       localInterfaceMock
	oldListWait int
	oldWait     int
	oldSleepSec int
}

func (suite *CacheTestSuite) SetupSuite() {
	suite.local = localInterfaceMock{}
	suite.mListCache = &ManifestListCache{local: &suite.local}
	suite.mCache = &ManifestCache{local: &suite.local}
	suite.oldListWait = maxManifestListWait
	suite.oldWait = maxManifestWait
	suite.oldSleepSec = sleepIntervalSec
	maxManifestListWait = 1
	maxManifestWait = 1
	sleepIntervalSec = 0
}

func (suite *CacheTestSuite) TearDownSuite() {
	maxManifestListWait = suite.oldListWait
	maxManifestWait = suite.oldWait
	sleepIntervalSec = suite.oldSleepSec
}
func (suite *CacheTestSuite) TestUpdateManifestList() {
	ctx := context.Background()
	amdDig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	armDig := "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"
	manifestList := manifestlist.ManifestList{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: manifestlist.MediaTypeManifestList,
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
	artInfo1 := lib.ArtifactInfo{
		Repository: "library/hello-world",
		Digest:     amdDig,
		Tag:        "",
	}
	ar := &artifact.Artifact{}
	suite.local.On("GetManifest", ctx, artInfo1).Return(ar, nil)
	suite.local.On("GetManifest", ctx, mock.Anything).Return(nil, nil)

	newMan, err := suite.mListCache.updateManifestList(ctx, "library/hello-world", manList)
	suite.Require().Nil(err)
	suite.Assert().Equal(len(newMan.References()), 1)
}

// TestUpdateManifestListParsedIndexes runs real payloads through UnmarshalManifest, so a
// change in the type the library returns for an index cannot silently skip the cache.
func (suite *CacheTestSuite) TestUpdateManifestListParsedIndexes() {
	amdDig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	armDig := "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"
	cases := []struct {
		name          string
		mediaType     string
		childType     string
		annotations   string
		wantAnnotated bool
	}{
		{"docker manifest list", manifestlist.MediaTypeManifestList, schema2.MediaTypeManifest, "", false},
		{"oci image index", v1.MediaTypeImageIndex, v1.MediaTypeImageManifest, `,"annotations":{"org.example.k":"v"}`, true},
	}
	for _, tc := range cases {
		suite.Run(tc.name, func() {
			ctx := context.Background()
			payload := fmt.Sprintf(`{"schemaVersion":2,"mediaType":%q,"manifests":[`+
				`{"mediaType":%q,"digest":%q,"size":3253,"platform":{"architecture":"amd64","os":"linux"}},`+
				`{"mediaType":%q,"digest":%q,"size":3253,"platform":{"architecture":"arm64","os":"linux"}}]%s}`,
				tc.mediaType, tc.childType, amdDig, tc.childType, armDig, tc.annotations)
			man, _, err := distribution.UnmarshalManifest(tc.mediaType, []byte(payload))
			suite.Require().NoError(err)

			local := localInterfaceMock{}
			local.On("GetManifest", ctx, lib.ArtifactInfo{Repository: "library/hello-world", Digest: amdDig}).Return(&artifact.Artifact{}, nil)
			local.On("GetManifest", ctx, mock.Anything).Return(nil, nil)
			cache := &ManifestListCache{local: &local}

			newMan, err := cache.updateManifestList(ctx, "library/hello-world", man)
			suite.Require().NoError(err)
			refs := newMan.References()
			suite.Require().Len(refs, 1)
			suite.Equal(amdDig, string(refs[0].Digest))

			mt, pl, err := newMan.Payload()
			suite.Require().NoError(err)
			suite.Equal(tc.mediaType, mt)
			var idx v1.Index
			suite.Require().NoError(json.Unmarshal(pl, &idx))
			suite.Equal(tc.mediaType, idx.MediaType)
			suite.Require().Len(idx.Manifests, 1)
			suite.Require().NotNil(idx.Manifests[0].Platform)
			suite.Equal("amd64", idx.Manifests[0].Platform.Architecture)
			if tc.wantAnnotated {
				suite.Equal(map[string]string{"org.example.k": "v"}, idx.Annotations)
			}
		})
	}
}

func (suite *CacheTestSuite) TestPushManifestList() {
	ctx := context.Background()
	amdDig := "sha256:1a9ec845ee94c202b2d5da74a24f0ed2058318bfa9879fa541efaecba272e86b"
	armDig := "sha256:92c7f9c92844bbbb5d0a101b22f7c2a7949e40f8ea90c8b3bc396879d95e899a"
	manifestList := manifestlist.ManifestList{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType: manifestlist.MediaTypeManifestList,
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
	repo := "library/hello-world"
	artInfo1 := lib.ArtifactInfo{
		Repository: repo,
		Digest:     amdDig,
		Tag:        "",
	}
	ar := &artifact.Artifact{}
	_, payload, err := manList.Payload()
	suite.Nil(err)
	originDigest := digest.FromBytes(payload)

	suite.local.On("GetManifest", ctx, artInfo1).Return(ar, nil)
	suite.local.On("GetManifest", ctx, mock.Anything).Return(nil, nil)

	suite.local.On("PushManifest", repo, originDigest, mock.Anything).Return(fmt.Errorf("wrong digest"))
	suite.local.On("PushManifest", repo, mock.Anything, mock.Anything).Return(nil)
	suite.local.On("UpdatePullTime", ctx, mock.Anything).Return(nil)

	err = suite.mListCache.push(ctx, "library/hello-world", string(originDigest), manList)
	suite.Require().Nil(err)
	suite.local.AssertCalled(suite.T(), "UpdatePullTime", ctx, mock.Anything)
}

func (suite *CacheTestSuite) TestManifestCache_CacheContent() {
	defer suite.local.AssertExpectations(suite.T())

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

	suite.local.On("CheckDependencies", ctx, artInfo.Repository, man).Once().Return([]distribution.Descriptor{})
	suite.local.On("PushManifest", artInfo.Repository, artInfo.Digest, man).Once().Return(nil)
	suite.local.On("PushManifest", artInfo.Repository, artInfo.Tag, man).Once().Return(nil)

	suite.mCache.CacheContent(ctx, repo, man, artInfo, nil, "")
}

func (suite *CacheTestSuite) TestManifestCache_push_succeeds() {
	defer suite.local.AssertExpectations(suite.T())

	manifest := ociManifest
	man, desc, err := distribution.UnmarshalManifest(v1.MediaTypeImageManifest, []byte(manifest))
	suite.Require().NoError(err)

	repo := "library/hello-world"

	artInfo := lib.ArtifactInfo{
		Repository: repo,
		Digest:     string(desc.Digest),
		Tag:        "latest",
	}

	suite.local.On("PushManifest", artInfo.Repository, artInfo.Digest, man).Once().Return(nil)
	suite.local.On("PushManifest", artInfo.Repository, artInfo.Tag, man).Once().Return(nil)

	err = suite.mCache.push(artInfo, man)
	suite.Assert().NoError(err)
}

func (suite *CacheTestSuite) TestManifestCache_push_fails() {
	defer suite.local.AssertExpectations(suite.T())

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
	suite.local.On("PushManifest", artInfo.Repository, artInfo.Digest, man).Once().Return(digestErr)
	suite.local.On("PushManifest", artInfo.Repository, artInfo.Tag, man).Once().Return(tagErr)

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
