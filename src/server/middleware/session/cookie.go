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

package session

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/beego/beego/v2/server/web"
	beegosession "github.com/beego/beego/v2/server/web/session"

	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
)

const (
	// sameSiteEnv overrides the SameSite attribute of the session cookie. Valid
	// values are Strict, Lax, None and Default.
	sameSiteEnv = "SESSION_COOKIE_SAMESITE"

	// sessionConfigKey is the beego app configuration key that registerSession
	// unmarshals its manager configuration from, in preference to its defaults.
	sessionConfigKey = "sessionConfig"
)

// tlsTerminated marks a request whose TLS was terminated in front of core. It
// carries no handshake details because nothing reads them, beego only looks at
// whether Request.TLS is set.
var tlsTerminated = &tls.ConnectionState{}

// ConfigureCookie hands beego the Secure and SameSite attributes of the session
// cookie through its own configuration key.
//
// Beego derives Secure from its own listener, which is plain HTTP whenever TLS is
// terminated in front of core, and it writes no SameSite attribute at all. Its
// registerSession start hook runs after every hook core adds, so a session
// manager installed from such a hook is replaced again before the first request.
// The configuration key is read by that hook and survives.
//
// Call it once the configuration is loaded and before web.Run.
func ConfigureCookie() error {
	secure := secureCookie()
	session := web.BConfig.WebConfig.Session
	conf, err := json.Marshal(&beegosession.ManagerConfig{
		CookieName:              session.SessionName,
		EnableSetCookie:         session.SessionAutoSetCookie,
		Gclifetime:              session.SessionGCMaxLifetime,
		CookieLifeTime:          session.SessionCookieLifeTime,
		ProviderConfig:          filepath.ToSlash(session.SessionProviderConfig),
		Domain:                  session.SessionDomain,
		DisableHTTPOnly:         session.SessionDisableHTTPOnly,
		EnableSidInHTTPHeader:   session.SessionEnableSidInHTTPHeader,
		SessionNameInHTTPHeader: session.SessionNameInHTTPHeader,
		EnableSidInURLQuery:     session.SessionEnableSidInURLQuery,
		SessionIDPrefix:         session.SessionIDPrefix,
		Secure:                  secure,
		CookieSameSite:          sameSiteMode(secure),
	})
	if err != nil {
		return err
	}
	return web.AppConfig.Set(sessionConfigKey, string(conf))
}

// secureCookie reports whether the external endpoint uses TLS, the way the CSRF
// middleware decides the same question for its own cookie.
func secureCookie() bool {
	ep, err := config.ExtEndpoint()
	if err != nil {
		log.Warningf("failed to get external endpoint: %v, set session cookie secure flag to true", err)
		return true
	}
	return !strings.HasPrefix(strings.ToLower(ep), "http://")
}

func sameSiteMode(secure bool) http.SameSite {
	// Lax keeps the session on top level navigation to Harbor from another site,
	// which Strict would break for the redirect back from an OIDC provider.
	mode := http.SameSiteLaxMode
	value := os.Getenv(sameSiteEnv)
	switch strings.ToLower(value) {
	case "strict":
		mode = http.SameSiteStrictMode
	case "lax", "":
		mode = http.SameSiteLaxMode
	case "none":
		mode = http.SameSiteNoneMode
	case "default":
		mode = http.SameSiteDefaultMode
	default:
		log.Warningf("unknown %s value %q, keeping SameSite=Lax", sameSiteEnv, value)
	}

	if mode == http.SameSiteNoneMode && !secure {
		log.Warningf("%s is None but the external endpoint is HTTP, SameSite=None requires a secure cookie, keeping SameSite=Lax", sameSiteEnv)
		mode = http.SameSiteLaxMode
	}
	return mode
}
