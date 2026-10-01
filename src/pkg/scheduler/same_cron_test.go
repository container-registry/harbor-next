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

package scheduler

import "testing"

func TestSameCron(t *testing.T) {
	cases := []struct {
		stored, requested string
		want              bool
	}{
		{"37 0 0 * * *", "0 0 0 * * *", true},
		{"0 0 0 * * *", "0 0 0 * * *", true},
		{"12  0 0 * * *", "0 0 0 * * *", true},
		{"37 0 1 * * *", "0 0 0 * * *", false},
		{"37 0 0 * * 1", "0 0 0 * * *", false},
		{"0 0 * * *", "0 0 * * *", false},
		{"", "0 0 0 * * *", false},
		{"37 0 0 * * *", "", false},
	}
	for _, c := range cases {
		if got := SameCron(c.stored, c.requested); got != c.want {
			t.Errorf("SameCron(%q, %q) = %v, want %v", c.stored, c.requested, got, c.want)
		}
	}
}
