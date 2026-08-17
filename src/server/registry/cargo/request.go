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

package cargo

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

const routePrefix = "/cargo/"

var (
	errInvalidPath       = errors.New("invalid cargo registry path")
	errUnsupportedMethod = errors.New("unsupported cargo registry method")
)

type requestType string

const (
	requestConfig   requestType = "config"
	requestIndex    requestType = "index"
	requestPublish  requestType = "publish"
	requestDownload requestType = "download"
	requestYank     requestType = "yank"
	requestUnyank   requestType = "unyank"
)

type packageRequest struct {
	Project string
	Crate   string
	Version string
	Type    requestType
}

func parseRequest(method, escapedPath string) (*packageRequest, error) {
	if method != http.MethodGet && method != http.MethodPut && method != http.MethodDelete {
		return nil, errUnsupportedMethod
	}
	path := strings.Trim(strings.TrimPrefix(escapedPath, routePrefix), "/")
	if path == escapedPath {
		return nil, errInvalidPath
	}
	project, rest, ok := strings.Cut(path, "/")
	if !ok || project == "" || rest == "" {
		return nil, errInvalidPath
	}
	project, err := url.PathUnescape(project)
	if err != nil || project == "" {
		return nil, errInvalidPath
	}
	switch {
	case method == http.MethodGet && rest == "config.json":
		return &packageRequest{Project: project, Type: requestConfig}, nil
	case method == http.MethodPut && rest == "api/v1/crates/new":
		return &packageRequest{Project: project, Type: requestPublish}, nil
	case method == http.MethodDelete && strings.HasPrefix(rest, "api/v1/crates/"):
		return parseYank(project, strings.TrimPrefix(rest, "api/v1/crates/"), "yank", requestYank)
	case method == http.MethodPut && strings.HasPrefix(rest, "api/v1/crates/"):
		return parseYank(project, strings.TrimPrefix(rest, "api/v1/crates/"), "unyank", requestUnyank)
	case method == http.MethodGet && strings.HasPrefix(rest, "api/v1/crates/"):
		return parseDownload(project, strings.TrimPrefix(rest, "api/v1/crates/"))
	case method == http.MethodGet:
		crate, ok := crateFromIndexPath(rest)
		if !ok {
			return nil, errInvalidPath
		}
		return &packageRequest{Project: project, Crate: crate, Type: requestIndex}, nil
	default:
		return nil, errUnsupportedMethod
	}
}

func parseYank(project, path, action string, requestType requestType) (*packageRequest, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != action {
		return nil, errInvalidPath
	}
	crate, err := url.PathUnescape(parts[0])
	if err != nil || crate == "" {
		return nil, errInvalidPath
	}
	version, err := url.PathUnescape(parts[1])
	if err != nil || version == "" {
		return nil, errInvalidPath
	}
	return &packageRequest{Project: project, Crate: normalizeCrate(crate), Version: version, Type: requestType}, nil
}

func parseDownload(project, path string) (*packageRequest, error) {
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] != "download" {
		return nil, errInvalidPath
	}
	crate, err := url.PathUnescape(parts[0])
	if err != nil {
		return nil, errInvalidPath
	}
	version, err := url.PathUnescape(parts[1])
	if err != nil {
		return nil, errInvalidPath
	}
	return &packageRequest{Project: project, Crate: normalizeCrate(crate), Version: version, Type: requestDownload}, nil
}

func crateFromIndexPath(path string) (string, bool) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "", false
	}
	name, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil || name == "" || strings.Contains(name, "/") {
		return "", false
	}
	if indexPath(name) != path {
		return "", false
	}
	return normalizeCrate(name), true
}

func normalizeCrate(name string) string {
	return strings.ToLower(name)
}

func indexPath(name string) string {
	lower := strings.ToLower(name)
	switch len(lower) {
	case 1:
		return "1/" + lower
	case 2:
		return "2/" + lower
	case 3:
		return "3/" + lower[:1] + "/" + lower
	default:
		return lower[:2] + "/" + lower[2:4] + "/" + lower
	}
}
