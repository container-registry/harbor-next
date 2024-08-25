package sftp

import (
	"bufio"
	"fmt"
	storagedriver "github.com/docker/distribution/registry/storage/driver"
	"github.com/pkg/sftp"
)

var _ storagedriver.FileWriter = &fileWriter{}

type fileWriter struct {
	file      *sftp.File
	size      int64
	bw        *bufio.Writer
	closed    bool
	committed bool
	cancelled bool
	closer    func()
}

func newFileWriter(file *sftp.File, size int64, closer func()) *fileWriter {
	return &fileWriter{
		file:   file,
		size:   size,
		bw:     bufio.NewWriter(file),
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
	n, err := fw.bw.Write(p)
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

	// closing anyway even if followed errored

	if err := fw.bw.Flush(); err != nil {
		return err
	}
	if err := fw.file.Sync(); err != nil {
		return err
	}

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
	if err := fw.bw.Flush(); err != nil {
		return err
	}
	if err := fw.file.Sync(); err != nil {
		return err
	}
	fw.committed = true
	return nil
}
