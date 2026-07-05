//go:build db

package csrf

import (
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/utils/test"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func resetMiddleware() {
	once = sync.Once{}
}

func TestMain(m *testing.M) {
	test.InitDatabaseFromEnv()
	conf := map[string]any{}
	config.InitWithSettings(conf)
	result := m.Run()
	if result != 0 {
		os.Exit(result)
	}
}

type handler struct {
}

func (h *handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestMiddleware(t *testing.T) {
	cases := []struct {
		req         *http.Request
		statusCode  int
		returnToken bool
	}{
		{
			req:         httptest.NewRequest(http.MethodGet, "/", nil),
			statusCode:  http.StatusOK,
			returnToken: true,
		},
		{
			req:         httptest.NewRequest(http.MethodDelete, "/", nil),
			statusCode:  http.StatusForbidden,
			returnToken: true,
		},
		{
			req:         httptest.NewRequest(http.MethodGet, "/api/2.0/projects", nil), // should be skipped
			statusCode:  http.StatusOK,
			returnToken: false,
		},
		{
			req:         httptest.NewRequest(http.MethodDelete, "/v2/library/hello-world/manifests/latest", nil), // should be skipped
			statusCode:  http.StatusOK,
			returnToken: false,
		},
	}
	for _, c := range cases {
		srv := Middleware()(&handler{})
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, c.req)
		assert.Equal(t, c.statusCode, rec.Result().StatusCode)
		assert.Equal(t, c.returnToken, rec.Result().Header.Get(tokenHeader) != "")
	}
}

func TestMiddlewareRejectsPlaintextHTTPOriginByDefault(t *testing.T) {
	conf := map[string]any{
		common.ExtEndpoint: "http://localhost:4500",
	}
	config.InitWithSettings(conf)
	t.Setenv(csrfPlaintextHTTPEnv, "false")
	resetMiddleware()
	defer func() {
		config.InitWithSettings(map[string]any{
			common.ExtEndpoint: "https://host01.com",
		})
		resetMiddleware()
	}()

	statusCode := postWithOrigin(t, "http://localhost:4500")
	assert.Equal(t, http.StatusForbidden, statusCode)
}

func TestMiddlewareAllowsPlaintextHTTPOriginWhenEnabled(t *testing.T) {
	conf := map[string]any{
		common.ExtEndpoint: "http://localhost:4500",
	}
	config.InitWithSettings(conf)
	resetMiddleware()
	t.Setenv(csrfPlaintextHTTPEnv, "true")
	defer func() {
		config.InitWithSettings(map[string]any{
			common.ExtEndpoint: "https://host01.com",
		})
		resetMiddleware()
	}()

	statusCode := postWithOrigin(t, "http://localhost:4500")
	assert.Equal(t, http.StatusOK, statusCode)
}

func postWithOrigin(t *testing.T, origin string) int {
	t.Helper()

	srv := Middleware()(&handler{})
	getReq := httptest.NewRequest(http.MethodGet, origin+"/c/login", nil)
	getRec := httptest.NewRecorder()
	srv.ServeHTTP(getRec, getReq)

	token := getRec.Result().Header.Get(tokenHeader)
	assert.NotEmpty(t, token)

	postReq := httptest.NewRequest(http.MethodPost, origin+"/c/login", nil)
	postReq.Header.Set("Origin", origin)
	postReq.Header.Set(tokenHeader, token)
	for _, cookie := range getRec.Result().Cookies() {
		postReq.AddCookie(cookie)
	}

	postRec := httptest.NewRecorder()
	srv.ServeHTTP(postRec, postReq)
	return postRec.Result().StatusCode
}

func TestMiddlewareInvalidKey(t *testing.T) {
	originalEnv := os.Getenv(csrfKeyEnv)
	defer os.Setenv(csrfKeyEnv, originalEnv)

	t.Run("invalid CSRF key", func(t *testing.T) {
		os.Setenv(csrfKeyEnv, "invalidkey")
		resetMiddleware()
		middleware := Middleware()
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("handler should not be reached when CSRF key is invalid")
		})

		handler := middleware(testHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestSecureCookie(t *testing.T) {
	assert.True(t, secureCookie())
	conf := map[string]any{
		common.ExtEndpoint: "http://harbor.test",
	}
	config.InitWithSettings(conf)

	assert.False(t, secureCookie())
	conf = map[string]any{}
	config.InitWithSettings(conf)
}
