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

package test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/goharbor/harbor/src/common"
)

// RequestHandlerMapping is a mapping between request and its handler
type RequestHandlerMapping struct {
	// Method is the method the request used
	Method string
	// Pattern is the pattern the request must match
	Pattern string
	// Handler is the handler which handles the request
	Handler func(http.ResponseWriter, *http.Request)
}

// ServeHTTP ...
func (rhm *RequestHandlerMapping) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if len(rhm.Method) != 0 && r.Method != strings.ToUpper(rhm.Method) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rhm.Handler(w, r)
}

// Response is a response used for unit test
type Response struct {
	// StatusCode is the status code of the response
	StatusCode int
	// Headers are the headers of the response
	Headers map[string]string
	// Body is the body of the response
	Body []byte
}

// Handler returns a handler function which handle request according to
// the response provided
func Handler(resp *Response) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if resp == nil {
			return
		}

		for k, v := range resp.Headers {
			w.Header().Add(http.CanonicalHeaderKey(k), v)
		}

		if resp.StatusCode == 0 {
			resp.StatusCode = http.StatusOK
		}
		w.WriteHeader(resp.StatusCode)

		if len(resp.Body) != 0 {
			io.Copy(w, bytes.NewReader(resp.Body))
		}
	}
}

// NewServer creates an HTTP server for unit test
func NewServer(mappings ...*RequestHandlerMapping) *httptest.Server {
	routes := make(prefixRouter, 0, len(mappings))
	for _, mapping := range mappings {
		routes = append(routes, &route{
			mapping: mapping,
			prefix:  compilePathPrefix(mapping.Pattern),
		})
	}

	return httptest.NewServer(routes)
}

// route pairs a mapping with the compiled form of its pattern.
type route struct {
	mapping *RequestHandlerMapping
	prefix  *regexp.Regexp
}

// prefixRouter serves the first route whose pattern prefixes the request path
// and whose method matches.
//
// A pattern is a prefix rather than a whole path, it is not segment aligned, and
// the routes are tried in registration order. Tests depend on all three: several
// register "/blobs/digest" alongside "/blobs/digest1" and expect the first to
// answer both, which a longest-match router would not do.
type prefixRouter []*route

func (routes prefixRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if cleaned := cleanPath(r.URL.Path); cleaned != r.URL.Path {
		target := *r.URL
		target.Path = cleaned
		http.Redirect(w, r, target.String(), http.StatusMovedPermanently)
		return
	}

	// A route whose method does not match is passed over rather than answered,
	// so a later route still gets its turn at the path.
	methodMismatch := false

	for _, route := range routes {
		if !route.prefix.MatchString(r.URL.Path) {
			continue
		}

		if r.Method != strings.ToUpper(route.mapping.Method) {
			methodMismatch = true
			continue
		}

		route.mapping.ServeHTTP(w, r)
		return
	}

	if methodMismatch {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	http.NotFound(w, r)
}

// pathVariable finds a "{name}" or "{name:regexp}" placeholder in a pattern.
var pathVariable = regexp.MustCompile(`\{[^{}]*\}`)

// compilePathPrefix turns a pattern into a regexp anchored at the start of the
// path. Literal text matches itself, and a placeholder stands for one path
// segment unless it carries its own regexp after a colon.
func compilePathPrefix(pattern string) *regexp.Regexp {
	var expr strings.Builder
	expr.WriteString("^")

	end := 0
	for _, location := range pathVariable.FindAllStringIndex(pattern, -1) {
		expr.WriteString(regexp.QuoteMeta(pattern[end:location[0]]))

		variable := pattern[location[0]+1 : location[1]-1]
		if _, pattern, ok := strings.Cut(variable, ":"); ok {
			expr.WriteString("(?:" + pattern + ")")
		} else {
			expr.WriteString("[^/]+")
		}

		end = location[1]
	}
	expr.WriteString(regexp.QuoteMeta(pattern[end:]))

	return regexp.MustCompile(expr.String())
}

// cleanPath normalises p the way the router did before matching, keeping the
// trailing slash that path.Clean drops.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}

	if p[0] != '/' {
		p = "/" + p
	}

	cleaned := path.Clean(p)
	if p[len(p)-1] == '/' && cleaned != "/" {
		cleaned += "/"
	}

	return cleaned
}

// GetUnitTestConfig ...
func GetUnitTestConfig() map[string]any {
	ipAddress := os.Getenv("IP")
	return map[string]any{
		common.ExtEndpoint:            fmt.Sprintf("https://%s", ipAddress),
		common.AUTHMode:               "db_auth",
		common.DatabaseType:           "postgresql",
		common.PostGreSQLHOST:         ipAddress,
		common.PostGreSQLPort:         5432,
		common.PostGreSQLUsername:     "postgres",
		common.PostGreSQLPassword:     "root123",
		common.PostGreSQLDatabase:     "registry",
		common.LDAPURL:                "ldap://ldap.vmware.com",
		common.LDAPSearchDN:           "cn=admin,dc=example,dc=com",
		common.LDAPSearchPwd:          "admin",
		common.LDAPBaseDN:             "dc=example,dc=com",
		common.LDAPUID:                "uid",
		common.LDAPFilter:             "",
		common.LDAPScope:              2,
		common.LDAPTimeout:            30,
		common.LDAPVerifyCert:         true,
		common.UAAVerifyCert:          true,
		common.AdminInitialPassword:   "Harbor12345",
		common.LDAPGroupSearchFilter:  "objectclass=groupOfNames",
		common.LDAPGroupAdminFilter:   "",
		common.LDAPGroupBaseDN:        "dc=example,dc=com",
		common.LDAPGroupAttributeName: "cn",
		common.LDAPGroupSearchScope:   2,
		common.LDAPGroupAdminDn:       "cn=harbor_users,ou=groups,dc=example,dc=com",
		common.SelfRegistration:       "true",
		common.TokenServiceURL:        "http://core:8080/service/token",
		common.RegistryURL:            fmt.Sprintf("http://%s:5000", ipAddress),
		common.ReadOnly:               false,
		common.RobotNamePrefix:        "robot$",
	}
}

// TraceCfgMap ...
func TraceCfgMap(cfgs map[string]any) {
	var keys []string
	for k := range cfgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%v=%v\n", k, cfgs[k])
	}
}

// CheckSetsEqual - check int set if they are equals
func CheckSetsEqual(setA, setB []int) bool {
	if len(setA) != len(setB) {
		return false
	}
	type void struct{}
	var exist void
	setAll := make(map[int]void)
	for _, r := range setA {
		setAll[r] = exist
	}
	for _, r := range setB {
		if _, ok := setAll[r]; !ok {
			return false
		}
	}

	setAll = make(map[int]void)
	for _, r := range setB {
		setAll[r] = exist
	}
	for _, r := range setA {
		if _, ok := setAll[r]; !ok {
			return false
		}
	}
	return true

}
