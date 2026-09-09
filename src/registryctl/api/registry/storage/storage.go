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

// Package storage reports the registry's storage volume usage (harbor-next
// #839, theme B). registryctl is the only Harbor component that shares the
// registry's mount, so this is where a truthful statfs can run. Object-store
// drivers expose no capacity API and are reported as unsupported.
package storage

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage"
	"github.com/goharbor/harbor/src/pkg/systeminfo/imagestorage/filesystem"
	"github.com/goharbor/harbor/src/registryctl/api"
)

const filesystemDriver = "filesystem"

// NewHandler returns the handler for GET /api/registry/storage.
func NewHandler(storageType, storageRoot string) http.Handler {
	h := &handler{storageType: storageType, storageRoot: storageRoot}
	if storageType == filesystemDriver {
		h.fs = filesystem.NewDriver(storageRoot)
	}
	return h
}

type handler struct {
	storageType string
	storageRoot string
	fs          imagestorage.Driver
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		api.HandleNotMethodAllowed(w)
		return
	}
	info := &imagestorage.VolumeInfo{Driver: h.storageType}
	if h.fs != nil {
		// filesystem.Cap reports zeros without an error for a missing path; that
		// must not be advertised as a measurement of an empty volume.
		if _, err := os.Stat(h.storageRoot); err != nil {
			log.Errorf("registry storage root %s is not accessible: %v", h.storageRoot, err)
			api.HandleInternalServerError(w, fmt.Errorf("registry storage root %s is not accessible: %w", h.storageRoot, err))
			return
		}
		capacity, err := h.fs.Cap()
		if err != nil {
			log.Errorf("failed to measure storage volume %s: %v", h.storageRoot, err)
			api.HandleInternalServerError(w, err)
			return
		}
		info.Supported = true
		info.Path = h.storageRoot
		info.Total = capacity.Total
		info.Free = capacity.Free
		if capacity.Total >= capacity.Free {
			info.Used = capacity.Total - capacity.Free
		}
		info.MeasuredAt = time.Now().UTC()
	}
	w.Header().Set("Content-Type", "application/json")
	_ = api.WriteJSON(w, info)
}
