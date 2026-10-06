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

package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

// JobServiceBackend bundles the job service handles the JobServiceCollector needs.
type JobServiceBackend struct {
	Pool      *redis.Pool
	Client    *work.Client
	Namespace string
}

// Backend supplies the collectors with the data whose access path differs
// between the standalone harbor-exporter binary and core running the
// collectors in-process.
type Backend interface {
	Health(ctx context.Context) (*responseHealth, error)
	SystemInfo(ctx context.Context) (*responseSysInfo, error)
	JobService(ctx context.Context) (*JobServiceBackend, error)
}

// restBackend reaches Harbor over its REST API and reads the job service state
// from the globals set up by InitBackendWorker. This is the standalone
// harbor-exporter behaviour.
type restBackend struct{}

// NewRESTBackend returns the backend used by the standalone exporter. It relies
// on InitHarborClient and InitBackendWorker having been called.
func NewRESTBackend() Backend {
	return restBackend{}
}

func (restBackend) Health(_ context.Context) (*responseHealth, error) {
	out := &responseHealth{}
	if err := getJSON(healthURL, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (restBackend) SystemInfo(_ context.Context) (*responseSysInfo, error) {
	out := &responseSysInfo{}
	if err := getJSON(sysInfoURL, out); err != nil {
		return nil, err
	}
	return out, nil
}

// getJSON fails closed on a non-2xx response. Decoding an error body would
// yield a zero-valued struct and publish misleading labels, e.g. reporting
// harbor_health 0 or an empty auth_mode because core answered 401.
func getJSON(path string, out any) error {
	res, err := hbrCli.Get(path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("GET %s returned %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (restBackend) JobService(_ context.Context) (*JobServiceBackend, error) {
	return &JobServiceBackend{
		Pool:      redisPool,
		Client:    jsClient,
		Namespace: jsNamespace,
	}, nil
}
