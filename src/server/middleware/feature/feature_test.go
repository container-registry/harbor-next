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

package feature

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/goharbor/harbor/src/lib/config"
	featuregate "github.com/goharbor/harbor/src/pkg/feature"

	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

const (
	testFeature   featuregate.Feature = "test_middleware_feature"
	testConfigKey                     = "enable_test_middleware_feature"
	testEnvKey                        = "HARBOR_ENABLE_TEST_MIDDLEWARE_FEATURE"
)

func init() {
	featuregate.Register(featuregate.Definition{Feature: testFeature, ConfigKey: testConfigKey, EnvKey: testEnvKey})
}

func TestRequire(t *testing.T) {
	t.Setenv(testEnvKey, "")
	if err := os.Unsetenv(testEnvKey); err != nil {
		t.Fatal(err)
	}
	handler := Require(testFeature)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, tc := range []struct {
		name    string
		enabled bool
		want    int
	}{
		{name: "disabled", want: http.StatusNotFound},
		{name: "enabled", enabled: true, want: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config.InitWithSettings(map[string]any{testConfigKey: tc.enabled})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/feature-route", nil))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestRequirePanicsOnUnregisteredFeature(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected Require to panic for an unregistered feature")
		}
	}()
	Require("not_registered")
}
