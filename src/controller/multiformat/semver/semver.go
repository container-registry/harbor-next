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

// Package semver implements SemVer 2.0.0 strict precedence as node-semver uses
// it (VERSION-ORDERING.md §2/§5: npm and Cargo share this comparator). Build
// metadata is ignored for precedence; prerelease < release; numeric prerelease
// identifiers compare numerically; numeric < alphanumeric; more identifiers >
// fewer when all preceding equal. Non-3-part input is rejected (Valid==false).
package semver

import (
	"strconv"
	"strings"
)

// Version is a parsed SemVer 2.0.0 version.
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string // prerelease identifiers, nil if none
	Valid               bool
}

// Parse parses a strict 3-part SemVer string. Returns Valid==false on any
// deviation (missing parts, non-numeric core, leading zeros in numeric idents
// are tolerated per node-semver's lenient core but core must be 3 numeric parts).
func Parse(s string) Version {
	v := Version{}
	// Split build metadata (ignored).
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core := s
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		core = s[:i]
		pre = s[i+1:]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return v
	}
	var err error
	if v.Major, err = strconv.ParseUint(parts[0], 10, 64); err != nil {
		return v
	}
	if v.Minor, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
		return v
	}
	if v.Patch, err = strconv.ParseUint(parts[2], 10, 64); err != nil {
		return v
	}
	if pre != "" {
		v.Pre = strings.Split(pre, ".")
	}
	v.Valid = true
	return v
}

// Compare returns -1, 0, +1 by SemVer 2.0.0 precedence. Invalid versions sort
// below valid ones (and equal to each other) so they never become latest.
func Compare(a, b Version) int {
	if !a.Valid || !b.Valid {
		switch {
		case a.Valid && !b.Valid:
			return 1
		case !a.Valid && b.Valid:
			return -1
		default:
			return 0
		}
	}
	if c := cmpUint(a.Major, b.Major); c != 0 {
		return c
	}
	if c := cmpUint(a.Minor, b.Minor); c != 0 {
		return c
	}
	if c := cmpUint(a.Patch, b.Patch); c != 0 {
		return c
	}
	return comparePre(a.Pre, b.Pre)
}

// comparePre applies SemVer §11 prerelease precedence. A version with no
// prerelease has higher precedence than one with a prerelease.
func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0: // a is release, b is prerelease
		return 1
	case len(b) == 0:
		return -1
	}
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if c := comparePreIdent(a[i], b[i]); c != 0 {
			return c
		}
	}
	// All preceding equal: longer set has higher precedence.
	return cmpInt(len(a), len(b))
}

func comparePreIdent(a, b string) int {
	an, aNum := toNum(a)
	bn, bNum := toNum(b)
	switch {
	case aNum && bNum:
		return cmpUint(an, bn)
	case aNum && !bNum: // numeric < alphanumeric
		return -1
	case !aNum && bNum:
		return 1
	default:
		return strings.Compare(a, b) // ASCII
	}
}

func toNum(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func cmpUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Max returns the highest version string by SemVer precedence, or "" if none are
// valid. Used only as the *default* latest when no dist-tag is set — npm's
// latest is otherwise the mutable dist-tag (VERSION-ORDERING §2).
func Max(versions []string) string {
	best := ""
	var bestV Version
	for _, s := range versions {
		v := Parse(s)
		if !v.Valid {
			continue
		}
		if best == "" || Compare(v, bestV) > 0 {
			best, bestV = s, v
		}
	}
	return best
}
