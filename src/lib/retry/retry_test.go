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

	"github.com/stretchr/testify/assert"
)

func TestAbort(t *testing.T) {
	assert := assert.New(t)

	e1 := Abort(nil)
	assert.Equal("retry abort", e1.Error())

	e2 := Abort(fmt.Errorf("failed to call func"))
	assert.Equal("retry abort, error: failed to call func", e2.Error())
}

func TestRetry(t *testing.T) {
	assert := assert.New(t)

	initialInterval := 5 * time.Millisecond
	maxInterval := 5 * time.Millisecond
	timeout := 25 * time.Millisecond

	i := 0
	f1 := func() error {
		i++
		return fmt.Errorf("failed")
	}
	assert.Error(Retry(f1, InitialInterval(initialInterval), MaxInterval(maxInterval), Timeout(timeout)))
	// f1 called time     0ms - sleep - 5ms - sleep - 10ms - sleep - 15ms - sleep - 20ms - sleep - 25ms
	// i after f1 called  1            2            3            4            5            6
	// the i may be 5 or 6 depend on timeout or default which is selected by the select statement
	assert.LessOrEqual(i, 6)

	f2 := func() error {
		return nil
	}
	assert.Nil(Retry(f2))

	i = 0
	f3 := func() error {
		defer func() {
			i++
		}()

		if i < 2 {
			return fmt.Errorf("failed")
		}
		return nil
	}
	assert.Nil(Retry(f3))

	Retry(
		f1,
		InitialInterval(initialInterval),
		MaxInterval(maxInterval),
		Timeout(timeout),
		Callback(func(err error, sleep time.Duration) {
			fmt.Printf("failed to exec f1 retry after %s : %v\n", sleep, err)
		}),
	)

	err := Retry(func() error {
		return fmt.Errorf("always failed")
	}, InitialInterval(initialInterval), MaxInterval(maxInterval), Timeout(timeout))

	assert.Error(err)
	assert.Equal("retry timeout: always failed", err.Error())

	i = 0
	f4 := func() error {
		if i == 3 {
			return Abort(fmt.Errorf("abort"))
		}

		i++
		return fmt.Errorf("error")
	}
	assert.Error(Retry(f4, InitialInterval(initialInterval), MaxInterval(maxInterval), Timeout(timeout)))
	assert.LessOrEqual(i, 3)
}
