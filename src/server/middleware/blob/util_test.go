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

package blob

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/pkg/blob/models"
)

func TestShouldTouchNone(t *testing.T) {
	cases := []struct {
		name       string
		window     string // GC_TIME_WINDOW_HOURS
		updateTime time.Time
		expected   bool
	}{
		{name: "fresh blob within half window", window: "2", updateTime: time.Now(), expected: false},
		{name: "blob just inside half window", window: "2", updateTime: time.Now().Add(-59 * time.Minute), expected: false},
		{name: "stale blob beyond half window", window: "2", updateTime: time.Now().Add(-61 * time.Minute), expected: true},
		{name: "blob older than full window", window: "2", updateTime: time.Now().Add(-3 * time.Hour), expected: true},
		{name: "zero window leaves no safety margin", window: "0", updateTime: time.Now(), expected: true},
		{name: "negative window leaves no safety margin", window: "-1", updateTime: time.Now(), expected: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("GC_TIME_WINDOW_HOURS", c.window)
			bb := &models.Blob{Status: models.StatusNone, UpdateTime: c.updateTime}
			assert.Equal(t, c.expected, shouldTouchNone(bb))
		})
	}
}
