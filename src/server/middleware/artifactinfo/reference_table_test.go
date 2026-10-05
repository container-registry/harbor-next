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

package artifactinfo

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib"
)

// Tag vs digest classification of the manifest reference.
func TestParseReferenceKind(t *testing.T) {
	const dgst = "sha256:08e4a417ff4e3913d8723a05cc34055db01c2fd165b588e049c5bad16ce6094f"
	cases := []struct {
		ref    string
		digest string
		tag    string
		err    bool
	}{
		{dgst, dgst, "", false},
		{"latest", "", "latest", false},
		{"v1.2.3-rc_1", "", "v1.2.3-rc_1", false},
		{strings.Repeat("t", 128), "", strings.Repeat("t", 128), false},
		{"_under", "", "_under", false},
		{".dot", "", ".dot", false}, // tag pattern is unanchored
		{"%%%", "", "", true},
		{"sha256:short", "sha256:short", "", false}, // go-digest pattern only checks shape
	}
	for _, tc := range cases {
		t.Run(tc.ref, func(t *testing.T) {
			u, err := url.Parse("/v2/library/app/manifests/" + url.PathEscape(tc.ref))
			require.NoError(t, err)
			m, match, err := parse(u)
			assert.True(t, match)
			if tc.err {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "library/app", m[lib.RepositorySubexp])
			assert.Equal(t, tc.digest, m[lib.DigestSubexp])
			assert.Equal(t, tc.tag, m[tag])
		})
	}
}
