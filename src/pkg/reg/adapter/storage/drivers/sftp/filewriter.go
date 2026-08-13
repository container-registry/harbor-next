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
	"fmt"

	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/pkg/sftp"
)

var _ storagedriver.FileWriter = &fileWriter{}

type fileWriter struct {
	file      *sftp.File
	session   *sftpSession
	size      int64
	closed    bool
	committed bool
	cancelled bool
}

func newFileWriter(file *sftp.File, session *sftpSession, size int64) *fileWriter {
	return &fileWriter{
		file:    file,
		session: session,
		size:    size,
	}
}

func (fw *fileWriter) Write(p []byte) (int, error) {
	if fw.closed {
		return 0, fmt.Errorf("already closed")
	} else if fw.committed {
		return 0, fmt.Errorf("already committed")
	} else if fw.cancelled {
		return 0, fmt.Errorf("already cancelled")
	}

	n, err := fw.file.Write(p)
	fw.size += int64(n)
	return n, err
}

func (fw *fileWriter) Size() int64 {
	return fw.size
}

func (fw *fileWriter) Close() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	err := fw.closeFileAndSession()

	fw.closed = true
	return err
}

// Cancel @todo add file delete
func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}
	fw.cancelled = true
	fw.closed = true
	return fw.closeFileAndSession()
}

func (fw *fileWriter) Commit() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	} else if fw.committed {
		return fmt.Errorf("already committed")
	} else if fw.cancelled {
		return fmt.Errorf("already cancelled")
	}
	fw.committed = true
	return nil
}

func (fw *fileWriter) closeFileAndSession() error {
	err := fw.file.Close()
	if closeErr := fw.session.Close(); err == nil {
		err = closeErr
	}
	return err
}
