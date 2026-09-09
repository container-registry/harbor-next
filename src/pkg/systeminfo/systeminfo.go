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

package systeminfo

import (
	"os"
	"sync"

	"github.com/goharbor/harbor/src/common/registryctl"
	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage/filesystem"
	regctldriver "github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage/registryctl"
)

var initOnce sync.Once

// Init image storage driver: registryctl measures the registry's own volume
// (harbor-next #839, theme B); core's local disk is only the fallback, kept
// for Compose installs where both share the host's data directory.
func Init() {
	initOnce.Do(func() {
		path := os.Getenv("IMAGE_STORE_PATH")
		if len(path) == 0 {
			path = "/data"
		}
		local := filesystem.NewDriver(path)
		imagestorage.GlobalDriver = regctldriver.NewDriver(registryctl.RegistryCtlClient, local)
	})
}
