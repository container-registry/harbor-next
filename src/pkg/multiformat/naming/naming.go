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

// Package naming implements the formal charset codec: deterministic,
// collision-free, OCI-grammar-conformant. Correctness does NOT depend on
// reversibility — the exact native identity is stored authoritatively in OCI
// metadata + PG. Decode is provided only as a convenience for debugging.
package naming

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"regexp"
	"strings"
)

// OCI repository name grammar (distribution): components match
// [a-z0-9]+(?:(?:[._]|__|[-]+)[a-z0-9]+)* separated by '/'. To stay safely
// inside it we restrict the *encoded* alphabet to lowercase alnum plus the
// escape marker, joining segments with '/'.

// maxComponentLen bounds an encoded path component before we fall back to the
// hash form. distribution allows long names but we keep components compact and
// well within the 255-char total-ref limit.
const maxComponentLen = 96

// componentPrefix guarantees every encoded component starts with a lowercase
// letter. The OCI repository grammar forbids a component starting with '_' or
// '-', so a name like "@scope" (which escapes to "_x40scope") must be prefixed.
// 'm' (multiformat) is reserved as the marker; it is injective because the rest of
// the component is the escape-encoded original.
const componentPrefix = "m"

// EscapeComponent encodes one native identifier segment into a single
// OCI-repo-safe path component. The mapping is injective: a constant 'm' prefix
// keeps the leading char a letter, then every byte that is not unreserved
// lowercase-alnum is escaped as _xNN (two lowercase hex digits); the literal '_'
// is escaped as _x5f so the scheme is unambiguous. Uppercase letters are escaped
// too (OCI repo names are lowercase-only), keeping the encoding injective after
// the registry's implicit lowercasing.
func EscapeComponent(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	b.WriteString(componentPrefix)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "_x%02x", c)
		}
	}
	out := b.String()
	if len(out) > maxComponentLen {
		// Over-long fallback: m + h + base32(sha256(original)), lowercase, no pad.
		sum := sha256.Sum256([]byte(s))
		enc := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
		out = componentPrefix + "h" + enc
	}
	return out
}

// cleanComponent matches an OCI repo path-component that is already readable and
// grammar-legal (a subset of the full grammar: single separators only). Such a
// segment is emitted verbatim; anything else falls back to the injective escaper.
var cleanComponent = regexp.MustCompile(`^[a-z0-9]+([._-][a-z0-9]+)*$`)

// componentOrEscape passes a readable, grammar-legal lowercase segment through
// unchanged, otherwise escapes it (uppercase, '@', ':', spaces, unicode, ...).
func componentOrEscape(s string) string {
	if cleanComponent.MatchString(s) {
		return s
	}
	return EscapeComponent(s)
}

// nameSegments splits a native package identifier into READABLE repo path
// segments, mirroring how each ecosystem lays artifacts out on disk:
//   - maven "groupId:artifactId" -> groupId dot-parts as directories + artifactId
//     (e.g. "org.springframework.boot:spring-boot-starter-test" ->
//     org/springframework/boot/spring-boot-starter-test), matching Maven/Nexus.
//   - npm "@scope/name" -> scope + name (drop the grammar-illegal '@'); plain
//     names stay a single segment.
//   - any other/unknown format -> a single segment (escaped as needed).
func nameSegments(format, nativeName string) []string {
	switch format {
	case "maven":
		if g, a, ok := strings.Cut(nativeName, ":"); ok {
			return append(strings.Split(g, "."), a)
		}
	case "npm":
		if scopeAndName, ok := strings.CutPrefix(nativeName, "@"); ok {
			if scope, name, ok := strings.Cut(scopeAndName, "/"); ok {
				return []string{scope, name}
			}
		}
	}
	return []string{nativeName}
}

// EncodeRepo maps (project, format, nativeName) to an OCI repository path
// "<project>/<format>/<readable native tree>".
//
// project and format are emitted VERBATIM (project is an existing Harbor project
// — Harbor resolves the owning project from the first path component, so escaping
// it would point at a non-existent project; format is a fixed lowercase token).
// The native identity is laid out as a readable tree (see nameSegments) so the
// stored repo name is human-meaningful everywhere — registry catalog, API, CLI,
// logs — not just in a decoded UI. Each segment is escaped only when it is not
// already grammar-legal lowercase, keeping the common case readable and the rare
// case (uppercase/odd chars) injective. Coordinates are stored authoritatively in
// the projection (multi_format_package.native_name), so the repo name is never decoded back.
func EncodeRepo(project, format, nativeName string) string {
	parts := []string{project, format}
	for _, seg := range nameSegments(format, nativeName) {
		parts = append(parts, componentOrEscape(seg))
	}
	return strings.Join(parts, "/")
}

// EncodeTag maps a native version string to an OCI tag. OCI tag grammar:
// [a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}. We reuse the injective _xNN escaper but
// allow '.' and '-' through unescaped since they are tag-legal and common in
// versions, keeping tags readable for debugging. The leading char is guaranteed
// legal because EscapeComponent output begins with alnum or '_'.
func EncodeTag(version string) string {
	var b strings.Builder
	b.Grow(len(version) + 8)
	for i := 0; i < len(version); i++ {
		c := version[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-':
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "_x%02x", c)
		}
	}
	out := b.String()
	if out == "" {
		out = "_e"
	}
	if len(out) > 127 {
		sum := sha256.Sum256([]byte(version))
		enc := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
		out = "v_h" + enc
		if len(out) > 128 {
			out = out[:128]
		}
	}
	return out
}
