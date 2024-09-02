package sftp

import (
	"fmt"
	"github.com/pkg/sftp"
)

type reader struct {
	*sftp.File
	num    int
	closer func()
}

func (r reader) Close() error {
	fmt.Printf("reader.Close %d\n", r.num)
	if r.closer != nil {
		r.closer()
	}

	return r.File.Close()
}
