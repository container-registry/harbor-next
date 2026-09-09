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

package client

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

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

func (c *clientTestSuite) TesHealth() {
	err := c.client.Health()
	c.Require().Nil(err)
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

func (c *clientTestSuite) TestStorage() {
	server := test.NewServer(
		&test.RequestHandlerMapping{
			Method:  "GET",
			Pattern: "/api/registry/storage",
			Handler: test.Handler(&test.Response{
				StatusCode: http.StatusOK,
				Body:       []byte(`{"driver":"filesystem","supported":true,"path":"/storage","total":100,"free":40,"used":60,"measured_at":"2026-09-08T12:00:00Z"}`),
			}),
		})
	defer server.Close()

	info, err := NewClient(server.URL, &Config{}).Storage(context.Background())
	c.Require().Nil(err)
	c.Equal("filesystem", info.Driver)
	c.True(info.Supported)
	c.Equal("/storage", info.Path)
	c.Equal(uint64(100), info.Total)
	c.Equal(uint64(40), info.Free)
	c.Equal(uint64(60), info.Used)
	c.Equal(2026, info.MeasuredAt.Year())
}

func (c *clientTestSuite) TestStorageErrors() {
	// non-2xx
	server := test.NewServer(
		&test.RequestHandlerMapping{
			Method:  "GET",
			Pattern: "/api/registry/storage",
			Handler: test.Handler(&test.Response{StatusCode: http.StatusInternalServerError, Body: []byte("boom")}),
		})
	_, err := NewClient(server.URL, &Config{}).Storage(context.Background())
	c.Require().NotNil(err)
	c.Contains(err.Error(), "boom")
	server.Close()

	// malformed body
	server = test.NewServer(
		&test.RequestHandlerMapping{
			Method:  "GET",
			Pattern: "/api/registry/storage",
			Handler: test.Handler(&test.Response{StatusCode: http.StatusOK, Body: []byte("{not json")}),
		})
	_, err = NewClient(server.URL, &Config{}).Storage(context.Background())
	c.Require().NotNil(err)
	server.Close()

	// transport failure: the server is gone
	_, err = NewClient(server.URL, &Config{}).Storage(context.Background())
	c.Require().NotNil(err)

	// canceled context
	server = test.NewServer(
		&test.RequestHandlerMapping{
			Method:  "GET",
			Pattern: "/api/registry/storage",
			Handler: test.Handler(&test.Response{StatusCode: http.StatusOK, Body: []byte("{}")}),
		})
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = NewClient(server.URL, &Config{}).Storage(ctx)
	c.Require().NotNil(err)
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
