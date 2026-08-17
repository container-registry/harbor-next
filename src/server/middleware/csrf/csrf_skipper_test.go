package csrf

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/goharbor/harbor/src/lib"
)

func TestCSRFSKipperPackageRoutes(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		carrySession bool
		want         bool
	}{
		{
			name: "npm client route",
			path: "/npm/library/-/user/org.couchdb.user:admin",
			want: true,
		},
		{
			name:         "maven client route with session",
			path:         "/maven/library/com/acme/demo/1.0.0/demo-1.0.0.jar",
			carrySession: true,
			want:         true,
		},
		{
			name: "pypi client route",
			path: "/pypi/library/simple/requests/",
			want: true,
		},
		{
			name:         "cargo client route with session",
			path:         "/cargo/library/api/v1/crates/new",
			carrySession: true,
			want:         true,
		},
		{
			name: "ui route",
			path: "/c/login",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, tt.path, nil)
			if tt.carrySession {
				req = req.WithContext(lib.WithCarrySession(req.Context(), true))
			}
			if got := csrfSkipper(req); got != tt.want {
				t.Fatalf("csrfSkipper() = %v, want %v", got, tt.want)
			}
		})
	}
}
