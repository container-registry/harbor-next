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
	"context"
	"strings"
	"sync"
	"time"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/server/registry/pkgproxy"
)

const mutableTTL = 5 * time.Minute

// Fetch retrieves one checksum database protocol path from its authority.
type Fetch func(context.Context) (*pkgproxy.Response, error)

// Mirror implements persistent read-through checksum database proxying.
type Mirror struct {
	store Store
	now   func() time.Time
	locks sync.Map
}

// NewMirror creates a persistent checksum database mirror.
func NewMirror(store Store) *Mirror {
	return &Mirror{store: store, now: time.Now}
}

// Resolve returns a persistent response or fetches and stores an upstream miss.
func (m *Mirror) Resolve(ctx context.Context, projectID int64, project string, database Database, requestPath string, fetch Fetch) (*Response, error) {
	unlock := m.lock(project, database, requestPath)
	defer unlock()

	stored, err := m.store.Open(ctx, project, database, requestPath)
	if err != nil && !errors.IsNotFoundErr(err) {
		return nil, err
	}
	mutable := IsMutable(requestPath)
	if stored != nil && (!mutable || m.now().Sub(stored.FetchedAt) < mutableTTL) {
		return stored, nil
	}

	upstream, err := fetch(ctx)
	if err != nil {
		if stored != nil {
			return stored, nil
		}
		return nil, err
	}
	response := &Response{
		Body:        upstream.Body,
		ContentType: upstream.ContentType,
		FetchedAt:   m.now().UTC(),
	}
	if err := m.store.Put(ctx, projectID, project, database, requestPath, response); err != nil {
		log.Warningf("failed to persist Go checksum response %s/%s: %v", database.Name, requestPath, err)
	}
	return response, nil
}

// IsMutable reports whether a checksum endpoint can change over time.
func IsMutable(requestPath string) bool {
	return requestPath == "latest" || strings.Contains(requestPath, ".p/")
}

func (m *Mirror) lock(project string, database Database, requestPath string) func() {
	key := project + "\x00" + database.Name + "\x00" + database.URL + "\x00" + requestPath
	value, _ := m.locks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}
