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

package gosum

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	harborerrors "github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

func TestMirrorPersistsImmutableResponse(t *testing.T) {
	store := newMemoryStore()
	mirror := NewMirror(store)
	var fetches atomic.Int64
	fetch := func(context.Context) (*pkgproxy.Response, error) {
		fetches.Add(1)
		return &pkgproxy.Response{Body: []byte("signed record\n"), ContentType: "text/plain"}, nil
	}

	database := Database{Name: "sum.golang.org", URL: "https://sum.golang.org"}
	first, err := mirror.Resolve(context.Background(), 7, "proxy", database, "lookup/example.com/mod@v1.0.0", fetch)
	require.NoError(t, err)
	second, err := mirror.Resolve(context.Background(), 7, "proxy", database, "lookup/example.com/mod@v1.0.0", func(context.Context) (*pkgproxy.Response, error) {
		return nil, harborerrors.New("upstream unavailable")
	})
	require.NoError(t, err)
	require.Equal(t, first.Body, second.Body)
	require.Equal(t, "text/plain", second.ContentType)
	require.Equal(t, int64(1), fetches.Load())
}

func TestMirrorRefreshesMutableResponseWithPersistentFallback(t *testing.T) {
	store := newMemoryStore()
	mirror := NewMirror(store)
	now := time.Date(2026, 7, 10, 1, 0, 0, 0, time.UTC)
	mirror.now = func() time.Time { return now }

	database := Database{Name: "sum.golang.org", URL: "https://sum.golang.org"}
	first, err := mirror.Resolve(context.Background(), 7, "proxy", database, "latest", func(context.Context) (*pkgproxy.Response, error) {
		return &pkgproxy.Response{Body: []byte("tree one")}, nil
	})
	require.NoError(t, err)
	require.Equal(t, []byte("tree one"), first.Body)

	now = now.Add(mutableTTL + time.Second)
	refreshed, err := mirror.Resolve(context.Background(), 7, "proxy", database, "latest", func(context.Context) (*pkgproxy.Response, error) {
		return &pkgproxy.Response{Body: []byte("tree two")}, nil
	})
	require.NoError(t, err)
	require.Equal(t, []byte("tree two"), refreshed.Body)

	now = now.Add(mutableTTL + time.Second)
	fallback, err := mirror.Resolve(context.Background(), 7, "proxy", database, "latest", func(context.Context) (*pkgproxy.Response, error) {
		return nil, harborerrors.New("upstream unavailable")
	})
	require.NoError(t, err)
	require.Equal(t, []byte("tree two"), fallback.Body)
}

func TestMirrorCoalescesConcurrentMisses(t *testing.T) {
	store := newMemoryStore()
	mirror := NewMirror(store)
	var fetches atomic.Int64
	fetch := func(context.Context) (*pkgproxy.Response, error) {
		fetches.Add(1)
		time.Sleep(10 * time.Millisecond)
		return &pkgproxy.Response{Body: []byte("tile")}, nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response, err := mirror.Resolve(context.Background(), 7, "proxy", Database{Name: "sum.golang.org"}, "tile/8/0/001", fetch)
			if err != nil {
				errs <- err
				return
			}
			if !bytes.Equal(response.Body, []byte("tile")) {
				errs <- fmt.Errorf("unexpected response %q", response.Body)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(1), fetches.Load())
}

func TestIsMutable(t *testing.T) {
	require.True(t, IsMutable("latest"))
	require.True(t, IsMutable("tile/8/0/001.p/3"))
	require.False(t, IsMutable("lookup/example.com/mod@v1.0.0"))
	require.False(t, IsMutable("tile/8/0/001"))
}

type memoryStore struct {
	mu        sync.RWMutex
	responses map[string]*Response
}

func newMemoryStore() *memoryStore {
	return &memoryStore{responses: map[string]*Response{}}
}

func (s *memoryStore) Open(_ context.Context, project string, database Database, requestPath string) (*Response, error) {
	s.mu.RLock()
	response, ok := s.responses[storeKey(project, database, requestPath)]
	s.mu.RUnlock()
	if !ok {
		return nil, harborerrors.NotFoundError(nil).WithMessage("checksum response not found")
	}
	clone := *response
	clone.Body = bytes.Clone(response.Body)
	return &clone, nil
}

func (s *memoryStore) Put(_ context.Context, _ int64, project string, database Database, requestPath string, response *Response) error {
	clone := *response
	clone.Body = bytes.Clone(response.Body)
	s.mu.Lock()
	s.responses[storeKey(project, database, requestPath)] = &clone
	s.mu.Unlock()
	return nil
}

func storeKey(project string, database Database, requestPath string) string {
	return project + "\x00" + database.Name + "\x00" + database.URL + "\x00" + requestPath
}
