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
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/blob/models"
)

func TestShouldTouchNone(t *testing.T) {
	cases := []struct {
		name       string
		window     string // GC_TIME_WINDOW_HOURS
		updateTime time.Time
		expected   bool
	}{
		{name: "fresh blob inside the skip age", window: "2", updateTime: time.Now(), expected: false},
		{name: "blob just inside the skip age", window: "2", updateTime: time.Now().Add(-4 * time.Minute), expected: false},
		{name: "blob just beyond the skip age", window: "2", updateTime: time.Now().Add(-6 * time.Minute), expected: true},
		// The skip age does not scale with the window: a blob half a window old
		// is touched, so the margin a skip leaves never falls to window/2.
		{name: "blob at half the window is still touched", window: "2", updateTime: time.Now().Add(-1 * time.Hour), expected: true},
		{name: "blob older than full window", window: "2", updateTime: time.Now().Add(-3 * time.Hour), expected: true},
		// The smallest window the hours-granular setting allows.
		{name: "one hour window still skips a fresh blob", window: "1", updateTime: time.Now(), expected: false},
		{name: "one hour window touches a blob past the skip age", window: "1", updateTime: time.Now().Add(-6 * time.Minute), expected: true},
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

// TestShouldTouchNoneLivenessMargin pins the property the skip has to preserve:
// GC collects an unassociated blob once update_time <= now() - window, so a
// skipped Touch must still leave enough margin for the push that skipped it.
// The margin a skip leaves is window minus the row's age, and the bound on that
// age is what this asserts - it must not scale with the window, or a small
// window silently shrinks the margin to a fraction of itself.
func TestShouldTouchNoneLivenessMargin(t *testing.T) {
	for _, hours := range []string{"1", "2", "6", "24"} {
		t.Run("window="+hours+"h", func(t *testing.T) {
			t.Setenv("GC_TIME_WINDOW_HOURS", hours)

			window, err := time.ParseDuration(hours + "h")
			require.NoError(t, err)

			// Walk the whole window a minute at a time and take the worst
			// margin over every age the skip actually accepts.
			worst := window
			for age := time.Duration(0); age <= window; age += time.Minute {
				bb := &models.Blob{Status: models.StatusNone, UpdateTime: time.Now().Add(-age)}
				if shouldTouchNone(bb) {
					continue // Touch runs, update_time is reset, margin is a full window.
				}
				if margin := window - age; margin < worst {
					worst = margin
				}
			}

			assert.GreaterOrEqual(t, worst, window-touchSkipMaxAge,
				"skipping a Touch must not cost more than touchSkipMaxAge of GC liveness margin")
		})
	}
}
