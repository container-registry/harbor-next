package naming

import (
	"regexp"
	"testing"
)

// ociRepoComponent is the distribution path-component grammar. Each '/'-separated
// component must match this; a leading '_' or '-' is illegal.
var ociRepoComponent = regexp.MustCompile(`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*$`)

var ociTag = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

func TestEncodeRepoReadableTree(t *testing.T) {
	cases := []struct{ format, name, want string }{
		{"maven", "org.springframework.boot:spring-boot-starter-test", "library/maven/org/springframework/boot/spring-boot-starter-test"},
		{"maven", "com.acme:widget2", "library/maven/com/acme/widget2"},
		{"npm", "lodash", "library/npm/lodash"},
		{"npm", "@types/node", "library/npm/types/node"},
	}
	for _, c := range cases {
		if got := EncodeRepo("library", c.format, c.name); got != c.want {
			t.Errorf("EncodeRepo(library,%s,%q) = %q, want %q", c.format, c.name, got, c.want)
		}
	}
	// Non-conforming segments (uppercase/odd) must stay grammar-legal via escape.
	for _, c := range []struct{ format, name string }{
		{"maven", "com.Example:Widget"}, {"npm", "@Scope/Name"}, {"maven", "a b:c d"},
	} {
		for _, comp := range splitSlash(EncodeRepo("library", c.format, c.name)) {
			if !ociRepoComponent.MatchString(comp) {
				t.Errorf("%s %q -> component %q violates OCI grammar", c.format, c.name, comp)
			}
		}
	}
}

func TestEncodeRepoGrammar(t *testing.T) {
	names := []string{
		"@multiformat/demo", "lodash", "@types/node", "left-pad",
		"UPPER_Case", "a.b.c", "with space", "weird@!#$%^&*()",
		"", "ünïcödé/名前", "com.example:demo",
	}
	for _, n := range names {
		repo := EncodeRepo("library", "npm", n)
		comps := splitSlash(repo)
		// Project and format segments must be emitted verbatim so Harbor can
		// resolve the owning project from the first repo component.
		if comps[0] != "library" {
			t.Errorf("name %q -> repo %q: project segment %q not verbatim", n, repo, comps[0])
		}
		if comps[1] != "npm" {
			t.Errorf("name %q -> repo %q: format segment %q not verbatim", n, repo, comps[1])
		}
		for _, comp := range comps {
			if !ociRepoComponent.MatchString(comp) {
				t.Errorf("name %q -> repo %q: component %q violates OCI grammar", n, repo, comp)
			}
		}
	}
}

func TestEncodeTagGrammar(t *testing.T) {
	versions := []string{"1.0.0", "1.0.0-beta.2", "1.0.0+build.1", "0.0.0-x_y", "@weird"}
	for _, v := range versions {
		tag := EncodeTag(v)
		if !ociTag.MatchString(tag) {
			t.Errorf("version %q -> tag %q violates OCI tag grammar", v, tag)
		}
	}
}

func TestEscapeInjective(t *testing.T) {
	// Distinct inputs must map to distinct components (collision-freedom for the
	// common case; over-long uses sha256 fallback which we don't fuzz here).
	seen := map[string]string{}
	for _, s := range []string{"@a/b", "_xa", "@a_b", "a/b", "a_x2fb"} {
		e := EscapeComponent(s)
		if prev, ok := seen[e]; ok {
			t.Errorf("collision: %q and %q both -> %q", prev, s, e)
		}
		seen[e] = s
	}
}

func splitSlash(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == '/' {
			out = append(out, cur)
			cur = ""
		} else {
			cur += string(c)
		}
	}
	return append(out, cur)
}
