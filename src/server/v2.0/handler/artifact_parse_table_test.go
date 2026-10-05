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

package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/lib/errors"
)

// The copy API's "from" value picks the source repository and reference.
func TestParseCopyFromTable(t *testing.T) {
	const dgst = "sha256:1b26826f602946860c279fce658f31050cff2c596583af237d971f4629b57792"
	cases := []struct {
		in   string
		repo string
		ref  string
		bad  bool
	}{
		{"library/app:v1", "library/app", "v1", false},
		{"library/app@" + dgst, "library/app", dgst, false},
		{"library/app:v1@" + dgst, "library/app", dgst, false},
		{"a/b/c:" + strings.Repeat("x", 128), "a/b/c", strings.Repeat("x", 128), false},
		{"p/a--b__c.d:t", "p/a--b__c.d", "t", false},
		{"library/app", "library/app", "", false},
		{"library/App:v1", "", "", true},
		{"library/app:" + strings.Repeat("x", 129), "", "", true},
		{"library/app@sha256:" + strings.Repeat("g", 64), "", "", true},
		{"library/app@sha256:abc", "", "", true},
		{"[::1]:5000/app:v1", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			repo, ref, err := parse(tc.in)
			if tc.bad {
				assert.True(t, errors.IsErr(err, errors.BadRequestCode), "want 400, got %v", err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tc.repo, repo)
			assert.Equal(t, tc.ref, ref)
		})
	}
}
