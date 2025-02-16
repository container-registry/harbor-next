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

package s3

import (
	"context"

	"github.com/docker/distribution/registry/storage/driver/s3-aws"

	"github.com/goharbor/harbor/src/pkg/reg/adapter/storage/health"
)

type Driver struct {
	*s3.Driver
}

func (d Driver) Health(ctx context.Context) error {
	_, err := d.List(ctx, "/")
	return err
}

var _ health.Checker = (*Driver)(nil)
