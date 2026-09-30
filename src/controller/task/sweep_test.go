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

package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSameCronIgnoringSeconds(t *testing.T) {
	cases := []struct {
		name   string
		stored string
		want   bool
	}{
		{name: "identical", stored: "0 0 0 * * *", want: true},
		{name: "random seconds", stored: "34 0 0 * * *", want: true},
		{name: "max seconds", stored: "59 0 0 * * *", want: true},
		{name: "extra whitespace", stored: " 34  0 0 * * * ", want: true},
		{name: "different minute", stored: "34 5 0 * * *", want: false},
		{name: "different hour", stored: "34 0 3 * * *", want: false},
		{name: "different day of month", stored: "34 0 0 1 * *", want: false},
		{name: "different month", stored: "34 0 0 * 1 *", want: false},
		{name: "different day of week", stored: "34 0 0 * * 1", want: false},
		{name: "five fields", stored: "0 0 * * *", want: false},
		{name: "seven fields", stored: "0 0 0 * * * *", want: false},
		{name: "seconds only", stored: "34", want: false},
		{name: "empty", stored: "", want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, sameCronIgnoringSeconds(c.stored, cronSpec))
		})
	}
}
