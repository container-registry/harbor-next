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

package feature

import "testing"

func TestPortalBookmarksIsBetaOptionalFeature(t *testing.T) {
	definition, ok := definitionFor(func(d Definition) bool { return d.Feature == PortalBookmarks })
	if !ok {
		t.Fatal("portal bookmarks feature is not registered")
	}
	if definition.ConfigKey != "enable_portal_bookmarks" || definition.EnvKey != "HARBOR_ENABLE_PORTAL_BOOKMARKS" {
		t.Fatalf("unexpected keys %+v", definition)
	}
	if definition.Section != OptionalSection || !definition.Beta || definition.Default {
		t.Fatalf("want a beta optional feature that is off by default, got %+v", definition)
	}
	if Sections()[0].ID != OptionalSection {
		t.Fatalf("optional section must lead the page, got %+v", Sections())
	}
}
