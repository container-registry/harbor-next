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

package test

// Hex digests chosen so DigestA and DigestB share the "aa" shard directory.
const (
	HexA = "aa11111111111111111111111111111111111111111111111111111111111111"
	HexB = "aa22222222222222222222222222222222222222222222222222222222222222"
	HexC = "bb33333333333333333333333333333333333333333333333333333333333333"
	HexD = "cc44444444444444444444444444444444444444444444444444444444444444"

	DigestA = "sha256:" + HexA
	DigestB = "sha256:" + HexB
	DigestC = "sha256:" + HexC
	DigestD = "sha256:" + HexD
)

// Paths of a small registry tree: three blobs, a tagged manifest C in
// library/app, a layer link to A, an upload, and a revision of A in another repo.
var (
	BlobA = StorageRoot + "/blobs/sha256/aa/" + HexA + "/data"
	BlobB = StorageRoot + "/blobs/sha256/aa/" + HexB + "/data"
	BlobC = StorageRoot + "/blobs/sha256/bb/" + HexC + "/data"

	AppLayerA     = StorageRoot + "/repositories/library/app/_layers/sha256/" + HexA + "/link"
	AppRevisionC  = StorageRoot + "/repositories/library/app/_manifests/revisions/sha256/" + HexC + "/link"
	AppRevisionB  = StorageRoot + "/repositories/library/app/_manifests/revisions/sha256/" + HexB + "/link"
	AppTagCurrent = StorageRoot + "/repositories/library/app/_manifests/tags/v1/current/link"
	AppTagIndexC  = StorageRoot + "/repositories/library/app/_manifests/tags/v1/index/sha256/" + HexC + "/link"
	AppUpload     = StorageRoot + "/repositories/library/app/_uploads/u1/data"
	OtherRevC     = StorageRoot + "/repositories/library/other/_manifests/revisions/sha256/" + HexC + "/link"
)

// AllFixturePaths lists every seeded path.
func AllFixturePaths() []string {
	return []string{
		BlobA, BlobB, BlobC,
		AppLayerA, AppRevisionC, AppRevisionB, AppTagCurrent, AppTagIndexC, AppUpload,
		OtherRevC,
	}
}
