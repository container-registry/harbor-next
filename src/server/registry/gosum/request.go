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

package gosum

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/tlog"
)

const routePrefix = "/go-sumdb/"

var errInvalidPath = errors.New("invalid Go checksum database path")

type request struct {
	Project string
	Path    string
}

func parseRequest(method, escapedPath string) (*request, error) {
	if method != http.MethodGet && method != http.MethodHead {
		return nil, errInvalidPath
	}
	raw := strings.TrimPrefix(escapedPath, routePrefix)
	if raw == escapedPath {
		return nil, errInvalidPath
	}
	raw = strings.Trim(raw, "/")
	project, protocolPath, ok := strings.Cut(raw, "/")
	if !ok || project == "" || protocolPath == "" {
		return nil, errInvalidPath
	}
	project, err := url.PathUnescape(project)
	if err != nil || project == "" || strings.Contains(project, "/") {
		return nil, errInvalidPath
	}
	protocolPath, err = url.PathUnescape(protocolPath)
	if err != nil || !ValidPath(protocolPath) {
		return nil, errInvalidPath
	}
	return &request{Project: project, Path: protocolPath}, nil
}

// ValidPath reports whether path is a canonical checksum database request.
func ValidPath(protocolPath string) bool {
	if protocolPath == "latest" {
		return true
	}
	if lookup, ok := strings.CutPrefix(protocolPath, "lookup/"); ok {
		modulePath, version, ok := strings.Cut(lookup, "@")
		if !ok || modulePath == "" || version == "" || strings.Contains(version, "@") {
			return false
		}
		if _, err := module.UnescapePath(modulePath); err != nil {
			return false
		}
		_, err := module.UnescapeVersion(version)
		return err == nil
	}
	if !strings.HasPrefix(protocolPath, "tile/") {
		return false
	}
	tile, err := tlog.ParseTilePath(protocolPath)
	return err == nil && tile.Path() == protocolPath
}
