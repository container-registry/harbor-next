package sftp

import (
	"github.com/pkg/sftp"
)

type reader struct {
	*sftp.File
	close func()
}

func (r reader) Close() error {
	r.close()
	return r.File.Close()
}
