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

package pkgstore

import "testing"

func TestRepository(t *testing.T) {
	store := &Store{Format: Format{RepositoryPrefix: "npm/"}}
	tests := []struct {
		name        string
		packageName string
		want        string
	}{
		{
			name:        "unscoped package",
			packageName: "react",
			want:        "library/npm/react",
		},
		{
			name:        "scoped package",
			packageName: "@angular/core",
			want:        "library/npm/angular/core",
		},
		{
			name:        "scoped package with hyphen",
			packageName: "@scope/pkg-name",
			want:        "library/npm/scope/pkg-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := store.Repository("library", tt.packageName); got != tt.want {
				t.Fatalf("Repository() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVersionTag(t *testing.T) {
	store := &Store{}
	if got := store.VersionTag("16.2.12"); got != "16.2.12" {
		t.Fatalf("VersionTag() = %q, want 16.2.12", got)
	}
}

func TestPublishTags(t *testing.T) {
	store := &Store{}
	tags, err := store.publishTags("16.2.12", []string{"latest", "latest", "beta"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"16.2.12", "latest", "beta"}
	if len(tags) != len(want) {
		t.Fatalf("publishTags() = %v, want %v", tags, want)
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("publishTags() = %v, want %v", tags, want)
		}
	}
}

func TestPublishTagsEncodesInvalidVersionTag(t *testing.T) {
	store := &Store{Format: Format{VersionTagPrefix: "test-"}}
	tags, err := store.publishTags("1.0.0+build", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || !validTag(tags[0]) || tags[0] == "1.0.0+build" {
		t.Fatalf("publishTags() = %v, want encoded valid tag", tags)
	}
}

func TestPublishTagsRejectInvalidAlias(t *testing.T) {
	store := &Store{}
	if _, err := store.publishTags("1.0.0", []string{"bad+alias"}); err == nil {
		t.Fatal("publishTags() error = nil, want invalid alias error")
	}
}

func TestHasTag(t *testing.T) {
	if !hasTag([]string{"1.0.0", "latest"}, "1.0.0") {
		t.Fatal("hasTag() = false, want true")
	}
	if hasTag([]string{"latest"}, "1.0.0") {
		t.Fatal("hasTag() = true, want false")
	}
	if hasTag(nil, "1.0.0") {
		t.Fatal("hasTag(nil) = true, want false")
	}
}
