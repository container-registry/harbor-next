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

package pypi

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/artifact"
)

func TestFilesReturnsFlatFileEntries(t *testing.T) {
	p := &processor{}
	a := &artifact.Artifact{
		ExtraAttrs: map[string]any{
			"metadata": map[string]any{
				"distributions": []map[string]any{
					{
						"filename":     "example-1.0.0.tar.gz",
						"content_type": "application/gzip",
						"size":         456,
						"sha256":       "def",
					},
					{
						"filename":     "example-1.0.0-py3-none-any.whl",
						"content_type": "application/octet-stream",
						"size":         123,
						"sha256":       "abc",
					},
				},
			},
		},
	}

	content, err := p.files(a)
	require.NoError(t, err)

	var files []fileEntry
	require.NoError(t, json.Unmarshal(content, &files))
	require.Equal(t, []fileEntry{
		{
			Path: "example-1.0.0-py3-none-any.whl",
			Name: "example-1.0.0-py3-none-any.whl",
			Size: 123,
		},
		{
			Path: "example-1.0.0.tar.gz",
			Name: "example-1.0.0.tar.gz",
			Size: 456,
		},
	}, files)
}
