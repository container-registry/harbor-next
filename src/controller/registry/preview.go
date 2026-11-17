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

package registry

import (
	"context"
	"strings"

	"github.com/goharbor/harbor/src/lib/errors"
	adp "github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func (c *controller) Preview(ctx context.Context, id int64, repository, reference string, filter *model.FileFilter) (*model.ResolveResult, error) {
	if strings.TrimSpace(repository) == "" {
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).WithMessage("the repository is required")
	}
	if err := filter.Validate(); err != nil {
		return nil, err
	}
	registry, err := c.regMgr.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	adapter, err := c.regMgr.CreateAdapter(ctx, registry)
	if err != nil {
		return nil, err
	}
	resolver, ok := adapter.(adp.ArtifactResolver)
	if !ok {
		return nil, errors.New(nil).WithCode(errors.BadRequestCode).
			WithMessagef("the registry %s does not support previews", registry.Name)
	}
	return resolver.Resolve(ctx, repository, reference, filter)
}
