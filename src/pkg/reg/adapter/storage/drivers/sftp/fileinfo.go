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

package sftp

import (
	"os"
	"time"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
)

type fileInfo struct {
	os.FileInfo
	path string
}

var _ storagedriver.FileInfo = fileInfo{}

// Path provides the full path of the target of this file info.
func (fi fileInfo) Path() string {
	return fi.path
}

// Size returns current length in bytes of the file. The return value can
// be used to write to the end of the file at path. The value is
// meaningless if IsDir returns true.
func (fi fileInfo) Size() int64 {
	if fi.IsDir() {
		return 0
	}

	return fi.FileInfo.Size()
}

// ModTime returns the modification time for the file. For backends that
// don't have a modification time, the creation time should be returned.
func (fi fileInfo) ModTime() time.Time {
	return fi.FileInfo.ModTime()
}

// IsDir returns true if the path is a directory.
func (fi fileInfo) IsDir() bool {
	return fi.FileInfo.IsDir()
}

type fileInfoMock struct {
	path string

	// Size is current length in bytes of the file. The value of this field
	// can be used to write to the end of the file at path. The value is
	// meaningless if IsDir is set to true.
	size int64

	// ModTime returns the modification time for the file. For backends that
	// don't have a modification time, the creation time should be returned.
	modTime time.Time

	// IsDir returns true if the path is a directory.
	isDir bool
}

func (f fileInfoMock) Path() string {
	return f.path
}

func (f fileInfoMock) Size() int64 {
	return f.size
}

func (f fileInfoMock) ModTime() time.Time {
	return f.modTime
}

func (f fileInfoMock) IsDir() bool {
	return f.isDir
}

var _ storagedriver.FileInfo = fileInfoMock{}
