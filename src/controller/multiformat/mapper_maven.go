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
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
	quotatypes "github.com/goharbor/harbor/src/pkg/quota/types"
)

// FileCoord identifies one file within a GAV (Maven multi-file model).
type FileCoord struct {
	Classifier  string // "" for the main artifact
	Extension   string // "jar", "pom", "war", ...
	Timestamp   string // SNAPSHOT yyyyMMdd.HHmmss ("" for release)
	BuildNumber int    // SNAPSHOT build number (0 for release)
}

// FilePublishEvent commits ONE file of a GAV as its own layer blob, RMW-ing the
// per-version multi-layer manifest and the _index across separate requests.
// Unlike PublishEvent (one immutable payload per version), a GAV version owns
// many files and SNAPSHOT versions accept appended timestamped builds without an
// immutability violation.
type FilePublishEvent struct {
	Format    string
	ProjectID int64
	Name      string // GA key: "groupId:artifactId"
	Version   string // GAV version, e.g. "1.0" or "1.0-SNAPSHOT"
	File      FileCoord
	Filename  string // exact wire filename
	Payload   []byte // exact bytes for THIS file = its own layer
	Meta      []byte // optional per-version meta (unused for Maven; manifest is authoritative)
	// SnapshotMutable: -SNAPSHOT versions accept appended timestamped files; only
	// an identical-(timestamp,filename) re-PUT with different bytes is rejected.
	SnapshotMutable bool
}

// PublishFile is the multi-file publish seam (Maven). Serialized under the
// per-package (GA-name) advisory lock, it:
//  1. pushes the file's blob (idempotent by digest);
//  2. RMW the per-version manifest: add/replace the layer for this filename
//     (idempotent by digest), enforcing immutability;
//  3. makes the version manifest UI-visible;
//  4. RMW the _index (commit point), ensuring the version descriptor;
//  5. updates the PG projection with the full file set + atomic proj_version bump.
func (m *mapper) PublishFile(ctx context.Context, project string, ev FilePublishEvent) (int64, error) {
	var projVersion int64
	resources := quotatypes.ResourceList{
		quotatypes.ResourceStorage: int64(len(ev.Payload) + len(ev.Meta)),
	}
	err := m.quota.Request(ctx, "project", strconv.FormatInt(ev.ProjectID, 10), resources, func() error {
		var err error
		projVersion, err = m.publishFile(ctx, project, ev)
		return err
	})
	return projVersion, err
}

func (m *mapper) publishFile(ctx context.Context, project string, ev FilePublishEvent) (int64, error) {
	repo := m.repo(project, ev.Format, ev.Name)
	lk := lockKey(ev.ProjectID, ev.Format, ev.Name)

	unlock, err := m.dao.AdvisoryLock(ctx, lk)
	if err != nil {
		return 0, err
	}
	defer unlock()

	// 1. Read authoritative _index.
	idx, _, haveIndex, err := m.store.FetchIndex(ctx, repo, IndexTag)
	if err != nil {
		return 0, errors.Wrap(err, "fetch _index")
	}

	// 2. Load the existing per-version manifest (if the version already has files).
	verTag := mavenVersionTag(ev.Version)
	var verManifest ocispec.Manifest
	haveVer := false
	if haveIndex {
		for _, d := range idx.Manifests {
			if d.Annotations[AnnVersion] == ev.Version {
				if vm, _, err := m.store.FetchManifest(ctx, repo, d.Digest.String()); err == nil {
					verManifest = vm
					haveVer = true
				}
				break
			}
		}
	}

	// File-layer descriptor over the EXACT bytes (client-verified digest).
	fileDesc := blobDescriptor(MavenFileMediaType, ev.Payload)
	fileDesc.Annotations = map[string]string{
		AnnFilename:    ev.Filename,
		AnnClassifier:  ev.File.Classifier,
		AnnExtension:   ev.File.Extension,
		AnnTimestamp:   ev.File.Timestamp,
		AnnBuildNumber: strconv.Itoa(ev.File.BuildNumber),
	}

	// 3. Immutability check: release repositories reject every redeploy, matching
	//    Maven repository policy. SNAPSHOT retries remain idempotent by digest;
	//    appending a new timestamped filename remains valid.
	if haveVer {
		for _, l := range verManifest.Layers {
			if l.Annotations[AnnFilename] == ev.Filename {
				if ev.SnapshotMutable && l.Digest.String() == fileDesc.Digest.String() {
					if pv, ok, err := m.dao.ProjVersion(ctx, ev.ProjectID, ev.Format, ev.Name); err == nil && ok {
						return pv, nil
					}
					return 0, nil
				}
				return 0, errors.ConflictError(nil).WithMessagef("immutable: file %s already exists", ev.Filename)
			}
		}
	}

	// 4. Push the file blob.
	if _, err := m.store.PushBlob(ctx, repo, MavenFileMediaType, ev.Payload); err != nil {
		return 0, errors.Wrap(err, "push maven file")
	}

	// 5. RMW the per-version manifest: add/replace the layer, sort by filename for
	//    a stable canonical manifest.
	created := time.Now().UTC()
	layers := append([]ocispec.Descriptor(nil), verManifest.Layers...)
	replaced := false
	for i := range layers {
		if layers[i].Annotations[AnnFilename] == ev.Filename {
			layers[i] = fileDesc
			replaced = true
			break
		}
	}
	if !replaced {
		layers = append(layers, fileDesc)
	}
	sort.Slice(layers, func(i, j int) bool {
		return layers[i].Annotations[AnnFilename] < layers[j].Annotations[AnnFilename]
	})

	// Config blob = the structured file manifest list (deterministic JSON). It
	// lets reconcile rebuild Files without fetching every layer.
	fileList := fileRefsFromLayers(layers)
	cfgBytes, err := json.Marshal(fileList)
	if err != nil {
		return 0, err
	}
	cfgDesc, err := m.store.PushBlob(ctx, repo, ConfigMediaType(ev.Format), cfgBytes)
	if err != nil {
		return 0, errors.Wrap(err, "push maven version config")
	}
	if haveVer {
		if c, ok := verManifest.Annotations[AnnCreated]; ok {
			if t, err := time.Parse(time.RFC3339, c); err == nil {
				created = t.UTC()
			}
		}
	}
	// ArtifactType is left empty so the abstractor resolves the artifact type from
	// the config media type, selecting the registered maven processor. Setting it
	// here would route to the default processor, which decodes the config blob into
	// a map and fails on the []FileRef JSON array.
	newVerManifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    cfgDesc,
		Layers:    layers,
		Annotations: map[string]string{
			AnnNativeName: ev.Name,
			AnnVersion:    ev.Version,
			AnnCreated:    created.Format(time.RFC3339),
		},
	}
	verDesc, err := m.store.PushManifest(ctx, repo, verTag, newVerManifest)
	if err != nil {
		return 0, errors.Wrap(err, "push maven version manifest")
	}

	// 5b. UI visibility for the per-version manifest ONLY (never the _index).
	if err := m.visibility.ensureVisible(ctx, ev.ProjectID, repo, verDesc, newVerManifest, verTag); err != nil {
		return 0, errors.Wrap(err, "ensure visible")
	}

	// 6. RMW _index (commit point). The version descriptor carries no single
	//    payload digest (multi-file); reconcile reads the manifest layers.
	if verDesc.Annotations == nil {
		verDesc.Annotations = map[string]string{}
	}
	verDesc.Annotations[AnnVersion] = ev.Version
	verDesc.Annotations[AnnCreated] = created.Format(time.RFC3339)
	newIdx := idx
	found := false
	for i := range newIdx.Manifests {
		if newIdx.Manifests[i].Annotations[AnnVersion] == ev.Version {
			newIdx.Manifests[i] = verDesc
			found = true
			break
		}
	}
	if !found {
		newIdx.Manifests = append(newIdx.Manifests, verDesc)
	}
	idxBytes, err := canonicalIndex(newIdx)
	if err != nil {
		return 0, err
	}
	idxDesc, err := m.store.PushIndex(ctx, repo, IndexTag, idxBytes)
	if err != nil {
		return 0, errors.Wrap(err, "push _index")
	}

	// 7. Projection txn: upsert the version with its full Files set, atomic bump.
	distTags := readDistTags(newIdx)
	var projVersion int64
	err = orm.WithTransaction(func(ctx context.Context) error {
		var e error
		v := model.Version{
			Version: ev.Version,
			Created: created,
			Meta:    cfgBytes,
			Files:   fileList,
		}
		projVersion, e = m.dao.UpsertVersion(ctx, ev.ProjectID, ev.Format, ev.Name, v, distTags, idxDesc.Digest.String())
		return e
	})(ctx)
	if err != nil {
		return 0, err
	}
	return projVersion, nil
}

// fileRefsFromLayers projects the manifest's file-layer descriptors into the
// typed FileRef list (sorted by filename, matching the manifest layer order).
func fileRefsFromLayers(layers []ocispec.Descriptor) []model.FileRef {
	out := make([]model.FileRef, 0, len(layers))
	for _, l := range layers {
		bn, _ := strconv.Atoi(l.Annotations[AnnBuildNumber])
		out = append(out, model.FileRef{
			Filename:    l.Annotations[AnnFilename],
			Classifier:  l.Annotations[AnnClassifier],
			Extension:   l.Annotations[AnnExtension],
			Timestamp:   l.Annotations[AnnTimestamp],
			BuildNumber: bn,
			Digest:      l.Digest.String(),
			Size:        l.Size,
		})
	}
	return out
}

// MavenFileBlob streams one file's layer bytes by digest (exact unmodified).
func (m *mapper) MavenFileBlob(ctx context.Context, project, format, name, dgst string, size int64) ([]byte, error) {
	repo := m.repo(project, format, name)
	rc, err := m.store.FetchBlob(ctx, repo, ocispec.Descriptor{
		MediaType: MavenFileMediaType,
		Digest:    parseDigest(dgst),
		Size:      size,
	})
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return readAll(rc), nil
}

// mavenVersionTag derives the OCI tag for a GAV version manifest. Versions like
// "1.0-SNAPSHOT" are already tag-legal; we keep it inline to avoid an import
// cycle and match EncodeTag's allowed set ([A-Za-z0-9._-]).
func mavenVersionTag(version string) string {
	b := make([]byte, 0, len(version)+8)
	for i := 0; i < len(version); i++ {
		c := version[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-':
			b = append(b, c)
		default:
			b = append(b, []byte(fmt.Sprintf("_x%02x", c))...)
		}
	}
	if len(b) == 0 {
		return "_e"
	}
	return string(b)
}
