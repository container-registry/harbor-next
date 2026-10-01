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
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/scheduler"
	schedulertesting "github.com/goharbor/harbor/src/testing/pkg/scheduler"
)

func TestScheduleSweepJob(t *testing.T) {
	cases := []struct {
		name       string
		stored     []*scheduler.Schedule
		reschedule bool
		unschedule bool
	}{
		{name: "no schedule", reschedule: true},
		{name: "randomized seconds, same cron", stored: []*scheduler.Schedule{{ID: 7, CRON: "37 0 0 * * *"}}},
		{name: "cron changed", stored: []*scheduler.Schedule{{ID: 7, CRON: "37 0 1 * * *"}}, unschedule: true, reschedule: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sched := schedulertesting.NewScheduler(t)
			orig := scheduler.Sched
			scheduler.Sched = sched
			t.Cleanup(func() { scheduler.Sched = orig })

			sched.On("ListSchedules", mock.Anything, mock.Anything).Return(c.stored, nil).Once()
			if c.unschedule {
				sched.On("UnScheduleByID", mock.Anything, int64(7)).Return(nil).Once()
			}
			if c.reschedule {
				sched.On("Schedule", mock.Anything, mock.Anything, mock.Anything, mock.Anything, cronSpec,
					mock.Anything, mock.Anything, mock.Anything).Return(int64(8), nil).Once()
			}

			require.NoError(t, ScheduleSweepJob(context.Background()))
		})
	}
}
