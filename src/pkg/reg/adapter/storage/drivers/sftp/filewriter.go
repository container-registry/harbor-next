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
	client    *clientWrapper
	closed    bool
	committed bool
	cancelled bool
}

func newFileWriter(file *sftp.File, client *clientWrapper, size int64) *fileWriter {
	return &fileWriter{
		file:   file,
		client: client,
		size:   size,
		bw:     bufio.NewWriter(file),
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
	if err := fw.bw.Flush(); err != nil {
		return err
	}
	if err := fw.file.Close(); err != nil {
		return err
	}
	fw.closed = true
	return nil
}

func (fw *fileWriter) Cancel() error {
	if fw.closed {
		return fmt.Errorf("already closed")
	}

	fw.cancelled = true

	if err := fw.file.Close(); err != nil {
		return err
	}
	return fw.client.Remove(fw.file.Name())
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
	fw.committed = true
	return nil
}
