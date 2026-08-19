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

package server

import (
	"net/http"

	multiformatctl "github.com/goharbor/harbor/src/controller/multiformat"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/pkg/commercial"
	commercialmiddleware "github.com/goharbor/harbor/src/server/middleware/commercial"
	"github.com/goharbor/harbor/src/server/middleware/multiformatauth"
	"github.com/goharbor/harbor/src/server/registry/maven"
	"github.com/goharbor/harbor/src/server/registry/npm"
	"github.com/goharbor/harbor/src/server/router"
)

// registerMultiFormatRoutes mounts the native npm and maven (multiformat) protocol
// adapters as siblings to the OCI registry routes. Each prefix is a beego
// catch-all gated by the multiformatauth middleware (project resolve + RBAC); the
// adapters themselves peel the leading <project> segment and reconstruct the
// native package name from the remaining path.
func registerMultiFormatRoutes() {
	baseURL, err := config.ExtEndpoint()
	if err != nil {
		// A misconfigured external endpoint only breaks the absolute tarball/file
		// URLs the adapters render; routing still works. Log and continue.
		log.Warningf("multiformat: failed to resolve external endpoint, rendered URLs may be wrong: %v", err)
	}

	ctl := multiformatctl.Ctl

	npmMux := http.NewServeMux()
	npm.Register(npmMux, ctl, baseURL)
	router.NewRoute().
		Path("/npm/*").
		Middleware(commercialmiddleware.Require(commercial.MultiFormatArtifacts)).
		Middleware(multiformatauth.Middleware()).
		Handler(npmMux)

	mavenMux := http.NewServeMux()
	maven.Register(mavenMux, ctl, baseURL)
	router.NewRoute().
		Path("/maven/*").
		Middleware(commercialmiddleware.Require(commercial.MultiFormatArtifacts)).
		Middleware(multiformatauth.Middleware()).
		Handler(mavenMux)
}
