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
	"strings"

	"github.com/goharbor/harbor/src/jobservice/errs"
	"github.com/goharbor/harbor/src/jobservice/logger"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/errors"
	tracelib "github.com/goharbor/harbor/src/lib/trace"
)

const (
	baseRoute  = "/api"
	apiVersion = "v1"
)

// Router defines the related routes for the job service and directs the request
// to the right handler method.
type Router interface {
	// ServeHTTP used to handle the http requests
	ServeHTTP(w http.ResponseWriter, req *http.Request)
}

// BaseRouter provides the basic routes for the job service based on the golang http server mux.
type BaseRouter struct {
	// Use the standard library mux to keep the routes mapping.
	router http.Handler

	// Handler used to handle the requests
	handler Handler

	// Do auth
	authenticator Authenticator
}

// NewBaseRouter is the constructor of BaseRouter.
func NewBaseRouter(handler Handler, authenticator Authenticator) Router {
	br := &BaseRouter{
		handler:       handler,
		authenticator: authenticator,
	}

	// Register routes here
	br.router = br.registerRoutes()

	if tracelib.Enabled() {
		br.router = tracelib.NewHandler(br.router, "serve-http")
	}
	return br
}

// ServeHTTP is the implementation of Router interface.
func (br *BaseRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// No auth required for /stats as it is a health check endpoint
	// Do auth for other services
	if req.URL.String() != fmt.Sprintf("%s/%s/stats", baseRoute, apiVersion) {
		if err := br.authenticator.DoAuth(req); err != nil {
			authErr := errs.UnauthorizedError(err)
			if authErr == nil {
				authErr = errors.Errorf("unauthorized: %s", err)
			}
			logger.Errorf("Serve http request '%s %s' failed with error: %s", lib.TrimLineBreaks(req.Method), req.URL.String(), authErr.Error())
			w.WriteHeader(http.StatusUnauthorized)
			writeDate(w, []byte(authErr.Error()))
			return
		}
	}

	// Directly pass requests to the server mux.
	br.router.ServeHTTP(w, req)
}

// registerRoutes builds the server mux carrying the job service routes.
func (br *BaseRouter) registerRoutes() http.Handler {
	prefix := fmt.Sprintf("%s/%s", baseRoute, apiVersion)
	router := http.NewServeMux()

	router.HandleFunc("POST "+prefix+"/jobs", br.handler.HandleLaunchJobReq)
	router.HandleFunc("GET "+prefix+"/jobs", br.handler.HandleGetJobsReq)
	router.HandleFunc("GET "+prefix+"/jobs/{job_id}", withJobID(br.handler.HandleGetJobReq))
	router.HandleFunc("POST "+prefix+"/jobs/{job_id}", withJobID(br.handler.HandleJobActionReq))
	router.HandleFunc("GET "+prefix+"/jobs/{job_id}/log", withJobID(br.handler.HandleJobLogReq))
	router.HandleFunc("GET "+prefix+"/stats", br.handler.HandleCheckStatusReq)
	router.HandleFunc("GET "+prefix+"/config", br.handler.HandleGetConfigReq)
	router.HandleFunc("GET "+prefix+"/jobs/{job_id}/executions", withJobID(br.handler.HandlePeriodicExecutions))

	return router
}

// withJobID turns away the job IDs the routes could not previously receive.
// ServeMux matches escaped path segments and unescapes the wildcard afterwards,
// so "%2F" and "%2e%2e" now reach a handler as a path value holding a separator
// or a parent reference; gorilla/mux matched the decoded path and answered 404.
// Keeping that answer leaves the routing contract where it was.
func withJobID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if id := req.PathValue("job_id"); strings.Contains(id, "..") || strings.ContainsRune(id, '/') {
			http.NotFound(w, req)
			return
		}

		next(w, req)
	}
}
