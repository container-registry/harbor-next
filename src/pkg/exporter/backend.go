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

	"github.com/gocraft/work"
	"github.com/gomodule/redigo/redis"
)

// JobServiceBackend bundles the job service handles the JobServiceCollector needs.
type JobServiceBackend struct {
	Pool      *redis.Pool
	Client    *work.Client
	Namespace string
}

// Backend supplies the collectors with the data they scrape. Core provides the
// in-process implementation in backend_local.go; tests provide fakes.
type Backend interface {
	Health(ctx context.Context) (*responseHealth, error)
	SystemInfo(ctx context.Context) (*responseSysInfo, error)
	JobService(ctx context.Context) (*JobServiceBackend, error)
}
