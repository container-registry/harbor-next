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

package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// These patterns are built from the distribution reference grammar. They route
// requests and pick the repository for auth and quota, so pin how real and
// awkward names are matched.
func TestRepositoryNameRe(t *testing.T) {
	cases := []struct {
		name  string
		valid bool
	}{
		{"a", true},
		{"library/app", true},
		{"a/b/c/d/e", true},
		{"a.b", true},
		{"a_b", true},
		{"a__b", true},
		{"a-b", true},
		{"a--b", true},
		{"a---b", true},
		{"proj/sub-dir_x.y/img", true},
		{"0/1", true},
		{"localhost:5000/a", true},
		{"registry.example.com/a/b", true},
		{"my-reg.example.com:443/a", true},
		{"UPPER/a", true}, // first component reads as a domain
		{"Example.COM/a", true},

		{"", false},
		{"A", false},
		{"lib/App", false},
		{"a//b", false},
		{"a/", false},
		{"/a", false},
		{"-a", false},
		{"a-", false},
		{"_a", false},
		{"a_", false},
		{"a.", false},
		{"a..b", false},
		{"a___b", false},
		{"a._b", false},
		{"a b", false},
		{"a:tag", false},
		{"a@sha256:abc", false},
		{"localhost:port/a", false},
		{"[::1]:5000/a", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.valid, RepositoryNameRe.MatchString(tc.name))
		})
	}
}

func TestV2URLPatternCaptures(t *testing.T) {
	const dgst = "sha256:1b26826f602946860c279fce658f31050cff2c596583af237d971f4629b57792"
	cases := []struct {
		name string
		path string
		repo string
		ref  string
		ok   bool
	}{
		{"manifest tag", "/v2/library/app/manifests/v1.0", "library/app", "v1.0", true},
		{"manifest digest", "/v2/library/app/manifests/" + dgst, "library/app", dgst, true},
		{"manifest nested", "/v2/a/b/c/manifests/latest", "a/b/c", "latest", true},
		{"manifest separators", "/v2/p/a--b__c.d/manifests/t", "p/a--b__c.d", "t", true},
		{"manifest repo named manifests", "/v2/p/manifests/manifests/t", "p/manifests", "t", true},
		{"manifest ambiguous split", "/v2/p/a/manifests/b/manifests/t", "p/a/manifests/b", "t", true},
		{"manifest domain-like project", "/v2/localhost:5000/a/manifests/t", "localhost:5000/a", "t", true},
		{"manifest upper project", "/v2/Lib/a/manifests/t", "Lib/a", "t", true},
		{"manifest upper repo", "/v2/lib/App/manifests/t", "", "", false},
		{"manifest double slash", "/v2/lib//a/manifests/t", "", "", false},
		{"manifest trailing dash", "/v2/lib/a-/manifests/t", "", "", false},
		{"manifest any reference", "/v2/lib/a/manifests/%%%", "lib/a", "%%%", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, ref, ok := MatchManifestURLPattern(tc.path)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.repo, repo)
			assert.Equal(t, tc.ref, ref)
		})
	}

	blobs := []struct {
		path string
		repo string
		dgst string
		ok   bool
	}{
		{"/v2/library/app/blobs/" + dgst, "library/app", dgst, true},
		{"/v2/p/blobs/blobs/" + dgst, "p/blobs", dgst, true},
		{"/v2/library/app/blobs/sha256:short", "library/app", "sha256:short", true},
		{"/v2/library/app/blobs/notadigest", "", "", false},
		{"/v2/library/App/blobs/" + dgst, "", "", false},
	}
	for _, tc := range blobs {
		t.Run("blob "+tc.path, func(t *testing.T) {
			m := V2BlobURLRe.FindStringSubmatch(tc.path)
			assert.Equal(t, tc.ok, m != nil)
			if m != nil {
				assert.Equal(t, tc.repo, m[V2BlobURLRe.SubexpIndex(RepositorySubexp)])
				assert.Equal(t, tc.dgst, m[V2BlobURLRe.SubexpIndex(DigestSubexp)])
			}
		})
	}

	others := []struct {
		name string
		re   func(string) []string
		path string
		repo string
	}{
		{"tags", V2TagListURLRe.FindStringSubmatch, "/v2/library/app/tags/list", "library/app"},
		{"tags nested", V2TagListURLRe.FindStringSubmatch, "/v2/a/b/tags/tags/list", "a/b/tags"},
		{"upload start", V2BlobUploadURLRe.FindStringSubmatch, "/v2/library/app/blobs/uploads/", "library/app"},
		{"upload session", V2BlobUploadURLRe.FindStringSubmatch, "/v2/library/app/blobs/uploads/3f1c-2a_b.c=", "library/app"},
		{"referrers", V2ReferrersURLRe.FindStringSubmatch, "/v2/library/app/referrers/" + dgst, "library/app"},
		{"no match", V2TagListURLRe.FindStringSubmatch, "/v2/Library/App/tags/list", ""},
	}
	for _, tc := range others {
		t.Run(tc.name, func(t *testing.T) {
			m := tc.re(tc.path)
			if tc.repo == "" {
				assert.Nil(t, m)
				return
			}
			if assert.NotNil(t, m) {
				assert.Equal(t, tc.repo, m[1])
			}
		})
	}
}
