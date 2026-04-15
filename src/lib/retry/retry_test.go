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

package retry

import (
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
)

func TestAbort(t *testing.T) {
	assert := assert.New(t)

	e1 := Abort(nil)
	assert.Equal("retry abort", e1.Error())

	e2 := Abort(fmt.Errorf("failed to call func"))
	assert.Equal("retry abort, error: failed to call func", e2.Error())
}

// advanceUntilDone starts a goroutine that advances the mock clock in
// small increments until the done channel is closed. This unblocks any
// Sleep or After calls inside Retry without requiring real wall time.
func advanceUntilDone(mock *clock.Mock, step time.Duration, done <-chan struct{}) {
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				mock.Add(step)
				// yield to let Retry process the tick
				time.Sleep(time.Millisecond)
			}
		}
	}()
}

func TestRetry(t *testing.T) {
	assert := assert.New(t)

	// Test 1: always-failing function hits timeout
	{
		i := 0
		f := func() error {
			i++
			return fmt.Errorf("failed")
		}
		mock := clock.NewMock()
		done := make(chan struct{})
		advanceUntilDone(mock, time.Second, done)

		assert.Error(Retry(f,
			InitialInterval(time.Second),
			MaxInterval(time.Second),
			Timeout(5*time.Second),
			WithClock(mock),
		))
		close(done)
		assert.GreaterOrEqual(i, 2)
	}

	// Test 2: immediately succeeding function
	{
		f := func() error { return nil }
		assert.Nil(Retry(f, WithClock(clock.NewMock())))
	}

	// Test 3: function succeeds after a few retries
	{
		i := 0
		f := func() error {
			defer func() { i++ }()
			if i < 2 {
				return fmt.Errorf("failed")
			}
			return nil
		}
		mock := clock.NewMock()
		done := make(chan struct{})
		advanceUntilDone(mock, 100*time.Millisecond, done)

		assert.Nil(Retry(f, WithClock(mock)))
		close(done)
	}

	// Test 4: callback is invoked on each failure
	{
		var cbCount int
		f := func() error { return fmt.Errorf("failed") }
		mock := clock.NewMock()
		done := make(chan struct{})
		advanceUntilDone(mock, time.Second, done)

		Retry(f,
			Timeout(5*time.Second),
			WithClock(mock),
			Callback(func(err error, sleep time.Duration) {
				cbCount++
			}),
		)
		close(done)
		assert.Greater(cbCount, 0)
	}

	// Test 5: always-failing produces correct error message
	{
		mock := clock.NewMock()
		done := make(chan struct{})
		advanceUntilDone(mock, time.Second, done)

		err := Retry(func() error {
			return fmt.Errorf("always failed")
		}, Timeout(time.Minute), WithClock(mock))
		close(done)

		assert.Error(err)
		assert.Equal("retry timeout: always failed", err.Error())
	}

	// Test 6: abort stops retrying immediately
	{
		i := 0
		f := func() error {
			if i == 3 {
				return Abort(fmt.Errorf("abort"))
			}
			i++
			return fmt.Errorf("error")
		}
		mock := clock.NewMock()
		done := make(chan struct{})
		advanceUntilDone(mock, time.Second, done)

		assert.Error(Retry(f,
			InitialInterval(time.Second),
			MaxInterval(time.Second),
			Timeout(5*time.Second),
			WithClock(mock),
		))
		close(done)
		assert.Equal(3, i)
	}
}
