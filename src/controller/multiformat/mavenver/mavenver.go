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

// Package mavenver implements the Aether GenericVersionScheme comparator that
// modern Maven (`mvn` / maven-resolver) uses to order versions
// (VERSION-ORDERING.md §1; canonical source apache/maven-resolver tag
// maven-resolver-1.9.18 GenericVersion.java). multi-format-oci synthesizes
// maven-metadata.xml and computes <latest>/<release>, so it must order versions
// EXACTLY as Aether does or it resolves the wrong artifact.
//
// Algorithm summary (verbatim from the canonical impl):
//   - Tokenize on '.', '-', '_' AND at every digit<->letter boundary.
//   - Item kinds with sort rank: MAX(8) > BIGINT(5) > INT(4) > STRING(3) >
//     QUALIFIER(2) > MIN(0). A numeric item outranks an unknown string; an
//     unknown string outranks a known qualifier.
//   - Known qualifiers (case-insensitive): alpha(-5) < beta(-4) < milestone(-3)
//     < cr=rc(-2) < snapshot(-1) < ga=final=release=""(0) < sp(+1). Aliases:
//     a=alpha, b=beta, m=milestone (only when followed by a digit, matching the
//     canonical impl).
//   - trimPadding: trailing "null" items (0 numeric / "" qualifier) are removed
//     so 1 == 1.0 == 1.0.0 and 1.0.0-ga == 1.
package mavenver

import (
	"math/big"
	"strings"
)

// item kinds (canonical GenericVersion constants).
const (
	kindMax       = 8
	kindBigint    = 5
	kindInt       = 4
	kindString    = 3
	kindQualifier = 2
	kindMin       = 0
)

// known-qualifier rank map (case-insensitive). Aliases a/b/m fold to the long
// form during tokenization.
var qualifierRank = map[string]int{
	"alpha":     -5,
	"beta":      -4,
	"milestone": -3,
	"rc":        -2,
	"cr":        -2,
	"snapshot":  -1,
	"":          0,
	"ga":        0,
	"final":     0,
	"release":   0,
	"sp":        1,
}

// item is one parsed token. Exactly one of the typed fields is meaningful per
// kind. For numeric kinds we keep the big.Int (BIGINT) value; small ints fit but
// we normalize through big.Int so comparison is uniform and leading zeros fold.
type item struct {
	kind int
	num  *big.Int // INT/BIGINT
	str  string   // STRING (unknown qualifier text, lowercased)
	qual int      // QUALIFIER rank
}

// Version is a parsed Maven version. Items is the trimmed token list.
type Version struct {
	Raw   string
	Items []item
}

// Parse tokenizes a Maven version string into comparable items.
func Parse(v string) Version {
	return Version{Raw: v, Items: trimPadding(tokenize(v))}
}

// tokenize splits on separators and digit<->letter boundaries, classifying each
// run. It mirrors GenericVersion's single-pass tokenizer: a separator flushes
// the current token; a transition between digit and non-digit also flushes.
func tokenize(v string) []item {
	v = strings.ToLower(v)
	var items []item
	var cur strings.Builder
	curIsDigit := false

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		items = append(items, classify(cur.String(), curIsDigit))
		cur.Reset()
	}

	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '.' || c == '-' || c == '_' {
			flush()
			continue
		}
		isDigit := c >= '0' && c <= '9'
		if cur.Len() > 0 && isDigit != curIsDigit {
			// digit<->letter boundary inserts a token break.
			flush()
		}
		cur.WriteByte(c)
		curIsDigit = isDigit
	}
	flush()
	return items
}

// classify turns a raw run into a typed item.
func classify(s string, isDigit bool) item {
	if isDigit {
		n := new(big.Int)
		n.SetString(s, 10) // leading zeros fold; "0" -> 0
		k := kindInt
		// Aether promotes to BIGINT past the int range; the rank difference only
		// matters when comparing an INT item against a BIGINT item at the same
		// position, which big.Int comparison already handles by value. We keep the
		// kind purely so cross-kind comparisons (numeric vs string/qualifier) use
		// the right rank; both INT and BIGINT outrank STRING/QUALIFIER, and two
		// numeric items always compare by value, so kindInt is sufficient here.
		return item{kind: k, num: n}
	}
	// Alias single letters a/b/m to long qualifier forms (canonical behavior:
	// only a leading single letter maps; the run here is a pure-letter run).
	q := s
	switch s {
	case "a":
		q = "alpha"
	case "b":
		q = "beta"
	case "m":
		q = "milestone"
	}
	if rank, ok := qualifierRank[q]; ok {
		return item{kind: kindQualifier, qual: rank, str: q}
	}
	return item{kind: kindString, str: s}
}

// nullItem reports whether an item equals the "default/null" item for its kind:
// numeric zero or the "" (ga/release rank 0) qualifier. trimPadding removes a
// trailing run of these so 1.0.0 trims to 1 and 1.0-ga trims to 1.
func (it item) isNull() bool {
	switch it.kind {
	case kindInt, kindBigint:
		return it.num.Sign() == 0
	case kindQualifier:
		return it.qual == 0
	default:
		return false
	}
}

// trimPadding strips trailing null items.
func trimPadding(items []item) []item {
	end := len(items)
	for end > 0 && items[end-1].isNull() {
		end--
	}
	return items[:end:end]
}

// rank returns the cross-kind sort rank.
func (it item) rank() int { return it.kind }

// compareItem compares two items. Different kinds compare by rank; same kind
// compares by value.
func compareItem(a, b item) int {
	if a.kind != b.kind {
		return cmpInt(a.rank(), b.rank())
	}
	switch a.kind {
	case kindInt, kindBigint:
		return a.num.Cmp(b.num)
	case kindQualifier:
		return cmpInt(a.qual, b.qual)
	case kindString:
		return strings.Compare(a.str, b.str)
	default:
		return 0
	}
}

// nullOf returns the implicit null item to compare against when one version has
// run out of items at a position (padding). The kind of the present item decides
// the null: a numeric present item pads against numeric-zero; a qualifier/string
// present item pads against the "" qualifier (rank 0).
func nullFor(present item) item {
	switch present.kind {
	case kindInt, kindBigint:
		return item{kind: kindInt, num: big.NewInt(0)}
	default:
		return item{kind: kindQualifier, qual: 0, str: ""}
	}
}

// Compare returns -1/0/+1 for a<b / a==b / a>b under Aether ordering.
func Compare(a, b Version) int {
	n := len(a.Items)
	if len(b.Items) > n {
		n = len(b.Items)
	}
	for i := 0; i < n; i++ {
		var ai, bi item
		switch {
		case i < len(a.Items) && i < len(b.Items):
			ai, bi = a.Items[i], b.Items[i]
		case i < len(a.Items):
			ai = a.Items[i]
			bi = nullFor(ai)
		default:
			bi = b.Items[i]
			ai = nullFor(bi)
		}
		if c := compareItem(ai, bi); c != 0 {
			return c
		}
	}
	return 0
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

// IsSnapshot reports whether the version is a SNAPSHOT (case-insensitive suffix
// or a -SNAPSHOT qualifier token). Used for <release> (excludes snapshots) and
// to set SnapshotMutable on publish.
func IsSnapshot(v string) bool {
	return strings.HasSuffix(strings.ToUpper(v), "-SNAPSHOT") ||
		strings.ToUpper(v) == "SNAPSHOT"
}

// Latest returns the max version (highest under Aether ordering). Empty input
// yields "".
func Latest(versions []string) string {
	best := ""
	for _, v := range versions {
		if best == "" || Compare(Parse(v), Parse(best)) > 0 {
			best = v
		}
	}
	return best
}

// Release returns the max NON-snapshot version (Maven <release>). Empty if all
// are snapshots.
func Release(versions []string) string {
	best := ""
	for _, v := range versions {
		if IsSnapshot(v) {
			continue
		}
		if best == "" || Compare(Parse(v), Parse(best)) > 0 {
			best = v
		}
	}
	return best
}
