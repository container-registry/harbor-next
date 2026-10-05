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

package cosign

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A signature push is linked to its subject through this pattern.
func TestCosignSignaturePattern(t *testing.T) {
	const hex = "1b26826f602946860c279fce658f31050cff2c596583af237d971f4629b57792"
	cases := []struct {
		path   string
		repo   string
		digest string
	}{
		{"/v2/library/app/manifests/sha256-" + hex + ".sig", "library/app", hex},
		{"/v2/a/b/c/manifests/sha256-" + hex + ".sig", "a/b/c", hex},
		{"/v2/p/a--b__c/manifests/sha256-" + hex + ".sig", "p/a--b__c", hex},
		{"/v2/p/manifests/manifests/sha256-" + hex + ".sig", "p/manifests", hex},
		{"/v2/library/app/manifests/sha256-" + hex + ".att", "", ""},
		{"/v2/library/app/manifests/sha256-" + hex[:63] + ".sig", "", ""},
		{"/v2/library/app/manifests/sha512-" + hex + ".sig", "", ""},
		{"/v2/library/App/manifests/sha256-" + hex + ".sig", "", ""},
		{"/v2/library/app/manifests/v1", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			m := cosignRe.FindStringSubmatch(tc.path)
			if tc.repo == "" {
				assert.Nil(t, m)
				return
			}
			if assert.NotNil(t, m) {
				assert.Equal(t, tc.repo, m[cosignRe.SubexpIndex(repositorySubexp)])
				assert.Equal(t, tc.digest, m[cosignRe.SubexpIndex(subArtDigestSubexp)])
			}
		})
	}
}
