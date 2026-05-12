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

package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/distribution/manifest/schema2"
	digest "github.com/opencontainers/go-digest"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/goharbor/harbor/src/lib/log"
	accessorymodel "github.com/goharbor/harbor/src/pkg/accessory/model"
	pkgartifact "github.com/goharbor/harbor/src/pkg/artifact"
)

const (
	buildKitReferenceTypeAnnotation         = "vnd.docker.reference.type"
	buildKitReferenceDigestAnnotation       = "vnd.docker.reference.digest"
	buildKitAttestationManifestType         = "attestation-manifest"
	inTotoLayerMediaType                    = "application/vnd.in-toto+json"
	maxBuildKitStatementBytes         int64 = 4 << 20 // 4 MiB
)

type buildKitStatement struct {
	Subject []buildKitSubject `json:"subject"`
}

type buildKitSubject struct {
	Name   string            `json:"name"`
	Digest map[string]string `json:"digest"`
}

func (a *abstractor) toBuildKitAttestationCandidate(ctx context.Context, repository string, descriptor v1.Descriptor, siblings []v1.Descriptor) (*pkgartifact.AccessoryCandidate, error) {
	if !isBuildKitAttestationDescriptor(descriptor) {
		return nil, nil
	}

	subjects, err := a.loadBuildKitAttestationSubjects(ctx, repository, descriptor.Digest.String())
	if err != nil {
		log.G(ctx).Debugf("could not load BuildKit attestation subjects for %s@%s, falling back to annotation-based resolution: %v", repository, descriptor.Digest.String(), err)
	}

	targetDigest := resolveBuildKitAttestationSubject(descriptor, siblings, subjects)
	if targetDigest == "" {
		return nil, nil
	}

	accessoryArt, err := a.artMgr.GetByDigest(ctx, repository, descriptor.Digest.String())
	if err != nil {
		return nil, err
	}
	targetArt, err := a.artMgr.GetByDigest(ctx, repository, targetDigest)
	if err != nil {
		return nil, err
	}

	return &pkgartifact.AccessoryCandidate{
		ArtifactID:        accessoryArt.ID,
		SubArtifactID:     targetArt.ID,
		SubArtifactRepo:   repository,
		SubArtifactDigest: targetDigest,
		Digest:            accessoryArt.Digest,
		Size:              accessoryArt.Size,
		Type:              accessorymodel.TypeInTotoAttestation,
	}, nil
}

func (a *abstractor) loadBuildKitAttestationSubjects(ctx context.Context, repository, digestRef string) ([]buildKitSubject, error) {
	manifest, _, err := a.regCli.PullManifest(repository, digestRef)
	if err != nil {
		return nil, err
	}

	mediaType, content, err := manifest.Payload()
	if err != nil {
		return nil, err
	}
	if mediaType != v1.MediaTypeImageManifest && mediaType != schema2.MediaTypeManifest {
		return nil, fmt.Errorf("unexpected BuildKit attestation media type %s", mediaType)
	}

	imageManifest := &v1.Manifest{}
	if err := json.Unmarshal(content, imageManifest); err != nil {
		return nil, err
	}

	for _, layer := range imageManifest.Layers {
		if layer.MediaType != inTotoLayerMediaType {
			continue
		}
		_, blob, err := a.regCli.PullBlob(repository, layer.Digest.String())
		if err != nil {
			return nil, err
		}
		defer blob.Close()

		if layer.Size > 0 && layer.Size > maxBuildKitStatementBytes {
			return nil, fmt.Errorf("in-toto payload too large: %d", layer.Size)
		}
		payload, err := io.ReadAll(io.LimitReader(blob, maxBuildKitStatementBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) > maxBuildKitStatementBytes {
			return nil, fmt.Errorf("in-toto payload exceeds %d bytes", maxBuildKitStatementBytes)
		}

		statement := &buildKitStatement{}
		if err := json.Unmarshal(payload, statement); err != nil {
			return nil, err
		}
		return statement.Subject, nil
	}

	return nil, fmt.Errorf("no in-toto payload found in %s", digestRef)
}

func isBuildKitAttestationDescriptor(descriptor v1.Descriptor) bool {
	if descriptor.MediaType != v1.MediaTypeImageManifest && descriptor.MediaType != schema2.MediaTypeManifest {
		return false
	}
	return descriptor.Annotations[buildKitReferenceTypeAnnotation] == buildKitAttestationManifestType
}

func resolveBuildKitAttestationSubject(descriptor v1.Descriptor, siblings []v1.Descriptor, subjects []buildKitSubject) string {
	platformChildren := buildKitPlatformChildren(siblings)
	if len(platformChildren) == 0 {
		return ""
	}

	if digestRef := descriptor.Annotations[buildKitReferenceDigestAnnotation]; buildKitDigestInIndex(platformChildren, digestRef) {
		return digestRef
	}

	for _, subject := range subjects {
		for _, digestRef := range buildKitSubjectDigests(subject) {
			if buildKitDigestInIndex(platformChildren, digestRef) {
				return digestRef
			}
		}
	}

	for _, subject := range subjects {
		if digestRef := buildKitDigestBySubjectName(platformChildren, subject.Name); digestRef != "" {
			return digestRef
		}
	}

	return ""
}

func buildKitPlatformChildren(siblings []v1.Descriptor) []v1.Descriptor {
	children := make([]v1.Descriptor, 0, len(siblings))
	for _, sibling := range siblings {
		if isBuildKitAttestationDescriptor(sibling) {
			continue
		}
		children = append(children, sibling)
	}
	return children
}

func buildKitDigestInIndex(siblings []v1.Descriptor, digestRef string) bool {
	if digestRef == "" {
		return false
	}
	for _, sibling := range siblings {
		if sibling.Digest.String() == digestRef {
			return true
		}
	}
	return false
}

func buildKitSubjectDigests(subject buildKitSubject) []string {
	digests := make([]string, 0, len(subject.Digest))
	for algorithm, encoded := range subject.Digest {
		if encoded == "" {
			continue
		}
		digestRef := digest.NewDigestFromEncoded(digest.Algorithm(algorithm), encoded)
		if digestRef.Validate() == nil {
			digests = append(digests, digestRef.String())
		}
	}
	return digests
}

func buildKitDigestBySubjectName(siblings []v1.Descriptor, name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	match := ""
	for _, sibling := range siblings {
		if !buildKitPlatformMatchesName(sibling.Platform, name) {
			continue
		}
		if match != "" {
			return ""
		}
		match = sibling.Digest.String()
	}
	return match
}

func buildKitPlatformMatchesName(platform *v1.Platform, name string) bool {
	if platform == nil {
		return false
	}

	candidates := []string{
		platform.Architecture,
	}
	if platform.OS != "" && platform.Architecture != "" {
		candidates = append(candidates, platform.OS+"/"+platform.Architecture)
	}
	if platform.OS != "" && platform.Architecture != "" && platform.Variant != "" {
		candidates = append(candidates, platform.OS+"/"+platform.Architecture+"/"+platform.Variant)
	}

	for _, candidate := range candidates {
		if candidate != "" && strings.EqualFold(candidate, name) {
			return true
		}
	}
	return false
}
