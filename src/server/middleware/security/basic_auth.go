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

package security

import (
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/security/local"
	"github.com/goharbor/harbor/src/core/auth"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/log"
)

type basicAuth struct{}

// mirror audit_log_ext.client_address/user_agent column widths in db
const (
	maxClientIPLen  = 255
	maxUserAgentLen = 1024
)

func trueClientIPHeaderName() string {
	// because the true client IP header varies based on the foreground proxy/lb settings,
	// make it configurable by env
	name := os.Getenv("TRUE_CLIENT_IP_HEADER")
	if len(name) == 0 {
		name = "x-forwarded-for"
	}
	return name
}

// GetClientIP get client ip from request, taking only the first hop of a
// comma-separated header and falling back to RemoteAddr if it's not an IP
func GetClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if raw := r.Header.Get(trueClientIPHeaderName()); raw != "" {
		first := strings.TrimSpace(strings.SplitN(raw, ",", 2)[0])
		if ip := parseIP(first); ip != nil {
			return truncateRunes(ip.String(), maxClientIPLen)
		}
	}
	return truncateRunes(r.RemoteAddr, maxClientIPLen)
}

// parseIP parses s as an IP address, stripping a "host:port" (or
// "[host]:port") wrapper first if present, since some proxies/LBs
// port-qualify the client-IP header entry.
func parseIP(s string) net.IP {
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	return net.ParseIP(s)
}

// GetUserAgent get the user agent of current request
func GetUserAgent(r *http.Request) string {
	if r == nil {
		return ""
	}
	return truncateRunes(r.Header.Get("user-agent"), maxUserAgentLen)
}

// truncateRunes caps s at maxLen runes, matching Postgres varchar(n) semantics
func truncateRunes(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s // byte count is always >= rune count
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

func (b *basicAuth) Generate(req *http.Request) security.Context {
	log := log.G(req.Context())
	username, password, ok := req.BasicAuth()
	if !ok {
		return nil
	}

	// In OIDC/LDAP/UAA modes only the admin uses basic auth; other principals are
	// handled by earlier generators (robot, OIDC CLI), so skipping them spares a
	// useless auth-backend round-trip. Empty mode is treated as DB auth (as in
	// auth.Login) so a config lookup failure can't lock out basic auth.
	authMode := lib.GetAuthMode(req.Context())
	if authMode != "" && authMode != common.DBAuth && !auth.IsSuperUser(req.Context(), username) {
		log.Debugf("basic auth skipped for user %s, auth mode is %s", username, authMode)
		return nil
	}

	user, err := auth.Login(req.Context(), models.AuthModel{
		Principal: username,
		Password:  password,
	})

	if err != nil {
		// Bad credentials are expected probe traffic (scanners/bots): log at DEBUG
		// without the eager WithField structured-logger cost on this hot path.
		// Unexpected backend/config errors stay at ERROR with client context.
		if _, ok := err.(auth.ErrAuth); ok {
			log.Debugf("failed to authenticate user:%s, error:%v", username, err)
		} else {
			log.WithField("client IP", GetClientIP(req)).WithField("user agent", GetUserAgent(req)).Errorf("failed to authenticate user:%s, error:%v", username, err)
		}
		return nil
	}
	if user == nil {
		log.Debug("basic auth user is nil")
		return nil
	}
	log.Debugf("a basic auth security context generated for request %s %s", req.Method, req.URL.Path)
	return local.NewSecurityContext(user)
}
