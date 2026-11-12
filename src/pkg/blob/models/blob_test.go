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

package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// GC decides between "delete manifest revision + tags" and "delete blob" from
// these answers. Media types are spelled out so a library constant that drifts
// or disappears fails here instead of silently changing GC.
func TestBlobClassification(t *testing.T) {
	cases := []struct {
		contentType string
		manifest    bool
		foreign     bool
	}{
		{"application/vnd.oci.image.manifest.v1+json", true, false},
		{"application/vnd.oci.image.index.v1+json", true, false},
		{"application/vnd.docker.distribution.manifest.v2+json", true, false},
		{"application/vnd.docker.distribution.manifest.list.v2+json", true, false},
		// schema1 rows can predate the v3 registry and must still be swept as manifests
		{"application/vnd.docker.distribution.manifest.v1+json", true, false},
		{"application/vnd.docker.distribution.manifest.v1+prettyjws", true, false},

		{"application/vnd.docker.image.rootfs.foreign.diff.tar.gzip", false, true},

		{"application/vnd.docker.image.rootfs.diff.tar.gzip", false, false},
		{"application/vnd.docker.container.image.v1+json", false, false},
		{"application/vnd.oci.image.layer.v1.tar+gzip", false, false},
		{"application/vnd.oci.image.layer.nondistributable.v1.tar+gzip", false, false},
		{"application/vnd.oci.image.config.v1+json", false, false},
		{"application/vnd.oci.empty.v1+json", false, false},
		{"application/vnd.cncf.helm.config.v1+json", false, false},
		{"application/octet-stream", false, false},
		{"application/json", false, false},
		{"", false, false},
		// matching is exact: parameters and case are not normalised
		{"application/vnd.oci.image.manifest.v1+json; charset=utf-8", false, false},
		{"Application/vnd.oci.image.manifest.v1+json", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.contentType, func(t *testing.T) {
			b := &Blob{ContentType: tc.contentType}
			assert.Equal(t, tc.manifest, b.IsManifest(), "IsManifest")
			assert.Equal(t, tc.foreign, b.IsForeignLayer(), "IsForeignLayer")
		})
	}
}
