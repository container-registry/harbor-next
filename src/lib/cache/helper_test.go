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

package cache

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
)

type Foobar struct {
	Foo string
	Bar int
}

type FetchOrSaveTestSuite struct {
	suite.Suite
	ctx context.Context
}

func (suite *FetchOrSaveTestSuite) SetupSuite() {
	suite.ctx = context.TODO()
}

func (suite *FetchOrSaveTestSuite) TestFetchInternalError() {
	c := &mockCache{
		FetchFunc: func(_ context.Context, _ string, _ any) error {
			return fmt.Errorf("oops")
		},
	}

	var str string
	err := FetchOrSave(suite.ctx, c, "key", &str, func() (any, error) {
		return "str", nil
	})

	suite.Equal(fmt.Errorf("oops"), err)
}

func (suite *FetchOrSaveTestSuite) TestBuildError() {
	c := &mockCache{
		FetchFunc: func(_ context.Context, _ string, _ any) error {
			return ErrNotFound
		},
	}

	var str string
	err := FetchOrSave(suite.ctx, c, "key", &str, func() (any, error) {
		return nil, fmt.Errorf("oops")
	})

	suite.Equal(fmt.Errorf("oops"), err)
}

func (suite *FetchOrSaveTestSuite) TestSaveError() {
	c := &mockCache{
		FetchFunc: func(_ context.Context, _ string, _ any) error {
			return ErrNotFound
		},
		SaveFunc: func(_ context.Context, _ string, _ any, _ ...time.Duration) error {
			return fmt.Errorf("oops")
		},
	}

	var str string
	err := FetchOrSave(suite.ctx, c, "key", &str, func() (any, error) {
		return "str", nil
	})

	suite.Nil(err)
	suite.Equal("str", str)
}

func (suite *FetchOrSaveTestSuite) TestSaveCalledOnlyOneTime() {
	c := &mockCache{}

	var data sync.Map

	c.FetchFunc = func(_ context.Context, key string, _ any) error {
		_, ok := data.Load(key)
		if ok {
			return nil
		}

		return ErrNotFound
	}

	c.SaveFunc = func(_ context.Context, key string, value any, _ ...time.Duration) error {
		data.Store(key, value)

		return nil
	}

	var wg sync.WaitGroup

	for range 1000 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			var str string
			FetchOrSave(suite.ctx, c, "key", &str, func() (any, error) {
				return "str", nil
			})
		}()
	}

	wg.Wait()

	suite.Equal(int64(1), c.saveCalls.Load())
}

func TestFetchOrSaveTestSuite(t *testing.T) {
	suite.Run(t, new(FetchOrSaveTestSuite))
}
