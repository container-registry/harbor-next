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

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/jobservice/errs"
)

// routeProbe records which handler a request reached and the job ID it carried.
type routeProbe struct {
	route  string
	jobID  string
	called bool
}

func (p *routeProbe) record(route string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		p.route, p.jobID, p.called = route, req.PathValue("job_id"), true
		w.WriteHeader(http.StatusOK)
	}
}

func (p *routeProbe) HandleLaunchJobReq(w http.ResponseWriter, r *http.Request) {
	p.record("launch")(w, r)
}
func (p *routeProbe) HandleGetJobReq(w http.ResponseWriter, r *http.Request) {
	p.record("get-job")(w, r)
}
func (p *routeProbe) HandleJobActionReq(w http.ResponseWriter, r *http.Request) {
	p.record("job-action")(w, r)
}
func (p *routeProbe) HandleCheckStatusReq(w http.ResponseWriter, r *http.Request) {
	p.record("stats")(w, r)
}
func (p *routeProbe) HandleJobLogReq(w http.ResponseWriter, r *http.Request) {
	p.record("job-log")(w, r)
}
func (p *routeProbe) HandlePeriodicExecutions(w http.ResponseWriter, r *http.Request) {
	p.record("executions")(w, r)
}
func (p *routeProbe) HandleGetJobsReq(w http.ResponseWriter, r *http.Request) {
	p.record("get-jobs")(w, r)
}
func (p *routeProbe) HandleGetConfigReq(w http.ResponseWriter, r *http.Request) {
	p.record("config")(w, r)
}

// openAuthenticator lets every request through so the tests see routing alone.
type openAuthenticator struct{}

func (openAuthenticator) DoAuth(_ *http.Request) error { return nil }

func serve(t *testing.T, method, target string) (*routeProbe, *httptest.ResponseRecorder) {
	t.Helper()

	probe := &routeProbe{}
	router := NewBaseRouter(probe, openAuthenticator{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, target, nil))

	return probe, rec
}

func TestRouterDispatch(t *testing.T) {
	base := fmt.Sprintf("%s/%s", baseRoute, apiVersion)

	cases := []struct {
		method string
		path   string
		route  string
		jobID  string
	}{
		{http.MethodPost, "/jobs", "launch", ""},
		{http.MethodGet, "/jobs", "get-jobs", ""},
		{http.MethodGet, "/jobs?page_number=2", "get-jobs", ""},
		{http.MethodGet, "/jobs/abc123", "get-job", "abc123"},
		{http.MethodPost, "/jobs/abc123", "job-action", "abc123"},
		{http.MethodGet, "/jobs/abc123/log", "job-log", "abc123"},
		{http.MethodGet, "/jobs/abc123/executions", "executions", "abc123"},
		{http.MethodGet, "/stats", "stats", ""},
		{http.MethodGet, "/config", "config", ""},
	}

	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			probe, rec := serve(t, c.method, base+c.path)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, probe.called, "no handler was reached")
			assert.Equal(t, c.route, probe.route)
			assert.Equal(t, c.jobID, probe.jobID)
		})
	}
}

// TestRouterRejectsJobIDsWithSeparators pins the routing contract gorilla/mux
// enforced by matching the decoded path. ServeMux matches escaped segments, so
// without withJobID these would reach a handler carrying "a/b" or "..".
func TestRouterRejectsJobIDsWithSeparators(t *testing.T) {
	base := fmt.Sprintf("%s/%s", baseRoute, apiVersion)

	paths := []string{
		"/jobs/a%2Fb",
		"/jobs/a%2Fb/log",
		"/jobs/a%2Fb/executions",
		"/jobs/%2e%2e",
		"/jobs/%2e%2e/log",
		"/jobs/%2e%2e/executions",
		"/jobs/%2e%2e%2f%2e%2e/log",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			probe, rec := serve(t, http.MethodGet, base+p)

			assert.Equal(t, http.StatusNotFound, rec.Code)
			assert.False(t, probe.called, "handler was reached with job_id %q", probe.jobID)
		})
	}
}

func TestRouterRejectsUnknownRoutes(t *testing.T) {
	base := fmt.Sprintf("%s/%s", baseRoute, apiVersion)

	probe, rec := serve(t, http.MethodGet, base+"/nope")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.False(t, probe.called)
}

// TestRouterRejectsWrongMethod records a deliberate change: gorilla/mux answered
// 404 when no method matcher fired, ServeMux answers 405 and names the methods.
func TestRouterRejectsWrongMethod(t *testing.T) {
	base := fmt.Sprintf("%s/%s", baseRoute, apiVersion)

	probe, rec := serve(t, http.MethodDelete, base+"/jobs")

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD, POST", rec.Header().Get("Allow"))
	assert.False(t, probe.called)
}

// TestRouterServesHeadOnGetRoutes records the other deliberate change: ServeMux
// ties HEAD to GET across every GET route, where gorilla/mux answered 405. This
// is not confined to the unauthenticated /stats endpoint — the authenticated GET
// routes (and the withJobID-wrapped log route) see the same 405-to-200 shift that
// callers such as core and harbor-cli will observe, so pin them all.
func TestRouterServesHeadOnGetRoutes(t *testing.T) {
	base := fmt.Sprintf("%s/%s", baseRoute, apiVersion)

	cases := []struct {
		path  string
		route string
		jobID string
	}{
		{"/stats", "stats", ""},
		{"/jobs", "get-jobs", ""},
		{"/config", "config", ""},
		{"/jobs/abc123", "get-job", "abc123"},
		{"/jobs/abc123/log", "job-log", "abc123"},
		{"/jobs/abc123/executions", "executions", "abc123"},
	}

	for _, c := range cases {
		t.Run("HEAD "+c.path, func(t *testing.T) {
			probe, rec := serve(t, http.MethodHead, base+c.path)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.True(t, probe.called, "no handler was reached")
			assert.Equal(t, c.route, probe.route)
			assert.Equal(t, c.jobID, probe.jobID)
		})
	}
}

// TestErrorResponseIsNotSniffable covers the reflected path value reaching the
// response body: the type is named and sniffing is turned off, so an error text
// carrying a job ID cannot be interpreted as HTML.
func TestErrorResponseIsNotSniffable(t *testing.T) {
	fc := &fakeController{}
	fc.On("GetJob", "<img src=x onerror=alert(1)>").
		Return(nil, errs.NoObjectFoundError("<img src=x onerror=alert(1)>"))

	handler := NewDefaultHandler(fc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/x", nil)
	req.SetPathValue("job_id", "<img src=x onerror=alert(1)>")

	handler.HandleGetJobReq(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}

// TestJobLogResponseIsNotSniffable covers a successful log whose bytes happen to
// look like HTML: the type is named text/plain and sniffing is turned off, so the
// log body cannot be rendered as a page when opened in a browser.
func TestJobLogResponseIsNotSniffable(t *testing.T) {
	fc := &fakeController{}
	fc.On("GetJobLogData", "abc123").Return([]byte("<script>alert(1)</script>"), nil)

	handler := NewDefaultHandler(fc)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/abc123/log", nil)
	req.SetPathValue("job_id", "abc123")

	handler.HandleJobLogReq(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}

func TestRouterRequiresAuthExceptStats(t *testing.T) {
	base := fmt.Sprintf("%s/%s", baseRoute, apiVersion)

	probe := &routeProbe{}
	router := NewBaseRouter(probe, &SecretAuthenticator{})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/jobs", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.False(t, probe.called)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/stats", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, probe.called)
}
