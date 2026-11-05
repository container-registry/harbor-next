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

package config

import (
	"os"
	"testing"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/testing/mock"
	testUser "github.com/goharbor/harbor/src/testing/pkg/user"
)

// A test feature cannot add config metadata after package init, so it
// borrows the existing boolean read_only item.
const featureTestEnvKey = "READ_ONLY"

func init() {
	feature.Register(feature.Definition{Feature: "controller_test_feature", ConfigKey: common.ReadOnly, EnvKey: featureTestEnvKey})
}

func newFeatureTestController() *controller {
	userMgr := &testUser.Manager{}
	userMgr.On("Count", mock.Anything, mock.Anything).Return(int64(1), nil)
	return &controller{userManager: userMgr}
}

func TestConvertForGetReportsEnvironmentControlledFeature(t *testing.T) {
	for _, envValue := range []bool{true, false} {
		t.Setenv(featureTestEnvKey, map[bool]string{true: "true", false: "false"}[envValue])

		result, err := newFeatureTestController().ConvertForGet(t.Context(), map[string]any{
			common.AUTHMode: common.DBAuth,
			common.ReadOnly: !envValue,
		}, false)
		if err != nil {
			t.Fatalf("ConvertForGet: %v", err)
		}
		got := result[common.ReadOnly]
		if got.Val != envValue || got.Editable {
			t.Errorf("env=%v: {value: %v, editable: %v}, want {%v, false}", envValue, got.Val, got.Editable, envValue)
		}
	}
}

func TestConvertForGetLeavesStoredFeatureEditable(t *testing.T) {
	t.Setenv(featureTestEnvKey, "")
	if err := os.Unsetenv(featureTestEnvKey); err != nil {
		t.Fatal(err)
	}

	result, err := newFeatureTestController().ConvertForGet(t.Context(), map[string]any{
		common.AUTHMode: common.DBAuth,
		common.ReadOnly: true,
	}, false)
	if err != nil {
		t.Fatalf("ConvertForGet: %v", err)
	}
	got := result[common.ReadOnly]
	if got.Val != true || !got.Editable {
		t.Errorf("{value: %v, editable: %v}, want {true, true}", got.Val, got.Editable)
	}
}

func TestValidateCfgRejectsEnvironmentControlledFeature(t *testing.T) {
	t.Setenv(featureTestEnvKey, "true")

	err := newFeatureTestController().validateCfg(t.Context(), map[string]any{common.ReadOnly: false})
	if !errors.IsErr(err, errors.BadRequestCode) {
		t.Fatalf("validateCfg error = %v, want a bad request", err)
	}
}
