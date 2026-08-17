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
	"fmt"
	"sort"
	"strconv"
	"time"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	quotactl "github.com/goharbor/harbor/src/controller/quota"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/multiformat/dao"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
	"github.com/goharbor/harbor/src/pkg/multiformat/naming"
	quotatypes "github.com/goharbor/harbor/src/pkg/quota/types"
)

// PublishEvent is one immutable version to commit (one artifact + its native
// metadata + the package-level mutable state to set).
type PublishEvent struct {
	Format    string
	ProjectID int64
	Name      string            // native package name
	Version   string            // native version string
	Payload   []byte            // exact unmodified artifact bytes (layer0)
	Meta      []byte            // per-version native metadata (canonical JSON)
	DistTags  map[string]string // full dist-tag map to set on _index (npm)
	Immutable bool              // reject re-push of a different payload
}

// mapper is the OCI mapping + projection/reconcile seam. It owns the publish
// commit path and reconcileFromIndex. Adapters never touch it directly.
type mapper struct {
	store      *store
	dao        dao.DAO
	visibility *visibility
	quota      quotaRequester
}

type quotaRequester interface {
	Request(ctx context.Context, reference, referenceID string, resources quotatypes.ResourceList, f func() error) error
}

// newMapper constructs a mapper.
func newMapper(st *store, d dao.DAO, vis *visibility) *mapper {
	return &mapper{store: st, dao: d, visibility: vis, quota: quotactl.Ctl}
}

// repo returns the OCI repo name for a package. The Harbor project name (which
// must pre-exist) is the leading segment; repos live under
// "<project>/<format>/<name>".
func (m *mapper) repo(project, format, name string) string {
	return naming.EncodeRepo(project, format, name)
}

// Publish serializes per package (advisory lock), enforces immutability against
// the authoritative _index, pushes the immutable version artifact, makes it
// UI-visible, RMW the _index (commit point), then updates the PG projection in
// one txn with an atomic proj_version bump. Returns the new proj_version.
func (m *mapper) Publish(ctx context.Context, project string, ev PublishEvent) (int64, error) {
	var projVersion int64
	resources := quotatypes.ResourceList{
		quotatypes.ResourceStorage: int64(len(ev.Payload) + len(ev.Meta)),
	}
	err := m.quota.Request(ctx, "project", strconv.FormatInt(ev.ProjectID, 10), resources, func() error {
		var err error
		projVersion, err = m.publish(ctx, project, ev)
		return err
	})
	return projVersion, err
}

func (m *mapper) publish(ctx context.Context, project string, ev PublishEvent) (int64, error) {
	repo := m.repo(project, ev.Format, ev.Name)
	lk := lockKey(ev.ProjectID, ev.Format, ev.Name)

	unlock, err := m.dao.AdvisoryLock(ctx, lk)
	if err != nil {
		return 0, err
	}
	defer unlock()

	// 1. Read authoritative _index (membership + mutable state).
	idx, _, haveIndex, err := m.store.FetchIndex(ctx, repo, IndexTag)
	if err != nil {
		return 0, errors.Wrap(err, "fetch _index")
	}

	// 2. Immutability check against _index, not OCI TagExists (tags are mutable).
	//    Discriminate on the layer0 (client-pinned) payload digest:
	//      - same bytes  -> idempotent success
	//      - diff bytes  -> reject (true immutability)
	payloadDesc := blobDescriptor(PayloadMediaType, ev.Payload)
	if haveIndex {
		if existing, found := findVersionPayload(idx, ev.Version); found && existing != "" {
			if existing == payloadDesc.Digest.String() {
				if pv, ok, err := m.dao.ProjVersion(ctx, ev.ProjectID, ev.Format, ev.Name); err == nil && ok {
					return pv, nil
				}
				return 0, nil
			}
			if ev.Immutable {
				return 0, errors.ConflictError(nil).WithMessagef("immutable: version %s already exists with different payload", ev.Version)
			}
		}
	}

	// 3. Push immutable version artifact: payload blob -> config blob -> manifest.
	if _, err := m.store.PushBlob(ctx, repo, PayloadMediaType, ev.Payload); err != nil {
		return 0, errors.Wrap(err, "push payload")
	}
	meta := ev.Meta
	if len(meta) == 0 {
		meta = []byte("{}")
	}
	configDesc, err := m.store.PushBlob(ctx, repo, ConfigMediaType(ev.Format), meta)
	if err != nil {
		return 0, errors.Wrap(err, "push config")
	}
	created := time.Now().UTC()
	// ArtifactType is left empty so the abstractor resolves the artifact type from
	// the config media type (abstractor.go falls back to manifest.Config.MediaType),
	// which selects the registered npm/maven processor. Setting it here would route
	// to the default processor and (for maven) break the []FileRef config decode.
	verManifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    configDesc,
		Layers:    []ocispec.Descriptor{payloadDesc},
		Annotations: map[string]string{
			AnnNativeName:  ev.Name,
			AnnVersion:     ev.Version,
			AnnPayloadDig:  payloadDesc.Digest.String(),
			AnnPayloadSize: fmt.Sprintf("%d", payloadDesc.Size),
			AnnCreated:     created.Format(time.RFC3339),
			// Stamp the yanked fact on the per-version manifest so the artifact
			// abstractor (npm/maven processors) can surface it via extra_attrs.
			// Publish always writes a fresh version (yanked=false); a future yank
			// mutation must preserve the prior value on re-publish.
			AnnYanked: strconv.FormatBool(false),
		},
	}
	verDesc, err := m.store.PushManifest(ctx, repo, naming.EncodeTag(ev.Version), verManifest)
	if err != nil {
		return 0, errors.Wrap(err, "push version manifest")
	}

	// 3b. UI visibility: blob accounting + repo + artifact Ensure for the
	//     per-version manifest ONLY (never the _index).
	if err := m.visibility.ensureVisible(ctx, ev.ProjectID, repo, verDesc, verManifest, naming.EncodeTag(ev.Version)); err != nil {
		return 0, errors.Wrap(err, "ensure visible")
	}

	// 4. RMW _index (commit point): replace this version's descriptor, set the
	//    mutable dist-tags. Canonical serialization (determinism).
	newIdx := upsertIndexEntry(idx, verDesc, payloadDesc, ev.Version, created, false)
	if ev.DistTags != nil {
		setDistTags(&newIdx, ev.DistTags)
	}
	idxBytes, err := canonicalIndex(newIdx)
	if err != nil {
		return 0, err
	}
	idxDesc, err := m.store.PushIndex(ctx, repo, IndexTag, idxBytes)
	if err != nil {
		return 0, errors.Wrap(err, "push _index")
	}

	// 5. Projection txn: upsert version + dist-tags, atomic proj_version bump.
	distTags := readDistTags(newIdx)
	var projVersion int64
	err = orm.WithTransaction(func(ctx context.Context) error {
		var e error
		projVersion, e = m.dao.UpsertVersion(ctx, ev.ProjectID, ev.Format, ev.Name, model.Version{
			Version:       ev.Version,
			PayloadDigest: payloadDesc.Digest.String(),
			PayloadSize:   payloadDesc.Size,
			Created:       created,
			Meta:          meta,
		}, distTags, idxDesc.Digest.String())
		return e
	})(ctx)
	if err != nil {
		return 0, err
	}
	return projVersion, nil
}

// SetDistTag re-points (version!="") or removes (version=="") a mutable dist-tag
// on a package WITHOUT republishing the artifact. It serializes per package,
// RMWs the _index dist-tags annotation, and bumps proj_version so the cached
// packument invalidates. Returns the new proj_version.
func (m *mapper) SetDistTag(ctx context.Context, project string, projectID int64, format, name, tag, version string) (int64, error) {
	repo := m.repo(project, format, name)
	lk := lockKey(projectID, format, name)

	unlock, err := m.dao.AdvisoryLock(ctx, lk)
	if err != nil {
		return 0, err
	}
	defer unlock()

	idx, _, ok, err := m.store.FetchIndex(ctx, repo, IndexTag)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.NotFoundError(nil).WithMessage("package not found")
	}
	if version != "" {
		if _, found := findVersionPayload(idx, version); !found {
			return 0, errors.NotFoundError(nil).WithMessagef("version %s not found", version)
		}
	}
	tags := readDistTags(idx)
	if version == "" {
		delete(tags, tag)
	} else {
		tags[tag] = version
	}
	setDistTags(&idx, tags)
	idxBytes, err := canonicalIndex(idx)
	if err != nil {
		return 0, err
	}
	idxDesc, err := m.store.PushIndex(ctx, repo, IndexTag, idxBytes)
	if err != nil {
		return 0, errors.Wrap(err, "push _index")
	}

	pv, err := m.dao.SetMutableState(ctx, projectID, format, name, tags, idxDesc.Digest.String())
	if err != nil {
		return 0, err
	}
	return pv, nil
}

// UpdateVersionMetadata replaces the native config for one version while
// preserving its immutable payload, creation time, and mutable package state.
func (m *mapper) UpdateVersionMetadata(ctx context.Context, project string, projectID int64, format, name, version string, meta []byte) (int64, error) {
	repo := m.repo(project, format, name)
	unlock, err := m.dao.AdvisoryLock(ctx, lockKey(projectID, format, name))
	if err != nil {
		return 0, err
	}
	defer unlock()

	idx, _, ok, err := m.store.FetchIndex(ctx, repo, IndexTag)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.NotFoundError(nil).WithMessage("package not found")
	}
	var oldDesc ocispec.Descriptor
	found := false
	for _, d := range idx.Manifests {
		if d.Annotations[AnnVersion] == version {
			oldDesc = d
			found = true
			break
		}
	}
	if !found {
		return 0, errors.NotFoundError(nil).WithMessagef("version %s not found", version)
	}
	manifest, _, err := m.store.FetchManifest(ctx, repo, oldDesc.Digest.String())
	if err != nil {
		return 0, err
	}
	configDesc, err := m.store.PushBlob(ctx, repo, ConfigMediaType(format), meta)
	if err != nil {
		return 0, errors.Wrap(err, "push config")
	}
	manifest.Config = configDesc
	verDesc, err := m.store.PushManifest(ctx, repo, naming.EncodeTag(version), manifest)
	if err != nil {
		return 0, err
	}
	if err := m.visibility.ensureVisible(ctx, projectID, repo, verDesc, manifest, naming.EncodeTag(version)); err != nil {
		return 0, errors.Wrap(err, "ensure visible")
	}
	created, _ := time.Parse(time.RFC3339, oldDesc.Annotations[AnnCreated])
	payload := ocispec.Descriptor{
		MediaType: PayloadMediaType,
		Digest:    parseDigest(oldDesc.Annotations[AnnPayloadDig]),
		Size:      parseInt(oldDesc.Annotations[AnnPayloadSize]),
	}
	newIdx := upsertIndexEntry(idx, verDesc, payload, version, created, oldDesc.Annotations[AnnYanked] == "true")
	idxBytes, err := canonicalIndex(newIdx)
	if err != nil {
		return 0, err
	}
	idxDesc, err := m.store.PushIndex(ctx, repo, IndexTag, idxBytes)
	if err != nil {
		return 0, errors.Wrap(err, "push _index")
	}
	var pv int64
	err = orm.WithTransaction(func(ctx context.Context) error {
		var e error
		pv, e = m.dao.UpsertVersion(ctx, projectID, format, name, model.Version{
			Version:       version,
			PayloadDigest: payload.Digest.String(),
			PayloadSize:   payload.Size,
			Yanked:        oldDesc.Annotations[AnnYanked] == "true",
			Created:       created,
			Meta:          meta,
		}, readDistTags(newIdx), idxDesc.Digest.String())
		return e
	})(ctx)
	return pv, err
}

// DeleteVersion removes one version from the authoritative index and derived
// projection. Unreferenced OCI content remains eligible for Harbor GC.
func (m *mapper) DeleteVersion(ctx context.Context, project string, projectID int64, format, name, version string) (int64, error) {
	repo := m.repo(project, format, name)
	unlock, err := m.dao.AdvisoryLock(ctx, lockKey(projectID, format, name))
	if err != nil {
		return 0, err
	}
	defer unlock()

	idx, _, ok, err := m.store.FetchIndex(ctx, repo, IndexTag)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, errors.NotFoundError(nil).WithMessage("package not found")
	}
	kept := idx.Manifests[:0]
	found := false
	for _, d := range idx.Manifests {
		if d.Annotations[AnnVersion] == version {
			found = true
			continue
		}
		kept = append(kept, d)
	}
	if !found {
		return 0, errors.NotFoundError(nil).WithMessagef("version %s not found", version)
	}
	idx.Manifests = kept
	tags := readDistTags(idx)
	for tag, taggedVersion := range tags {
		if taggedVersion == version {
			delete(tags, tag)
		}
	}
	setDistTags(&idx, tags)
	idxBytes, err := canonicalIndex(idx)
	if err != nil {
		return 0, err
	}
	idxDesc, err := m.store.PushIndex(ctx, repo, IndexTag, idxBytes)
	if err != nil {
		return 0, errors.Wrap(err, "push _index")
	}
	var pv int64
	err = orm.WithTransaction(func(ctx context.Context) error {
		var e error
		pv, e = m.dao.DeleteVersion(ctx, projectID, format, name, version, tags, idxDesc.Digest.String())
		return e
	})(ctx)
	return pv, err
}

// LoadState returns the projection state, reconciling from the _index if PG is
// cold (O(1): one _index GET).
func (m *mapper) LoadState(ctx context.Context, project string, projectID int64, format, name string) (model.PackageState, bool, error) {
	st, ok, err := m.dao.LoadState(ctx, projectID, format, name)
	if err != nil {
		return st, false, err
	}
	if ok {
		return st, true, nil
	}
	return m.reconcileFromIndex(ctx, project, projectID, format, name)
}

// reconcileFromIndex rebuilds the projection for one package from its OCI _index
// alone. It fetches each referenced version manifest's payload facts from the
// descriptor annotations.
func (m *mapper) reconcileFromIndex(ctx context.Context, project string, projectID int64, format, name string) (model.PackageState, bool, error) {
	repo := m.repo(project, format, name)
	idx, idxDesc, ok, err := m.store.FetchIndex(ctx, repo, IndexTag)
	if err != nil {
		return model.PackageState{}, false, err
	}
	if !ok {
		return model.PackageState{Format: format, Name: name, DistTags: map[string]string{}}, false, nil
	}
	distTags := readDistTags(idx)

	// Sort manifests so the LAST upsert wins deterministically.
	descs := append([]ocispec.Descriptor(nil), idx.Manifests...)
	sort.Slice(descs, func(i, j int) bool {
		return descs[i].Annotations[AnnVersion] < descs[j].Annotations[AnnVersion]
	})

	err = orm.WithTransaction(func(ctx context.Context) error {
		for _, d := range descs {
			ver := d.Annotations[AnnVersion]
			if ver == "" {
				continue
			}
			var meta []byte
			payloadDigest := d.Annotations[AnnPayloadDig]
			payloadSize := parseInt(d.Annotations[AnnPayloadSize])
			var files []model.FileRef
			if vm, _, err := m.store.FetchManifest(ctx, repo, d.Digest.String()); err == nil {
				if rc, err := m.store.FetchBlob(ctx, repo, vm.Config); err == nil {
					meta = readAllClose(rc)
				}
				if len(vm.Layers) > 0 && vm.Layers[0].MediaType == MavenFileMediaType {
					files = fileRefsFromLayers(vm.Layers)
				} else if len(vm.Layers) > 0 {
					payloadDigest = vm.Layers[0].Digest.String()
					payloadSize = vm.Layers[0].Size
				}
			}
			created, _ := time.Parse(time.RFC3339, d.Annotations[AnnCreated])
			v := model.Version{
				Version:       ver,
				PayloadDigest: payloadDigest,
				PayloadSize:   payloadSize,
				Yanked:        d.Annotations[AnnYanked] == "true",
				Created:       created.UTC(),
				Meta:          meta,
				Files:         files,
			}
			if _, err := m.dao.UpsertVersion(ctx, projectID, format, name, v, distTags, idxDesc.Digest.String()); err != nil {
				return err
			}
		}
		return nil
	})(ctx)
	if err != nil {
		return model.PackageState{}, false, err
	}
	return m.dao.LoadState(ctx, projectID, format, name)
}

// PayloadBlob streams the layer0 payload for a version by its stored digest.
func (m *mapper) PayloadBlob(ctx context.Context, project, format, name, dgst string, size int64) ([]byte, error) {
	repo := m.repo(project, format, name)
	rc, err := m.store.FetchBlob(ctx, repo, ocispec.Descriptor{
		MediaType: PayloadMediaType,
		Digest:    parseDigest(dgst),
		Size:      size,
	})
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readAll(rc), nil
}

// lockKey derives a stable advisory-lock key string for a package. The Postgres
// hashtext() is applied DB-side in AdvisoryLock; we just need a deterministic
// per-package string scoped by project + format + name.
func lockKey(projectID int64, format, name string) string {
	return fmt.Sprintf("multiformat:%d:%s:%s", projectID, format, name)
}

// sortManifestsByVersion sorts index manifest descriptors by their version
// annotation for canonical serialization.
func sortManifestsByVersion(ms []ocispec.Descriptor) {
	sort.Slice(ms, func(i, j int) bool {
		return ms[i].Annotations[AnnVersion] < ms[j].Annotations[AnnVersion]
	})
}
