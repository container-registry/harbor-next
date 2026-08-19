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

package multiformat

import (
	"context"

	artifactctl "github.com/goharbor/harbor/src/controller/artifact"
	blobctl "github.com/goharbor/harbor/src/controller/blob"
	"github.com/goharbor/harbor/src/controller/repository"
	"github.com/goharbor/harbor/src/pkg/multiformat/dao"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
	"github.com/goharbor/harbor/src/pkg/registry"
)

// Controller is the multiformat domain entry point. It is the public surface the
// npm/Maven HTTP adapters call (through the format.Deps shim). Every method
// takes the resolved Harbor project name + project id; RBAC is enforced upstream
// in the multiformatauth middleware before any call reaches here.
type Controller interface {
	// Publish commits one immutable single-payload version (npm). Returns the new
	// proj_version.
	Publish(ctx context.Context, project string, ev PublishEvent) (int64, error)
	// PublishFile commits one file of a multi-file GAV version (Maven). Returns
	// the new proj_version.
	PublishFile(ctx context.Context, project string, ev FilePublishEvent) (int64, error)
	// LoadState returns the typed projection state, reconciling from the OCI
	// _index if the projection is cold.
	LoadState(ctx context.Context, project string, projectID int64, format, name string) (model.PackageState, bool, error)
	// SetDistTag re-points (version!="") or removes (version=="") a mutable
	// dist-tag without republishing. Returns the new proj_version.
	SetDistTag(ctx context.Context, project string, projectID int64, format, name, tag, version string) (int64, error)
	// UpdateVersionMetadata replaces native metadata without changing package bytes.
	UpdateVersionMetadata(ctx context.Context, project string, projectID int64, format, name, version string, meta []byte) (int64, error)
	// DeleteVersion removes a version from the authoritative package index.
	DeleteVersion(ctx context.Context, project string, projectID int64, format, name, version string) (int64, error)
	// PayloadBlob streams a single-payload version's layer0 bytes by digest.
	PayloadBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error)
	// MavenFileBlob streams one Maven file's layer bytes by digest.
	MavenFileBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error)
}

// Ctl is the global multiformat controller instance.
var Ctl = NewController()

// controller is the concrete Controller. It delegates the OCI mapping +
// projection work to the mapper, which holds the store seam, the DAO, and the
// UI-visibility glue.
type controller struct {
	mapper *mapper
}

// NewController wires the default controller over the global registry client,
// the multiformat DAO, and the global blob/repository/artifact controllers.
func NewController() Controller {
	return newControllerWith(registry.Cli, dao.New(), blobctl.Ctl, repository.Ctl, artifactctl.Ctl)
}

// newControllerWith is the testable constructor seam.
func newControllerWith(cli registry.Client, d dao.DAO, bc blobctl.Controller, rc repository.Controller, ac artifactctl.Controller) Controller {
	vis := &visibility{
		blobCtl: bc,
		repoCtl: rc,
		artEnsure: func(ctx context.Context, repo, digest string, tags []string) (bool, int64, error) {
			return ac.Ensure(ctx, repo, digest, &artifactctl.ArtOption{Tags: tags})
		},
	}
	st := newStore(cli)
	return &controller{
		mapper: newMapper(st, d, vis),
	}
}

func (c *controller) Publish(ctx context.Context, project string, ev PublishEvent) (int64, error) {
	return c.mapper.Publish(ctx, project, ev)
}

func (c *controller) PublishFile(ctx context.Context, project string, ev FilePublishEvent) (int64, error) {
	return c.mapper.PublishFile(ctx, project, ev)
}

func (c *controller) LoadState(ctx context.Context, project string, projectID int64, format, name string) (model.PackageState, bool, error) {
	return c.mapper.LoadState(ctx, project, projectID, format, name)
}

func (c *controller) SetDistTag(ctx context.Context, project string, projectID int64, format, name, tag, version string) (int64, error) {
	return c.mapper.SetDistTag(ctx, project, projectID, format, name, tag, version)
}

func (c *controller) UpdateVersionMetadata(ctx context.Context, project string, projectID int64, format, name, version string, meta []byte) (int64, error) {
	return c.mapper.UpdateVersionMetadata(ctx, project, projectID, format, name, version, meta)
}

func (c *controller) DeleteVersion(ctx context.Context, project string, projectID int64, format, name, version string) (int64, error) {
	return c.mapper.DeleteVersion(ctx, project, projectID, format, name, version)
}

func (c *controller) PayloadBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error) {
	return c.mapper.PayloadBlob(ctx, project, format, name, digest, size)
}

func (c *controller) MavenFileBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error) {
	return c.mapper.MavenFileBlob(ctx, project, format, name, digest, size)
}
