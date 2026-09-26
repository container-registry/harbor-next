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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/beego/beego/v2/server/web"
	beegosession "github.com/beego/beego/v2/server/web/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestSession(t *testing.T) {
	config.InitWithSettings(map[string]any{})
	carrySession := false
	skipRenewal := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		carrySession = lib.GetCarrySession(r.Context())
		skipRenewal = lib.GetSkipSessionRenewal(r.Context())
	})
	// no session
	req, err := http.NewRequest("POST", "http://127.0.0.1:8080/api/users", nil)
	require.Nil(t, err)
	Middleware()(handler).ServeHTTP(nil, req)
	assert.False(t, carrySession)
	assert.False(t, skipRenewal)

	// contains session
	web.BConfig.WebConfig.Session.SessionName = config.SessionCookieName
	conf := &beegosession.ManagerConfig{
		CookieName:      web.BConfig.WebConfig.Session.SessionName,
		Gclifetime:      web.BConfig.WebConfig.Session.SessionGCMaxLifetime,
		ProviderConfig:  filepath.ToSlash(web.BConfig.WebConfig.Session.SessionProviderConfig),
		Secure:          web.BConfig.Listen.EnableHTTPS,
		EnableSetCookie: web.BConfig.WebConfig.Session.SessionAutoSetCookie,
		Domain:          web.BConfig.WebConfig.Session.SessionDomain,
		CookieLifeTime:  web.BConfig.WebConfig.Session.SessionCookieLifeTime,
	}
	web.GlobalSessions, err = beegosession.NewManager("memory", conf)
	require.Nil(t, err)
	_, err = web.GlobalSessions.SessionStart(httptest.NewRecorder(), req)
	require.Nil(t, err)
	Middleware()(handler).ServeHTTP(nil, req)
	assert.True(t, carrySession)
	assert.False(t, skipRenewal)

	// With no-session-renewal header.
	req.Header.Set(HeaderNoSessionRenewal, "true")
	Middleware()(handler).ServeHTTP(nil, req)
	assert.True(t, carrySession)
	assert.True(t, skipRenewal)

	// Without no-session-renewal header.
	req.Header.Del(HeaderNoSessionRenewal)
	Middleware()(handler).ServeHTTP(nil, req)
	assert.False(t, skipRenewal)
}

func TestConfigureCookie(t *testing.T) {
	cases := []struct {
		name         string
		extEndpoint  string
		sameSiteEnv  string
		wantSecure   bool
		wantSameSite http.SameSite
	}{
		{
			name:         "TLS terminated in front of core",
			extEndpoint:  "https://harbor.test",
			wantSecure:   true,
			wantSameSite: http.SameSiteLaxMode,
		},
		{
			name:         "plain HTTP endpoint",
			extEndpoint:  "http://harbor.test",
			wantSecure:   false,
			wantSameSite: http.SameSiteLaxMode,
		},
		{
			name:         "SameSite from the environment",
			extEndpoint:  "https://harbor.test",
			sameSiteEnv:  "Strict",
			wantSecure:   true,
			wantSameSite: http.SameSiteStrictMode,
		},
		{
			name:         "an unknown SameSite value keeps Lax",
			extEndpoint:  "https://harbor.test",
			sameSiteEnv:  "sometimes",
			wantSecure:   true,
			wantSameSite: http.SameSiteLaxMode,
		},
		{
			name:         "SameSite None needs a secure cookie",
			extEndpoint:  "http://harbor.test",
			sameSiteEnv:  "None",
			wantSecure:   false,
			wantSameSite: http.SameSiteLaxMode,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config.InitWithSettings(map[string]any{common.ExtEndpoint: c.extEndpoint})
			// set in every case, so an exported value cannot decide the outcome
			t.Setenv(sameSiteEnv, c.sameSiteEnv)
			web.BConfig.WebConfig.Session.SessionName = config.SessionCookieName

			require.Nil(t, ConfigureCookie())

			raw, err := web.AppConfig.String(sessionConfigKey)
			require.Nil(t, err)
			conf := &beegosession.ManagerConfig{}
			require.Nil(t, json.Unmarshal([]byte(raw), conf))

			assert.Equal(t, config.SessionCookieName, conf.CookieName)
			assert.Equal(t, c.wantSecure, conf.Secure)
			assert.Equal(t, c.wantSameSite, conf.CookieSameSite)
		})
	}
}

// Beego marks a session cookie secure only when the request carries TLS, so the
// middleware reports the protocol of the external endpoint.
func TestSessionTLSMarker(t *testing.T) {
	cases := []struct {
		name        string
		extEndpoint string
		wantTLS     bool
	}{
		{name: "TLS terminated in front of core", extEndpoint: "https://harbor.test", wantTLS: true},
		{name: "plain HTTP endpoint", extEndpoint: "http://harbor.test", wantTLS: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config.InitWithSettings(map[string]any{common.ExtEndpoint: c.extEndpoint})

			var gotTLS bool
			handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				gotTLS = r.TLS != nil
			})

			req, err := http.NewRequest("GET", "http://127.0.0.1:8080/c/login", nil)
			require.Nil(t, err)
			Middleware()(handler).ServeHTTP(httptest.NewRecorder(), req)

			assert.Equal(t, c.wantTLS, gotTLS)
			assert.Nil(t, req.TLS, "the request the server handed us is left alone")
		})
	}
}

// Takes the configuration beego is given, builds the session manager from it the
// way beego's registerSession hook does, and reads the cookie off the response.
func TestSessionCookieOnResponse(t *testing.T) {
	cases := []struct {
		name        string
		extEndpoint string
		wantSecure  bool
	}{
		{name: "TLS terminated in front of core", extEndpoint: "https://harbor.test", wantSecure: true},
		{name: "plain HTTP endpoint", extEndpoint: "http://harbor.test", wantSecure: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config.InitWithSettings(map[string]any{common.ExtEndpoint: c.extEndpoint})
			t.Setenv(sameSiteEnv, "")
			web.BConfig.WebConfig.Session.SessionName = config.SessionCookieName
			require.Nil(t, ConfigureCookie())

			raw, err := web.AppConfig.String(sessionConfigKey)
			require.Nil(t, err)
			conf := &beegosession.ManagerConfig{}
			require.Nil(t, json.Unmarshal([]byte(raw), conf))
			manager, err := beegosession.NewManager("memory", conf)
			require.Nil(t, err)

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, err := manager.SessionStart(w, r)
				require.Nil(t, err)
			})

			// a server side request, whose URL carries no scheme, which is what
			// makes beego fall through to Request.TLS
			req := httptest.NewRequest("GET", "/c/login", nil)
			rec := httptest.NewRecorder()
			Middleware()(handler).ServeHTTP(rec, req)

			cookies := rec.Result().Cookies()
			require.Len(t, cookies, 1)
			assert.Equal(t, config.SessionCookieName, cookies[0].Name)
			assert.True(t, cookies[0].HttpOnly)
			assert.Equal(t, c.wantSecure, cookies[0].Secure)
			assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
		})
	}
}
