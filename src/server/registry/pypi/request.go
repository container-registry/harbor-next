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

package pypi

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const routePrefix = "/pypi/"

var (
	errInvalidPath       = errors.New("invalid pypi registry path")
	errUnsupportedMethod = errors.New("unsupported pypi registry method")
	normalizePattern     = regexp.MustCompile(`[-_.]+`)
	validNamePattern     = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)
)

type requestType string

const (
	requestUpload       requestType = "upload"
	requestSimpleRoot   requestType = "simple-root"
	requestSimple       requestType = "simple"
	requestDistribution requestType = "distribution"
	requestMetadata     requestType = "metadata"
)

type packageRequest struct {
	Project  string
	Package  string
	Version  string
	Filename string
	Type     requestType
}

func parseRequest(method, escapedPath string) (*packageRequest, error) {
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
		return nil, errUnsupportedMethod
	}
	path := strings.TrimPrefix(escapedPath, routePrefix)
	if path == escapedPath {
		return nil, errInvalidPath
	}
	path = strings.Trim(path, "/")
	project, rest, ok := strings.Cut(path, "/")
	if !ok || project == "" {
		if method == http.MethodPost && path != "" {
			project = path
			rest = ""
		} else {
			return nil, errInvalidPath
		}
	}
	project, err := url.PathUnescape(project)
	if err != nil || project == "" {
		return nil, errInvalidPath
	}
	if method == http.MethodPost {
		if rest != "" {
			return nil, errInvalidPath
		}
		return &packageRequest{Project: project, Type: requestUpload}, nil
	}
	if strings.EqualFold(rest, "simple") || strings.EqualFold(rest, "simple/") {
		return &packageRequest{Project: project, Type: requestSimpleRoot}, nil
	}
	if strings.HasPrefix(rest, "simple/") {
		name := strings.Trim(strings.TrimPrefix(rest, "simple/"), "/")
		if name == "" || strings.Contains(name, "/") {
			return nil, errInvalidPath
		}
		name, err = url.PathUnescape(name)
		if err != nil || !validNamePattern.MatchString(name) {
			return nil, errInvalidPath
		}
		return &packageRequest{Project: project, Package: normalizeName(name), Type: requestSimple}, nil
	}
	if strings.HasPrefix(rest, "packages/") {
		parts := strings.Split(strings.TrimPrefix(rest, "packages/"), "/")
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return nil, errInvalidPath
		}
		name, err := url.PathUnescape(parts[0])
		if err != nil {
			return nil, errInvalidPath
		}
		version, err := url.PathUnescape(parts[1])
		if err != nil {
			return nil, errInvalidPath
		}
		filename, err := url.PathUnescape(parts[2])
		if err != nil || strings.Contains(filename, "/") {
			return nil, errInvalidPath
		}
		reqType := requestDistribution
		if strings.HasSuffix(filename, ".metadata") {
			filename = strings.TrimSuffix(filename, ".metadata")
			if filename == "" {
				return nil, errInvalidPath
			}
			reqType = requestMetadata
		}
		return &packageRequest{Project: project, Package: normalizeName(name), Version: version, Filename: filename, Type: reqType}, nil
	}
	return nil, errInvalidPath
}

func normalizeName(name string) string {
	return normalizePattern.ReplaceAllString(strings.ToLower(name), "-")
}
