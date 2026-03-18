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

package attestation

import (
	"github.com/goharbor/harbor/src/pkg/accessory/model"
	"github.com/goharbor/harbor/src/pkg/accessory/model/base"
)

// BuildKitAttestation models BuildKit provenance and SBOM attachments.
type BuildKitAttestation struct {
	base.Default
}

// Kind returns RefHard — attestations are hard-linked to their subject.
func (a *BuildKitAttestation) Kind() string {
	return model.RefHard
}

// IsHard returns true — attestations are hard-linked to their subject.
func (a *BuildKitAttestation) IsHard() bool {
	return true
}

// New returns a BuildKit attestation accessory.
func New(data model.AccessoryData) model.Accessory {
	return &BuildKitAttestation{base.Default{Data: data}}
}

func init() {
	model.Register(model.TypeBuildKitAttestation, New)
}
