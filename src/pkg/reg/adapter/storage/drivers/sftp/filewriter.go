package sftp

import (
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/pkg/sftp"
)

var _ storagedriver.FileWriter = &fileWriter{}

type fileWriter struct {
	file      *sftp.File
	size      int64
	closed    bool
	committed bool
	cancelled bool
	closer    func()
}

func newFileWriter(file *sftp.File, size int64, closer func()) *fileWriter {
	return &fileWriter{
		file:   file,
		size:   size,
		closer: closer,
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

	defer func() {
		if fw.closer == nil {
			return
		}
		fw.closer()
		fw.closed = true
	}()

	return fw.file.Close()
}

// Cancel @todo add file delete
func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}
	fw.cancelled = true
	return nil
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
