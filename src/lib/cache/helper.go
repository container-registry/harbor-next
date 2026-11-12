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
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/goharbor/harbor/src/lib/log"
)

// fetchOrSaveGroup deduplicates concurrent FetchOrSave builds for the same key.
var fetchOrSaveGroup singleflight.Group

// FetchOrSave retrieves the value for the key if present in the cache.
// Otherwise, it builds the value with the builder, saves it to the cache and
// populates value with the built result. Concurrent calls for the same key
// share a single builder execution and its result.
func FetchOrSave(ctx context.Context, c Cache, key string, value any, builder func() (any, error), expiration ...time.Duration) error {
	return fetchOrSave(ctx, nil, c, key, value, builder, expiration...)
}

// FetchOrSaveWithContext is FetchOrSave that returns ctx.Err() when ctx ends before the value
// is ready, also while it waits for another caller's build of the same key. The build keeps
// running and still saves its result, and may still write value after an error is returned,
// so the caller must not use value on error.
func FetchOrSaveWithContext(ctx context.Context, c Cache, key string, value any, builder func() (any, error), expiration ...time.Duration) error {
	return fetchOrSave(ctx, ctx.Done(), c, key, value, builder, expiration...)
}

// fetchOrSave stops waiting for the build when done is closed; a nil done waits for it.
func fetchOrSave(ctx context.Context, done <-chan struct{}, c Cache, key string, value any, builder func() (any, error), expiration ...time.Duration) error {
	err := c.Fetch(ctx, key, value)
	// value found from the cache
	if err == nil {
		return nil
	}
	// internal error
	if !errors.Is(err, ErrNotFound) {
		return err
	}

	// Use singleflight to deduplicate concurrent builds for the same key: only the
	// first caller runs builder(), all concurrent callers share its result.
	groupKey := fmt.Sprintf("%p:%s", c, key)

	for {
		ch := fetchOrSaveGroup.DoChan(groupKey, func() (any, error) {
			if err := c.Fetch(ctx, key, value); err == nil {
				return codec.Encode(value)
			} else if !errors.Is(err, ErrNotFound) {
				return nil, err
			}

			val, err := builder()
			if err != nil {
				if done != nil && ctx.Err() != nil {
					return nil, &abandonedBuildError{err: err}
				}
				return nil, err
			}

			// Save with a non-cancelable context so a canceled request context (e.g. the
			// HTTP client disconnected) does not prevent the cache from being populated.
			saveCtx := context.WithoutCancel(ctx)
			if err := c.Save(saveCtx, key, val, expiration...); err != nil {
				log.Warningf("failed to save value to cache, error: %v", err)
			}

			return codec.Encode(val)
		})
		var res singleflight.Result
		select {
		case res = <-ch:
		case <-done:
			return ctx.Err()
		}
		var abandoned *abandonedBuildError
		if errors.As(res.Err, &abandoned) {
			// The caller that started the build left; a caller still waiting builds again.
			if ctx.Err() == nil {
				continue
			}
			if done != nil {
				return ctx.Err()
			}
			return abandoned.err
		}
		if res.Err != nil {
			return res.Err
		}
		return codec.Decode(res.Val.([]byte), value)
	}
}

// abandonedBuildError marks a FetchOrSaveWithContext build that failed after its caller's
// context ended, so callers sharing it can build again instead of inheriting the cancellation.
type abandonedBuildError struct {
	err error
}

func (e *abandonedBuildError) Error() string { return e.err.Error() }

func (e *abandonedBuildError) Unwrap() error { return e.err }
