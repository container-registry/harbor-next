package sftp

import (
	"fmt"
	"github.com/pkg/sftp"
)

type reader struct {
	*sftp.File
	closer func()
}

func (r reader) Close() error {
	fmt.Println("reader.Close")
	if r.closer != nil {
		r.closer()
	}

	return r.File.Close()
}
