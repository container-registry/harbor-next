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

package v2auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	securitytesting "github.com/goharbor/harbor/src/testing/common/security"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	"github.com/goharbor/harbor/src/testing/mock"
)

// TestMiddlewareProjectLookupError asserts that a project lookup failing for a
// reason other than a missing project is answered with 503, not with the 401
// that tells the client its credentials are wrong.
func TestMiddlewareProjectLookupError(t *testing.T) {
	cases := []struct {
		name       string
		lookupErr  error
		wantStatus int
	}{
		{"database unreachable", errors.New("failed to connect to `user=harbor database=registry`: connection refused"), http.StatusServiceUnavailable},
		{"project not found", errors.NotFoundError(nil).WithMessage("project library not found"), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctl := &projecttesting.Controller{}
			mock.OnAnything(ctl, "Get").Return(nil, tc.lookupErr)
			Middleware()
			orig := checker
			checker = reqChecker{ctl: ctl}
			t.Cleanup(func() { checker = orig })

			sc := &securitytesting.Context{}
			sc.On("IsAuthenticated").Return(true)
			ctx := security.NewContext(context.Background(), sc)
			ctx = lib.WithArtifactInfo(ctx, lib.ArtifactInfo{
				Repository:  "library/hello-world",
				Reference:   "latest",
				ProjectName: "library",
			})
			req := httptest.NewRequest(http.MethodGet, "/v2/library/hello-world/manifests/latest", nil).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer token")

			called := false
			next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
			rec := httptest.NewRecorder()
			Middleware()(next).ServeHTTP(rec, req)

			assert.False(t, called)
			assert.Equal(t, tc.wantStatus, rec.Code)
			if tc.wantStatus == http.StatusServiceUnavailable {
				assert.Equal(t, "5", rec.Header().Get("Retry-After"))
				assert.Empty(t, rec.Header().Get("Www-Authenticate"))
				assert.JSONEq(t, `{"errors":[{"code":"UNAVAILABLE","message":"service unavailable"}]}`, rec.Body.String())
			}
		})
	}
}
