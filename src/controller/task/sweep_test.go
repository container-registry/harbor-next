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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/scheduler"
	schedulertesting "github.com/goharbor/harbor/src/testing/pkg/scheduler"
)

func mockSweepScheduler(t *testing.T, stored []*scheduler.Schedule) *schedulertesting.Scheduler {
	m := schedulertesting.NewScheduler(t)
	orig := scheduler.Sched
	scheduler.Sched = m
	t.Cleanup(func() { scheduler.Sched = orig })
	sweepOnly := mock.MatchedBy(func(query *q.Query) bool {
		return query != nil && query.Keywords["vendor_type"] == job.ExecSweepVendorType
	})
	m.On("ListSchedules", mock.Anything, sweepOnly).Return(stored, nil).Once()
	return m
}

func expectSweepSchedule(m *schedulertesting.Scheduler) *mock.Call {
	return m.On("Schedule", mock.Anything, job.ExecSweepVendorType, int64(systemVendorID), cronTypeCustom,
		cronSpec, SchedulerCallback, nil, map[string]any(nil)).Return(int64(2), nil).Once()
}

func TestScheduleSweepJobKeepsScheduleWithRandomSeconds(t *testing.T) {
	m := mockSweepScheduler(t, []*scheduler.Schedule{
		{ID: 1, VendorType: job.ExecSweepVendorType, CRON: "34 0 0 * * *"},
	})

	require.NoError(t, ScheduleSweepJob(context.TODO()))
	m.AssertNotCalled(t, "UnScheduleByID", mock.Anything, mock.Anything)
	m.AssertNotCalled(t, "Schedule", mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestScheduleSweepJobReschedulesChangedCron(t *testing.T) {
	m := mockSweepScheduler(t, []*scheduler.Schedule{
		{ID: 1, VendorType: job.ExecSweepVendorType, CRON: "0 5 3 * * *"},
	})
	unschedule := m.On("UnScheduleByID", mock.Anything, int64(1)).Return(nil).Once()
	expectSweepSchedule(m).NotBefore(unschedule)

	require.NoError(t, ScheduleSweepJob(context.TODO()))
}

func TestScheduleSweepJobCreatesMissingSchedule(t *testing.T) {
	m := mockSweepScheduler(t, nil)
	expectSweepSchedule(m)

	require.NoError(t, ScheduleSweepJob(context.TODO()))
	m.AssertNotCalled(t, "UnScheduleByID", mock.Anything, mock.Anything)
}

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
