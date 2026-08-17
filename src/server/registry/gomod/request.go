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

package gomod

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/mod/module"

	"github.com/goharbor/harbor/src/server/registry/gosum"
)

const routePrefix = "/go/"

var errInvalidPath = errors.New("invalid go module proxy path")

type requestType string

const (
	requestList   requestType = "list"
	requestLatest requestType = "latest"
	requestInfo   requestType = "info"
	requestMod    requestType = "mod"
	requestZip    requestType = "zip"
	requestSumDB  requestType = "sumdb"
)

type moduleRequest struct {
	Project string
	Module  string
	Version string
	SumDB   string
	Path    string
	Type    requestType
}

func parseRequest(method, escapedPath string) (*moduleRequest, error) {
	if method != http.MethodGet && method != http.MethodHead {
		return nil, errInvalidPath
	}
	raw := strings.TrimPrefix(escapedPath, routePrefix)
	if raw == escapedPath {
		return nil, errInvalidPath
	}
	raw = strings.Trim(raw, "/")
	project, rest, ok := strings.Cut(raw, "/")
	if !ok || project == "" || rest == "" {
		return nil, errInvalidPath
	}
	project, err := url.PathUnescape(project)
	if err != nil || project == "" {
		return nil, errInvalidPath
	}
	if strings.HasPrefix(rest, "sumdb/") {
		sumdbRest := strings.TrimPrefix(rest, "sumdb/")
		sumdb, sumdbPath, ok := strings.Cut(sumdbRest, "/")
		if !ok || sumdb == "" || sumdbPath == "" {
			return nil, errInvalidPath
		}
		sumdb, err = url.PathUnescape(sumdb)
		if err != nil {
			return nil, errInvalidPath
		}
		sumdbPath, err = url.PathUnescape(sumdbPath)
		if err != nil || sumdbPath == "" || sumdbPath != "supported" && !gosum.ValidPath(sumdbPath) {
			return nil, errInvalidPath
		}
		return &moduleRequest{Project: project, SumDB: sumdb, Path: sumdbPath, Type: requestSumDB}, nil
	}
	if modulePath, ok := strings.CutSuffix(rest, "/@latest"); ok && modulePath != "" {
		modulePath, err := parseModulePath(modulePath)
		if err != nil {
			return nil, err
		}
		return &moduleRequest{Project: project, Module: modulePath, Type: requestLatest}, nil
	}
	modulePath, suffix, ok := strings.Cut(rest, "/@v/")
	if !ok || modulePath == "" || suffix == "" {
		return nil, errInvalidPath
	}
	modulePath, err = parseModulePath(modulePath)
	if err != nil {
		return nil, err
	}
	switch {
	case suffix == "list":
		return &moduleRequest{Project: project, Module: modulePath, Type: requestList}, nil
	case strings.HasSuffix(suffix, ".info"):
		return versionRequest(project, modulePath, strings.TrimSuffix(suffix, ".info"), requestInfo)
	case strings.HasSuffix(suffix, ".mod"):
		return versionRequest(project, modulePath, strings.TrimSuffix(suffix, ".mod"), requestMod)
	case strings.HasSuffix(suffix, ".zip"):
		return versionRequest(project, modulePath, strings.TrimSuffix(suffix, ".zip"), requestZip)
	default:
		return nil, errInvalidPath
	}
}

func parseModulePath(modulePath string) (string, error) {
	modulePath = strings.Trim(modulePath, "/")
	modulePath, err := url.PathUnescape(modulePath)
	if err != nil || modulePath == "" {
		return "", errInvalidPath
	}
	modulePath, err = module.UnescapePath(modulePath)
	if err != nil {
		return "", errInvalidPath
	}
	return modulePath, nil
}

func versionRequest(project, modulePath, version string, typ requestType) (*moduleRequest, error) {
	version, err := url.PathUnescape(version)
	if err != nil || version == "" {
		return nil, errInvalidPath
	}
	version, err = module.UnescapeVersion(version)
	if err != nil {
		return nil, errInvalidPath
	}
	return &moduleRequest{Project: project, Module: modulePath, Version: version, Type: typ}, nil
}
