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

// Package model holds the format-agnostic value types that flow between the
// adapters, the projection, and the synthesizers. SynthFunc consumes a typed
// PackageState rather than a bare []Version so mutable aggregate state
// (dist-tags, yanked) is first-class.
package model

import "time"

// Version is one immutable published version of a package, as projected from the
// OCI `_index` control artifact. Native-format-specific facts that the
// synthesizer needs are carried in Meta (verbatim, deterministic).
type Version struct {
	// Version is the native version string exactly as published (e.g. "1.0.0",
	// "@scope" is part of the package name, not here).
	Version string
	// PayloadDigest is the OCI digest of layer0 = the exact unmodified artifact
	// bytes the client pinned (no recompression). Used to fetch the blob on
	// download.
	PayloadDigest string
	// PayloadSize is the artifact byte length.
	PayloadSize int64
	// Yanked is a PG-authoritative mutable fact. Reconcile must not clobber it
	// from annotations.
	Yanked bool
	// Created is the publish time (UTC), rendered as RFC3339 for determinism.
	Created time.Time
	// Meta is the per-version native metadata blob (canonical JSON bytes) that
	// the synthesizer splices into the rendered index. For npm this is the
	// version's package.json manifest as published.
	Meta []byte
	// Files is the multi-file member set for formats whose version owns many
	// payload blobs (Maven GAV: POM + main + classified + SNAPSHOT timestamped
	// builds). Empty for single-payload formats, which use PayloadDigest. Each
	// FileRef points at its own layer blob (exact client-verified bytes).
	Files []FileRef
}

// FileRef is one file inside a multi-file version (Maven GAV member). Each file
// is its own OCI layer blob; Digest/Size are the exact unmodified bytes the
// client checksum-verifies.
type FileRef struct {
	// Filename is the exact wire filename (e.g. "demo-1.0.jar",
	// "demo-1.0-sources.jar", "demo-1.0-20240101.120000-1.jar").
	Filename string `json:"filename"`
	// Classifier is the Maven classifier ("" for the main artifact, "sources",
	// "javadoc", ...).
	Classifier string `json:"classifier,omitempty"`
	// Extension is the file extension without the dot ("jar", "pom", "war", ...).
	Extension string `json:"extension"`
	// Timestamp is the SNAPSHOT build timestamp "yyyyMMdd.HHmmss" ("" for a
	// release file).
	Timestamp string `json:"timestamp,omitempty"`
	// BuildNumber is the SNAPSHOT build number (0 for a release file).
	BuildNumber int `json:"buildNumber,omitempty"`
	// Digest is the OCI digest of the file's layer blob (the exact bytes).
	Digest string `json:"digest"`
	// Size is the file byte length.
	Size int64 `json:"size"`
}

// PackageState is the full typed aggregate state of one package: its membership
// (Versions) plus mutable package-level facts (DistTags). It is the sole input
// to a SynthFunc besides config.
type PackageState struct {
	// Format is the native format identifier ("npm", ...).
	Format string
	// Name is the native package name exactly as published (case-sensitive,
	// scoped names include "@scope/").
	Name string
	// ProjVersion is the monotonic per-package projection counter used in the
	// cache key. PG-authoritative; bumped atomically in the publish txn.
	ProjVersion int64
	// Versions is the membership, unordered here; the synthesizer applies the
	// format's canonical comparator.
	Versions []Version
	// DistTags maps a tag name to a version string. PG-authoritative mutable
	// state (npm). "latest" is NOT computed as max — it is whatever the
	// publisher set.
	DistTags map[string]string
}
