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

	"github.com/docker/distribution"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	blobctl "github.com/goharbor/harbor/src/controller/blob"
	"github.com/goharbor/harbor/src/controller/repository"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
)

// visibility replicates the blob accounting + repo/artifact Ensure path that the
// real OCI push middleware (src/server/middleware/blob/put_manifest.go) performs
// after a PUT manifest. Pushing through registry.Cli directly BYPASSES that
// accounting, so without this an artifact would list but GC would later sweep
// its blobs. It is invoked AFTER each per-version manifest push, NEVER for the
// _index (the _index is never Ensure'd, so it stays UI-invisible).
type visibility struct {
	blobCtl blobctl.Controller
	repoCtl repository.Controller
	// artEnsure is the artifact.Ctl.Ensure closure; kept as a func so the
	// concrete controller type stays an implementation detail.
	artEnsure func(ctx context.Context, repository, digest string, tags []string) (bool, int64, error)
}

// ensureVisible runs the load-bearing UI-visibility path for one per-version
// manifest. Order replicates the real OCI push path:
//  1. blob accounting: Sync the manifest's references, associate each blob with
//     the project, Ensure the manifest blob + associate it with the project,
//     associate all blobs with the artifact;
//  2. repository.Ctl.Ensure (MUST precede artifact Ensure);
//  3. artifact.Ctl.Ensure with the version tag (the UI-visibility entry point).
func (v *visibility) ensureVisible(ctx context.Context, projectID int64, repo string, manifestDesc ocispec.Descriptor, manifest ocispec.Manifest, tag string) error {
	logger := log.G(ctx)

	references := manifestReferences(manifest)

	// Sync blobs (config + layers) — creates blob rows when absent, updates
	// content type when present.
	if err := v.blobCtl.Sync(ctx, references); err != nil {
		return errors.Wrap(err, "sync blobs")
	}

	// Associate each referenced blob with the project (associations may have been
	// cleaned up by others).
	for _, ref := range references {
		if err := v.blobCtl.AssociateWithProjectByDigest(ctx, ref.Digest.String(), projectID); err != nil {
			return errors.Wrap(err, "associate blob with project")
		}
	}

	// Ensure the manifest blob and associate it with the project.
	blobID, err := v.blobCtl.Ensure(ctx, manifestDesc.Digest.String(), manifestDesc.MediaType, manifestDesc.Size)
	if err != nil {
		return errors.Wrap(err, "ensure manifest blob")
	}
	if err := v.blobCtl.AssociateWithProjectByID(ctx, blobID, projectID); err != nil {
		return errors.Wrap(err, "associate manifest blob with project")
	}

	// Associate all referenced blobs with the artifact (manifest digest).
	var blobDigests []string
	for _, ref := range references {
		blobDigests = append(blobDigests, ref.Digest.String())
	}
	if err := v.blobCtl.AssociateWithArtifact(ctx, blobDigests, manifestDesc.Digest.String()); err != nil {
		return errors.Wrap(err, "associate blobs with artifact")
	}

	// repository.Ctl.Ensure MUST run before artifact.Ctl.Ensure (ensureArtifact
	// looks up the repo row and errors if absent).
	if _, _, err := v.repoCtl.Ensure(ctx, repo); err != nil {
		return errors.Wrap(err, "ensure repository")
	}

	if _, _, err := v.artEnsure(ctx, repo, manifestDesc.Digest.String(), []string{tag}); err != nil {
		return errors.Wrap(err, "ensure artifact")
	}

	logger.Debugf("multiformat: ensured artifact %s@%s tag=%s visible", repo, manifestDesc.Digest.String(), tag)
	return nil
}

// manifestReferences converts an OCI manifest's config + layer descriptors into
// docker/distribution descriptors for blob accounting.
func manifestReferences(m ocispec.Manifest) []distribution.Descriptor {
	refs := make([]distribution.Descriptor, 0, len(m.Layers)+1)
	refs = append(refs, toDistDescriptor(m.Config))
	for _, l := range m.Layers {
		refs = append(refs, toDistDescriptor(l))
	}
	return refs
}

func toDistDescriptor(d ocispec.Descriptor) distribution.Descriptor {
	return distribution.Descriptor{
		MediaType: d.MediaType,
		Size:      d.Size,
		Digest:    d.Digest,
	}
}
