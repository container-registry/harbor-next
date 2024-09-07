package s3

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/docker/distribution/registry/storage/driver/s3-aws"
	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
)

type Driver struct {
	*s3.Driver
}

func (d Driver) Health(ctx context.Context) error {
	_, err := d.List(ctx, "")
	spew.Dump(err)
	return err
}

var _ health.Checker = (*Driver)(nil)
