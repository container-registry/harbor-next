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

package test

import (
	"context"
	"sort"
	"testing"

	"github.com/distribution/distribution/v3/registry/storage/driver"
)

// StorageRoot is the prefix distribution writes every registry path under.
const StorageRoot = "/docker/registry/v2"

// SeedFiles writes each path with a small payload so tests can assert the exact
// set of files a deletion leaves behind, independent of the distribution version.
func SeedFiles(t *testing.T, d driver.StorageDriver, paths []string) {
	t.Helper()
	for _, p := range paths {
		if err := d.PutContent(context.Background(), p, []byte(p)); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
}

// ListFiles returns every file path under root, sorted.
func ListFiles(t *testing.T, d driver.StorageDriver, root string) []string {
	t.Helper()
	var files []string
	var walk func(string)
	walk = func(p string) {
		fi, err := d.Stat(context.Background(), p)
		if err != nil {
			if _, ok := err.(driver.PathNotFoundError); ok {
				return
			}
			t.Fatalf("stat %s: %v", p, err)
		}
		if !fi.IsDir() {
			files = append(files, p)
			return
		}
		children, err := d.List(context.Background(), p)
		if err != nil {
			t.Fatalf("list %s: %v", p, err)
		}
		for _, c := range children {
			walk(c)
		}
	}
	walk(root)
	sort.Strings(files)
	return files
}

// Without returns all minus the given paths, sorted.
func Without(all []string, drop ...string) []string {
	skip := make(map[string]bool, len(drop))
	for _, d := range drop {
		skip[d] = true
	}
	var out []string
	for _, p := range all {
		if !skip[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
