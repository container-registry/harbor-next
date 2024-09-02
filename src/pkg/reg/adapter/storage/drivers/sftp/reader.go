package sftp

import (
	"github.com/pkg/sftp"
)

type reader struct {
	*sftp.File
	closer func()
}

func (r reader) Close() error {
	if r.closer != nil {
		r.closer()
	}

	return r.File.Close()
}
