package sftp

import (
	"github.com/pkg/sftp"
	"github.com/silenceper/pool"
	"io"
)

type fileReader struct {
	*sftp.File
	pool   pool.Pool
	client *clientWrapper
}

func (f fileReader) Close() error {
	if err := f.File.Close(); err != nil {
		return err
	}
	return f.pool.Put(f.client)
}

func newFileReader(file *sftp.File, pool pool.Pool, client *clientWrapper) *fileReader {
	return &fileReader{File: file, pool: pool, client: client}
}

var _ io.ReadCloser = (*fileReader)(nil)
