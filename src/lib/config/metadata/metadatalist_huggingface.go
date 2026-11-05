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

package metadata

// EnableHuggingFaceModelImport switches the Hugging Face registry type on.
// The config manager reads EnvKey only for system-scope items; the override
// of this user-scope item is applied by src/pkg/feature.
const EnableHuggingFaceModelImport = "enable_huggingface_model_import"

func init() {
	ConfigList = append(ConfigList, Item{
		Name:         EnableHuggingFaceModelImport,
		Scope:        UserScope,
		Group:        BasicGroup,
		EnvKey:       "HARBOR_ENABLE_HUGGINGFACE_MODEL_IMPORT",
		DefaultValue: "false",
		ItemType:     &BoolType{},
		Editable:     true,
		Description:  `Enable importing Hugging Face models through the huggingface registry type (beta)`,
	})
}
