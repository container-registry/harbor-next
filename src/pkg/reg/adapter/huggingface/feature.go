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

package huggingface

import (
	"github.com/goharbor/harbor/src/lib/config/metadata"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

// Feature gates the huggingface registry type. It is off by default.
const Feature feature.Feature = "huggingface_model_import"

// EnvKey overrides the stored setting and locks it in the UI.
const EnvKey = "HARBOR_ENABLE_HUGGINGFACE_MODEL_IMPORT"

func init() {
	feature.Register(feature.Definition{
		Feature:   Feature,
		ConfigKey: metadata.EnableHuggingFaceModelImport,
		EnvKey:    EnvKey,
	})
	feature.RegisterRegistryTypes(Feature, model.RegistryTypeHuggingFace)
}
