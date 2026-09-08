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

package imagestorage

import "time"

// GlobalDriver is a global image storage driver
var GlobalDriver Driver

// Capacity holds information about capacity of image storage
type Capacity struct {
	// total size(byte)
	Total uint64 `json:"total"`
	// available size(byte)
	Free uint64 `json:"free"`
	// used size(byte)
	Used uint64 `json:"used"`
	// Driver is the registry storage driver the numbers describe.
	Driver string `json:"driver"`
	// Measured is true when the numbers were read on the registry's own storage
	// volume (harbor-next #839, theme B) rather than on core's local disk.
	Measured bool `json:"measured"`
	// MeasuredAt is when the measurement was taken; zero when not measured.
	MeasuredAt time.Time `json:"measured_at"`
}

// VolumeInfo is what registryctl reports about the registry's storage volume.
type VolumeInfo struct {
	// Driver is the registry storage driver name.
	Driver string `json:"driver"`
	// Supported is false when the driver cannot be measured (object stores).
	Supported bool `json:"supported"`
	// Path is the measured root directory for the filesystem driver.
	Path       string    `json:"path,omitempty"`
	Total      uint64    `json:"total"`
	Free       uint64    `json:"free"`
	Used       uint64    `json:"used"`
	MeasuredAt time.Time `json:"measured_at"`
}

// Driver defines methods that an image storage driver must implement
type Driver interface {
	// Name returns a human-readable name of the driver
	Name() string
	// Cap returns the capacity of the image storage
	Cap() (*Capacity, error)
}
