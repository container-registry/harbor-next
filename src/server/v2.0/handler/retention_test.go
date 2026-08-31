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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/server/v2.0/models"
)

// TestRetentionMetadataSelectors asserts the payload advertises both engines
// and that doublestar keeps the leading position it has always had, since the
// portal falls back to the first entry
func TestRetentionMetadataSelectors(t *testing.T) {
	for _, selectors := range [][]*models.RetentionSelectorMetadata{
		rentenitionMetadataPayload.ScopeSelectors,
		rentenitionMetadataPayload.TagSelectors,
	} {
		require.Len(t, selectors, 2)
		assert.Equal(t, "doublestar", selectors[0].Kind)
		assert.Equal(t, "regex", selectors[1].Kind)
		assert.Equal(t, selectors[0].DisplayText, selectors[1].DisplayText)
		assert.Equal(t, selectors[0].Decorations, selectors[1].Decorations)
	}

	assert.Equal(t, []string{"repoMatches", "repoExcludes"}, rentenitionMetadataPayload.ScopeSelectors[0].Decorations)
	assert.Equal(t, []string{"matches", "excludes"}, rentenitionMetadataPayload.TagSelectors[0].Decorations)
}
