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

package distribution

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const sha = "sha256:1b26826f602946860c279fce658f31050cff2c596583af237d971f4629b57792"

func TestParseRefTable(t *testing.T) {
	tag128 := "t" + strings.Repeat("x", 127)
	cases := []struct {
		in       string
		repo     string
		ref      string
		hasError bool
	}{
		{"library/app:v1", "library/app", "v1", false},
		{"library/app@" + sha, "library/app", sha, false},
		{"library/app:v1@" + sha, "library/app", sha, false},
		{"library/app", "library/app", "", false},
		{"a/b/c:1.0-rc_1", "a/b/c", "1.0-rc_1", false},
		{"localhost:5000/a:v1", "localhost:5000/a", "v1", false},
		{"a:" + tag128, "a", tag128, false},
		{"a__b--c:x", "a__b--c", "x", false},

		{"a:" + tag128 + "x", "", "", true},
		{"a:.bad", "", "", true},
		{"a:-bad", "", "", true},
		{"A/b:v1", "A/b", "v1", false},
		{"lib/App:v1", "", "", true},
		{"a@sha256:abc", "", "", true},
		{"a@sha256:" + strings.Repeat("z", 64), "", "", true},
		{"a@md5:" + strings.Repeat("a", 32), "", "", true},
		{"", "", "", true},
		{"a:", "", "", true},
		{"[::1]:5000/a:v1", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			repo, ref, err := ParseRef(tc.in)
			if tc.hasError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.repo, repo)
			assert.Equal(t, tc.ref, ref)
		})
	}
}

func TestIsDigestTable(t *testing.T) {
	cases := map[string]bool{
		sha:                                  true,
		"sha512:" + strings.Repeat("a", 128): true,
		"sha256:" + strings.Repeat("a", 32):  true,
		"sha256:" + strings.Repeat("a", 31):  false,
		"latest":                             false,
		"sha256:":                            false,
		"sha256:XYZ":                         false,
		"":                                   false,
		"v1@" + sha:                          true, // unanchored
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, IsDigest(in))
		})
	}
}

func TestParseURLTable(t *testing.T) {
	cases := []struct {
		path, name, project, ref, session string
	}{
		{"/v2/library/app/manifests/v1", "library/app", "library", "v1", ""},
		{"/v2/library/app/manifests/" + sha, "library/app", "library", sha, ""},
		{"/v2/p/a/b/manifests/latest", "p/a/b", "p", "latest", ""},
		{"/v2/p/a/blobs/" + sha, "p/a", "p", "", ""},
		{"/v2/p/a/blobs/uploads/abc-123_x.y=", "p/a", "p", "", "abc-123_x.y="},
		{"/v2/p/a/tags/list", "p/a", "p", "", ""},
		{"/v2/p/manifests/manifests/v1", "p/manifests", "p", "v1", ""},
		{"/v2/p/App/manifests/v1", "", "", "", ""},
		{"/v2/_catalog", "", "", "", ""},
		{"/v2/p/a/manifests/.bad", "p/a", "p", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			assert.Equal(t, tc.name, ParseName(tc.path), "name")
			assert.Equal(t, tc.project, ParseProjectName(tc.path), "project")
			assert.Equal(t, tc.ref, ParseReference(tc.path), "reference")
			assert.Equal(t, tc.session, ParseSessionID(tc.path), "session")
		})
	}
}
