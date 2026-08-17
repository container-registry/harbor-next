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

// Package multiformat is the OCI mapping layer for native package formats (npm,
// Maven) served on top of Harbor's OCI backend. It turns native publish events
// into the per-version immutable manifest + the per-package `_index` control
// artifact, and drives the PG projection. It is the only place that assembles
// OCI manifests (adapters never reach this). Three invariants it enforces:
//
//  1. The wire index is ONLY synthesizer output; it is never derived from blob
//     or annotation layout. multiformat produces projection rows, not wire bytes.
//  2. layer0 = the exact unmodified artifact bytes = the digest the client pins.
//     No wire recompression.
//  3. Mutable post-publish facts (npm dist-tags, yanked) are PG-authoritative
//     and live in the _index annotations; reconcile never clobbers them.
package multiformat

const (
	// IndexTag is the reserved per-package tag for the `_index` control artifact.
	IndexTag = "_index"

	// IndexArtifactType distinguishes the multiformat control index from a real
	// multi-arch image index so generic OCI tooling does not misrender it and it
	// never leaks to a native client.
	IndexArtifactType = "application/vnd.harbor.multiformat.index.v1+json"

	// PayloadMediaType is the media type for layer0, the raw artifact bytes.
	PayloadMediaType = "application/vnd.harbor.multiformat.payload.v1"

	// MavenFileMediaType is the media type for a Maven file layer (POM/jar/etc).
	// Each file in a GAV is its own layer blob; this distinguishes the multi-file
	// model from the single-payload PayloadMediaType.
	MavenFileMediaType = "application/vnd.harbor.maven.file.v1"

	// FormatNPM / FormatMaven are the native format identifiers used as the
	// middle repo segment and the projection format column.
	FormatNPM   = "npm"
	FormatMaven = "maven"
)

// Per-version manifest config media types. They are FORMAT-SPECIFIC because the
// Harbor abstractor derives the artifact type label from the config media type
// (default processor regexp ^application/vnd\.[^.]*\.(.*)\.config\.[^.]*\+json$
// -> ToUpper(group1)), and the npm/maven processors register on these exact
// literals. Do NOT collapse them to a single media type.
const (
	// NpmConfigMediaType marks the per-version manifest config blob for npm.
	NpmConfigMediaType = "application/vnd.harbor.npm.config.v1+json"

	// MavenConfigMediaType marks the per-version manifest config blob for Maven.
	MavenConfigMediaType = "application/vnd.harbor.maven.config.v1+json"
)

// ConfigMediaType returns the format-specific per-version config media type.
// Unknown formats fall back to the npm media type shape so the abstractor still
// derives a sane type label.
func ConfigMediaType(format string) string {
	switch format {
	case FormatMaven:
		return MavenConfigMediaType
	case FormatNPM:
		return NpmConfigMediaType
	default:
		return "application/vnd.harbor." + format + ".config.v1+json"
	}
}

// Annotation keys on the `_index` (mutable package state, PG-authoritative
// source of truth) and on per-version manifests (flat facts for reconcile).
const (
	AnnNativeName  = "vnd.harbor.multiformat.native-name"
	AnnVersion     = "vnd.harbor.multiformat.version"
	AnnDistTags    = "vnd.harbor.multiformat.dist-tags" // JSON object on _index
	AnnYanked      = "vnd.harbor.multiformat.yanked"    // "true"/"false" per version desc
	AnnPayloadDig  = "vnd.harbor.multiformat.payload-digest"
	AnnPayloadSize = "vnd.harbor.multiformat.payload-size"
	AnnCreated     = "vnd.harbor.multiformat.created" // RFC3339 UTC

	// Per-file annotations on a Maven multi-file version manifest's layer
	// descriptors. Each file layer carries enough to reconstruct the GAV file set
	// and synthesize maven-metadata.xml from the _index/manifest alone.
	AnnFilename    = "vnd.harbor.maven.filename"
	AnnClassifier  = "vnd.harbor.maven.classifier"
	AnnExtension   = "vnd.harbor.maven.extension"
	AnnTimestamp   = "vnd.harbor.maven.timestamp"    // SNAPSHOT yyyyMMdd.HHmmss
	AnnBuildNumber = "vnd.harbor.maven.build-number" // SNAPSHOT build number
)
