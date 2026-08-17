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

package maven

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	regmultiformat "github.com/goharbor/harbor/src/server/registry/multiformat"
)

type recordingFilePublisher struct {
	event regmultiformat.FilePublishEvent
}

func (p *recordingFilePublisher) PublishFile(_ context.Context, _ string, event regmultiformat.FilePublishEvent) (int64, error) {
	p.event = event
	return 1, nil
}

func TestPublishDeniedByProjectPolicy(t *testing.T) {
	handler := &handler{
		authorizePush: func(_ context.Context, project, registryType string) error {
			if project != "proxy" {
				t.Errorf("project = %q, want proxy", project)
			}
			if registryType != "maven" {
				t.Errorf("registry type = %q, want maven", registryType)
			}
			return errors.New("push disabled")
		},
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/proxy/com/example/demo/1.0/demo-1.0.jar", nil)
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestProxyCachePreservesProjectIDForQuotaAccounting(t *testing.T) {
	publisher := &recordingFilePublisher{}
	handler := &handler{deps: regmultiformat.Deps{FilePublisher: publisher}}

	handler.cacheProxiedFile(context.Background(), 42, "proxy", "com/example/demo/1.0/demo-1.0.jar", []byte("jar"))

	if publisher.event.ProjectID != 42 {
		t.Fatalf("ProjectID = %d, want 42", publisher.event.ProjectID)
	}
}
