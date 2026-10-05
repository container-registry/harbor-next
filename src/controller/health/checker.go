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

package health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/v2/client/orm"
	"github.com/docker/distribution/health"

	httputil "github.com/goharbor/harbor/src/common/http"
	"github.com/goharbor/harbor/src/common/utils"
	"github.com/goharbor/harbor/src/lib/cache"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
)

// HTTPStatusCodeHealthChecker implements a Checker to check that the HTTP status code
// returned matches the expected one
func HTTPStatusCodeHealthChecker(method string, url string, header http.Header,
	timeout time.Duration, statusCode int) health.Checker {
	client := &http.Client{
		Transport: httputil.GetHTTPTransport(),
		Timeout:   timeout,
	}
	return httpHealthChecker(client, method, url, header, statusCode)
}

func httpHealthChecker(client *http.Client, method string, url string, header http.Header,
	statusCodes ...int) health.Checker {
	return health.CheckFunc(func() error {
		req, err := http.NewRequest(method, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %v", err)
		}
		for key, values := range header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to check health: %v", err)
		}
		defer resp.Body.Close()
		if !slices.Contains(statusCodes, resp.StatusCode) {
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Debugf("failed to read response body: %v", err)
			}
			return fmt.Errorf("received unexpected status code: %d %s", resp.StatusCode, string(data))
		}

		return nil
	})
}

type updater struct {
	sync.Mutex
	status error
}

func (u *updater) Check() error {
	u.Lock()
	defer u.Unlock()

	return u.status
}

func (u *updater) update(status error) {
	u.Lock()
	defer u.Unlock()

	u.status = status
}

// PeriodicHealthChecker implements a Checker to check status periodically
func PeriodicHealthChecker(checker health.Checker, period time.Duration) health.Checker {
	u := &updater{
		// init the "status" as "unknown status" error to avoid returning nil error(which means healthy)
		// before the first health check request finished
		status: errors.New("unknown status"),
	}

	go func() {
		ticker := time.NewTicker(period)
		for {
			u.update(checker.Check())
			<-ticker.C
		}
	}()

	return u
}

func coreHealthChecker() health.Checker {
	return health.CheckFunc(func() error {
		return nil
	})
}

func portalHealthChecker() health.Checker {
	period := 10 * time.Second
	return PeriodicHealthChecker(portalHTTPChecker(config.GetPortalURL(), 60*time.Second), period)
}

// portalHTTPChecker treats a redirect as healthy without following it. With TLS
// the portal redirects plain HTTP to the external HTTPS port, which nothing
// listens on inside the container network.
func portalHTTPChecker(url string, timeout time.Duration) health.Checker {
	client := &http.Client{
		Transport: httputil.GetHTTPTransport(),
		Timeout:   timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return httpHealthChecker(client, http.MethodGet, url, nil,
		http.StatusOK, http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect)
}

func jobserviceHealthChecker() health.Checker {
	url := config.InternalJobServiceURL() + "/api/v1/stats"
	timeout := 60 * time.Second
	period := 10 * time.Second
	checker := HTTPStatusCodeHealthChecker(http.MethodGet, url, nil, timeout, http.StatusOK)
	return PeriodicHealthChecker(checker, period)
}

func registryHealthChecker() health.Checker {
	url := getRegistryURL() + "/"
	timeout := 60 * time.Second
	period := 10 * time.Second
	checker := HTTPStatusCodeHealthChecker(http.MethodGet, url, nil, timeout, http.StatusOK)
	return PeriodicHealthChecker(checker, period)
}

func registryCtlHealthChecker() health.Checker {
	url := config.GetRegistryCtlURL() + "/api/health"
	timeout := 60 * time.Second
	period := 10 * time.Second
	checker := HTTPStatusCodeHealthChecker(http.MethodGet, url, nil, timeout, http.StatusOK)
	return PeriodicHealthChecker(checker, period)
}

func databaseHealthChecker() health.Checker {
	period := 10 * time.Second
	checker := health.CheckFunc(func() error {
		_, err := orm.NewOrm().Raw("SELECT 1").Exec()
		if err != nil {
			return fmt.Errorf("failed to run SQL \"SELECT 1\": %v", err)
		}
		return nil
	})
	return PeriodicHealthChecker(checker, period)
}

func redisHealthChecker() health.Checker {
	period := 10 * time.Second
	checker := health.CheckFunc(func() error {
		return cache.Default().Ping(context.TODO())
	})
	return PeriodicHealthChecker(checker, period)
}

func trivyHealthChecker() health.Checker {
	url := strings.TrimSuffix(config.TrivyAdapterURL(), "/") + "/probe/healthy"
	timeout := 60 * time.Second
	period := 10 * time.Second
	checker := HTTPStatusCodeHealthChecker(http.MethodGet, url, nil, timeout, http.StatusOK)
	return PeriodicHealthChecker(checker, period)
}

// RegisterHealthCheckers ...
func RegisterHealthCheckers() {
	registry["core"] = coreHealthChecker()
	registry["portal"] = portalHealthChecker()
	registry["jobservice"] = jobserviceHealthChecker()
	registry["registry"] = registryHealthChecker()
	registry["registryctl"] = registryCtlHealthChecker()
	registry["database"] = databaseHealthChecker()
	registry["redis"] = redisHealthChecker()
	if config.WithTrivy() {
		registry["trivy"] = trivyHealthChecker()
	}
}

func getRegistryURL() string {
	endpoint, err := config.RegistryURL()
	if err != nil {
		log.Errorf("failed to get the URL of registry: %v", err)
		return ""
	}
	url, err := utils.ParseEndpoint(endpoint)
	if err != nil {
		log.Errorf("failed to parse the URL of registry: %v", err)
		return ""
	}
	return url.String()
}
