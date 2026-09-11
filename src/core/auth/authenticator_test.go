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
package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

// stubAuthenticateHelper always succeeds, so any lock rejection observed in
// a test can only come from the lock check itself, not from Authenticate.
type stubAuthenticateHelper struct {
	DefaultAuthenticateHelper
}

func (*stubAuthenticateHelper) Authenticate(_ context.Context, m models.AuthModel) (*models.User, error) {
	return &models.User{Username: m.Principal}, nil
}

// TestLoginLockedUserReturnsError guards against a regression of the bug where a
// locked principal made Login return (nil, nil): callers that don't explicitly
// nil-check the user (as opposed to checking err != nil) would treat that as a
// successful, anonymous authentication instead of a rejection.
func TestLoginLockedUserReturnsError(t *testing.T) {
	// AUTH_MODE left unset resolves to "", which Login maps to common.DBAuth
	// without consulting IsSuperUser (which needs a real user store) - see the
	// authMode == "" short-circuit in Login.
	config.InitWithSettings(map[string]any{})
	Register(common.DBAuth, &stubAuthenticateHelper{})

	principal := "locked-test-user"
	lock.Lock(principal)

	user, err := Login(context.Background(), models.AuthModel{
		Principal: principal,
		Password:  "irrelevant",
	})

	assert.Nil(t, user)
	if assert.Error(t, err) {
		assert.IsType(t, ErrAuth{}, err)
	}
}
