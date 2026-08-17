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

// Package multiformat defines the narrowed dependency surface that native-protocol
// HTTP adapters (npm, Maven) get. Adapters know the native wire only; they never
// see raw OCI (no store, no PushManifest). They reach the OCI mapping + Postgres
// projection exclusively through Deps, which is backed by the multiformat controller.
//
// The multi-format-oci POC keyed on a free-form namespace string; Harbor keys on a
// resolved project (name + id), so every method carries both: the project name
// is the leading OCI repo segment and the project id scopes the projection. The
// multiformatauth middleware resolves and authorizes the project before any handler
// runs, then stashes the name + id for the adapter to pass through.
package multiformat

import (
	"context"
	"net/http"

	controller "github.com/goharbor/harbor/src/controller/multiformat"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
)

// Format is one native-protocol adapter. It mounts its routes on the mux.
type Format interface {
	// Name is the native format identifier ("npm", "maven").
	Name() string
	// Mount registers the adapter's HTTP routes.
	Mount(mux *http.ServeMux, deps Deps)
}

// FileCoord identifies one file within a GAV (mirrors controller.FileCoord).
type FileCoord struct {
	Classifier  string
	Extension   string
	Timestamp   string
	BuildNumber int
}

// PublishEvent commits one immutable single-payload version (npm). It mirrors
// controller.PublishEvent; the Deps shim translates between them.
type PublishEvent struct {
	Format    string
	ProjectID int64
	Name      string
	Version   string
	Payload   []byte
	Meta      []byte
	DistTags  map[string]string
	Immutable bool
}

// FilePublishEvent commits ONE file of a multi-file GAV version (Maven). It
// mirrors controller.FilePublishEvent.
type FilePublishEvent struct {
	Format          string
	ProjectID       int64
	Name            string
	Version         string
	File            FileCoord
	Filename        string
	Payload         []byte
	Meta            []byte
	SnapshotMutable bool
}

// Publisher commits an immutable single-payload version.
type Publisher interface {
	Publish(ctx context.Context, project string, ev PublishEvent) (projVersion int64, err error)
}

// FilePublisher commits ONE file of a multi-file version (Maven GAV).
type FilePublisher interface {
	PublishFile(ctx context.Context, project string, ev FilePublishEvent) (projVersion int64, err error)
}

// StateSource yields the typed PackageState for a package, reconciling from OCI
// if the projection is cold. Adapters render their wire index from this.
type StateSource interface {
	LoadState(ctx context.Context, project string, projectID int64, format, name string) (model.PackageState, bool, error)
}

// PayloadFetcher streams layer0 (the exact unmodified artifact bytes).
type PayloadFetcher interface {
	PayloadBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error)
}

// MavenFileFetcher streams one Maven file's layer bytes by digest.
type MavenFileFetcher interface {
	MavenFileBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error)
}

// DistTagWriter re-points or removes a mutable dist-tag without republishing
// (npm). version=="" removes the tag.
type DistTagWriter interface {
	SetDistTag(ctx context.Context, project string, projectID int64, format, name, tag, version string) (projVersion int64, err error)
}

// VersionWriter updates mutable per-version metadata or removes a version.
// Package bytes remain immutable; npm uses this seam for deprecate/unpublish.
type VersionWriter interface {
	UpdateVersionMetadata(ctx context.Context, project string, projectID int64, format, name, version string, meta []byte) (projVersion int64, err error)
	DeleteVersion(ctx context.Context, project string, projectID int64, format, name, version string) (projVersion int64, err error)
}

// Deps is the narrowed dependency surface handed to every adapter. All members
// are backed by the multiformat controller (see NewDeps); BaseURL is the external
// URL prefix the adapter uses to render absolute tarball / file URLs.
type Deps struct {
	Publisher     Publisher
	FilePublisher FilePublisher
	State         StateSource
	Payload       PayloadFetcher
	MavenFile     MavenFileFetcher
	DistTags      DistTagWriter
	Versions      VersionWriter
	BaseURL       string
}

// NewDeps builds a Deps backed by the multiformat controller. The controller
// satisfies every Deps interface directly, so the shim is a single adapter
// value translating the local event types to the controller's.
func NewDeps(ctl controller.Controller, baseURL string) Deps {
	a := ctlAdapter{ctl: ctl}
	return Deps{
		Publisher:     a,
		FilePublisher: a,
		State:         a,
		Payload:       a,
		MavenFile:     a,
		DistTags:      a,
		Versions:      a,
		BaseURL:       baseURL,
	}
}

// ctlAdapter adapts controller.Controller to the local Deps interfaces. It
// translates the format-package event types to the controller's identically
// shaped ones (boundary discipline: adapters never import the controller types).
type ctlAdapter struct {
	ctl controller.Controller
}

func (a ctlAdapter) Publish(ctx context.Context, project string, ev PublishEvent) (int64, error) {
	return a.ctl.Publish(ctx, project, controller.PublishEvent{
		Format:    ev.Format,
		ProjectID: ev.ProjectID,
		Name:      ev.Name,
		Version:   ev.Version,
		Payload:   ev.Payload,
		Meta:      ev.Meta,
		DistTags:  ev.DistTags,
		Immutable: ev.Immutable,
	})
}

func (a ctlAdapter) PublishFile(ctx context.Context, project string, ev FilePublishEvent) (int64, error) {
	return a.ctl.PublishFile(ctx, project, controller.FilePublishEvent{
		Format:    ev.Format,
		ProjectID: ev.ProjectID,
		Name:      ev.Name,
		Version:   ev.Version,
		File: controller.FileCoord{
			Classifier:  ev.File.Classifier,
			Extension:   ev.File.Extension,
			Timestamp:   ev.File.Timestamp,
			BuildNumber: ev.File.BuildNumber,
		},
		Filename:        ev.Filename,
		Payload:         ev.Payload,
		Meta:            ev.Meta,
		SnapshotMutable: ev.SnapshotMutable,
	})
}

func (a ctlAdapter) LoadState(ctx context.Context, project string, projectID int64, format, name string) (model.PackageState, bool, error) {
	return a.ctl.LoadState(ctx, project, projectID, format, name)
}

func (a ctlAdapter) PayloadBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error) {
	return a.ctl.PayloadBlob(ctx, project, format, name, digest, size)
}

func (a ctlAdapter) MavenFileBlob(ctx context.Context, project, format, name, digest string, size int64) ([]byte, error) {
	return a.ctl.MavenFileBlob(ctx, project, format, name, digest, size)
}

func (a ctlAdapter) SetDistTag(ctx context.Context, project string, projectID int64, format, name, tag, version string) (int64, error) {
	return a.ctl.SetDistTag(ctx, project, projectID, format, name, tag, version)
}

func (a ctlAdapter) UpdateVersionMetadata(ctx context.Context, project string, projectID int64, format, name, version string, meta []byte) (int64, error) {
	return a.ctl.UpdateVersionMetadata(ctx, project, projectID, format, name, version, meta)
}

func (a ctlAdapter) DeleteVersion(ctx context.Context, project string, projectID int64, format, name, version string) (int64, error) {
	return a.ctl.DeleteVersion(ctx, project, projectID, format, name, version)
}
