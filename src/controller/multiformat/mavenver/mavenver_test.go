package mavenver

import "testing"

// TestOrdering encodes every Aether GenericVersionScheme vector from
// VERSION-ORDERING.md §1 and DESIGN-nuget-maven.md Part 2. These are the binding
// P0 comparator corpus; each maps to a confirmed Aether test vector.
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
			t.Errorf("Compare(%q,%q)=%d, want 0 (equal)", a, b, got)
		}
		if got := Compare(Parse(b), Parse(a)); got != 0 {
			t.Errorf("Compare(%q,%q)=%d, want 0 (equal, swapped)", b, a, got)
		}
	}

	// Single-letter aliases.
	eq("1.0-alpha1", "1.0-a1")
	eq("1-alpha1", "1-a1")
	eq("1-beta1", "1-b1")
	eq("1-milestone1", "1-m1")
	// cr == rc.
	eq("1.0-cr1", "1.0-rc1")
	eq("1-rc", "1-cr")

	// trimPadding equivalences.
	eq("1", "1.0.0.0.0")
	eq("1", "1.0.0.0.0.0.0.0.0.0.0.0.0.0")
	eq("1.0.0-ga", "1")
	eq("1.0.0", "1")
	eq("1.0.0.0", "1.0")

	// Known-qualifier ranks.
	lt("1-alpha", "1-beta")
	lt("1-snapshot", "1")
	lt("2.0-rc1", "2.0-SNAPSHOT") // snapshot(-1) > rc(-2)

	// Unknown qualifier outranks base release AND every known qualifier.
	lt("1.0", "1.0-foo")    // 1.0-foo > 1.0
	lt("1.0-sp", "1.0-foo") // 1.0-foo > 1.0-sp
	lt("1", "1-abc")        // confirmed Aether vector 1-abc > 1
	lt("1-sp", "1-abc")     // 1-abc > 1-sp

	// Numeric after separator outranks a qualifier.
	lt("1.0-ga", "1.0-1") // 1.0-1 > 1.0-ga
	lt("1-ga", "1-1")

	// final (qualifier 0) < numeric at same position.
	lt("1.0-final-1", "1.0-1")

	// '.' and '-' are both separators.
	eq("1.0.alpha", "1.0-alpha")
	eq("1.0-alpha1", "1.0-alpha-1")

	// Build-up chain.
	lt("1.0-alpha", "1.0-beta")
	lt("1.0-beta", "1.0-milestone")
	lt("1.0-milestone", "1.0-rc")
	lt("1.0-rc", "1.0-snapshot")
	lt("1.0-snapshot", "1.0")
	lt("1.0", "1.0-sp")
	lt("1.0-sp", "1.0-foo")

	// Numeric value compare, leading zeros fold.
	eq("1.01", "1.1")
	lt("1.2", "1.10")
}

func TestLatestRelease(t *testing.T) {
	vs := []string{"1.0", "2.0", "1.5"}
	if got := Latest(vs); got != "2.0" {
		t.Errorf("Latest=%q want 2.0", got)
	}
	if got := Release(vs); got != "2.0" {
		t.Errorf("Release=%q want 2.0", got)
	}

	// The 1.0-foo gotcha: a stray unknown qualifier wins <latest>.
	vs2 := []string{"1.0", "1.0-foo", "1.0-sp"}
	if got := Latest(vs2); got != "1.0-foo" {
		t.Errorf("Latest=%q want 1.0-foo (unknown qualifier outranks)", got)
	}

	// Release excludes SNAPSHOT; latest includes it.
	vs3 := []string{"1.0", "1.1-SNAPSHOT", "0.9"}
	if got := Latest(vs3); got != "1.1-SNAPSHOT" {
		t.Errorf("Latest=%q want 1.1-SNAPSHOT", got)
	}
	if got := Release(vs3); got != "1.0" {
		t.Errorf("Release=%q want 1.0 (excludes snapshot)", got)
	}

	if !IsSnapshot("1.0-SNAPSHOT") || !IsSnapshot("1.0-snapshot") {
		t.Errorf("IsSnapshot should detect -SNAPSHOT case-insensitively")
	}
	if IsSnapshot("1.0") {
		t.Errorf("IsSnapshot(1.0) should be false")
	}
}
