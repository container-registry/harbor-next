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

//go:build !db

package authproxy

import (
	"context"
	"testing"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"
	k8s_api_v1beta1 "k8s.io/api/authentication/v1beta1"

	"github.com/goharbor/harbor/src/lib/orm"
	ormtesting "github.com/goharbor/harbor/src/testing/lib/orm"
)

const deadlockTimeout = 10 * time.Second

// POST /c/login in http_auth mode reaches UserFromReviewStatus from the login
// handler, inside the request transaction. Onboarding the user's groups on an
// ORM of its own needs a second pool connection and wedges core under
// concurrent logins (#850).
func TestUserFromReviewStatusRunsOnTheRequestConnection(t *testing.T) {
	ormtesting.RegisterLimitedPool(t, 1)

	status := k8s_api_v1beta1.TokenReviewStatus{
		Authenticated: true,
		User: k8s_api_v1beta1.UserInfo{
			Username: "jack",
			Groups:   []string{"group-b", "group-a"},
		},
	}

	ctx := orm.NewContext(context.Background(), beegoorm.NewOrm())
	completed := ormtesting.RunsWithin(deadlockTimeout, func() {
		_ = orm.WithTransaction(func(txCtx context.Context) error {
			_, err := UserFromReviewStatus(txCtx, status, nil, nil)
			return err
		})(ctx)
	})

	if !completed {
		t.Fatalf("UserFromReviewStatus did not return within %s: it asked the pool for a "+
			"second connection while the request transaction still held the first", deadlockTimeout)
	}
}
