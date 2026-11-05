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

package gcclient

import (
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/common/utils/test"
)

type clientTestSuite struct {
	suite.Suite
	client Client
}

func (c *clientTestSuite) SetupTest() {
	server, err := test.NewRegistryCtl(nil)
	if err != nil {
		fmt.Printf("failed to create registry: %v", err)
		os.Exit(1)
	}
	c.client = NewClient(server.URL, &Config{})
}

func (c *clientTestSuite) TestHealth() {
	c.Require().NoError(c.client.Health())
}

func (c *clientTestSuite) TestHealthFailsWithoutGCAPI() {
	// An unpatched registry has no /api/health and answers 404.
	server := test.NewServer(&test.RequestHandlerMapping{
		Method:  http.MethodGet,
		Pattern: "/v2/",
		Handler: test.Handler(&test.Response{StatusCode: http.StatusOK}),
	})
	defer server.Close()

	err := NewClient(server.URL, &Config{}).Health()
	c.Require().ErrorContains(err, "does not serve the GC API")
}

func (c *clientTestSuite) TestHealthFailsWhenUnreachable() {
	attempts, interval := healthAttempts, healthInterval
	healthAttempts, healthInterval = 2, time.Millisecond
	defer func() { healthAttempts, healthInterval = attempts, interval }()

	server := test.NewServer()
	url := server.URL
	server.Close()

	err := NewClient(url, nil).Health()
	c.Require().ErrorContains(err, "unreachable")
}

func (c *clientTestSuite) TestDeleteManifest() {
	server := test.NewServer(
		&test.RequestHandlerMapping{
			Method:  "DELETE",
			Pattern: "/api/registry/library/hello-world/manifests/latest",
			Handler: test.Handler(&test.Response{
				StatusCode: http.StatusAccepted,
			}),
		})
	defer server.Close()

	err := NewClient(server.URL, &Config{}).DeleteManifest("library/hello-world", "latest")
	c.Require().Nil(err)
}

func (c *clientTestSuite) TestDeleteBlob() {
	server := test.NewServer(
		&test.RequestHandlerMapping{
			Method:  "DELETE",
			Pattern: "/api/registry/blob/sha256:adfasa34r2sfadf234n23n4",
			Handler: test.Handler(&test.Response{
				StatusCode: http.StatusAccepted,
			}),
		})
	defer server.Close()

	err := NewClient(server.URL, &Config{}).DeleteBlob("sha256:adfasa34r2sfadf234n23n4")
	c.Require().Nil(err)
}

func TestClientTestSuite(t *testing.T) {
	suite.Run(t, &clientTestSuite{})
}
