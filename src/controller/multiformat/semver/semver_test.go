package semver

import "testing"

// TestOrdering encodes the VERSION-ORDERING.md §2 (npm/node-semver) edge-case
// table as executable assertions. This is the binding P0 comparator corpus.
func TestOrdering(t *testing.T) {
	lt := func(a, b string) {
		t.Helper()
		if got := Compare(Parse(a), Parse(b)); got != -1 {
			t.Errorf("Compare(%q,%q)=%d, want -1 (a<b)", a, b, got)
		}
		if got := Compare(Parse(b), Parse(a)); got != 1 {
			t.Errorf("Compare(%q,%q)=%d, want 1 (b>a)", b, a, got)
		}
	}
	eq := func(a, b string) {
		t.Helper()
		if got := Compare(Parse(a), Parse(b)); got != 0 {
			t.Errorf("Compare(%q,%q)=%d, want 0 (equal precedence)", a, b, got)
		}
	}

	// Canonical SPEC chain (§2).
	lt("1.0.0-alpha", "1.0.0-alpha.1")
	lt("1.0.0-alpha.1", "1.0.0-alpha.beta")
	lt("1.0.0-alpha.beta", "1.0.0-beta")
	lt("1.0.0-beta", "1.0.0-beta.2")
	lt("1.0.0-beta.2", "1.0.0-beta.11")
	lt("1.0.0-beta.11", "1.0.0-rc.1")
	lt("1.0.0-rc.1", "1.0.0")

	// Edge-case table.
	lt("1.0.0-beta.2", "1.0.0-beta.11")  // numeric prerelease compared as ints
	lt("1.0.0-1", "1.0.0-alpha")         // numeric < alphanumeric
	eq("1.0.0+build.1", "1.0.0+build.2") // build ignored
	lt("1.0.0-alpha", "1.0.0-alpha.1")   // more identifiers > fewer

	// Non-3-part is invalid.
	if Parse("1.0").Valid {
		t.Errorf("Parse(\"1.0\") should be invalid (SemVer requires 3 parts)")
	}
	if Parse("1").Valid {
		t.Errorf("Parse(\"1\") should be invalid")
	}
	if !Parse("1.0.0").Valid {
		t.Errorf("Parse(\"1.0.0\") should be valid")
	}

	// Max picks the highest valid release.
	if got := Max([]string{"1.0.0-beta.2", "1.0.0-beta.11", "1.0.0"}); got != "1.0.0" {
		t.Errorf("Max = %q, want 1.0.0", got)
	}
}
