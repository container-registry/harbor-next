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

package middlewares

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func Test_readonlySkipper(t *testing.T) {
	type args struct {
		r *http.Request
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"login", args{httptest.NewRequest(http.MethodPost, "/c/login", nil)}, true},
		{"login get", args{httptest.NewRequest(http.MethodGet, "/c/login", nil)}, false},
		{"onboard", args{httptest.NewRequest(http.MethodPost, "/c/oidc/onboard", nil)}, true},
		{"user exist", args{httptest.NewRequest(http.MethodPost, "/c/userExists", nil)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pass bool
			for _, skipper := range readonlySkippers {
				if got := skipper(tt.args.r); got == tt.want {
					pass = true
				}
			}
			if !pass {
				t.Errorf("readonlySkippers() = %v, want %v", tt.args, tt.want)
			}
		})
	}
}

// TestTransactionRunsAfterTheDatabaseReadingMiddlewares covers harbor-next #92.
// transaction.Middleware holds a pool connection for the whole request. Any
// middleware inside it that reads config or user records needs a second
// connection while holding the first, and the config-cache builder it queues
// behind needs one too, so the pool deadlocks against itself under mixed read
// and write traffic.
func TestTransactionRunsAfterTheDatabaseReadingMiddlewares(t *testing.T) {
	chain := middlewareChain()
	position := map[string]int{}
	for i, entry := range chain {
		if _, dup := position[entry.name]; dup {
			t.Fatalf("middleware %q appears twice in the chain", entry.name)
		}
		position[entry.name] = i
	}

	if last := chain[len(chain)-1].name; last != "transaction" {
		t.Errorf("%q runs inside the request transaction; transaction must be the innermost middleware so nothing before the handler holds a pool connection while it waits for another", last)
	}

	for _, name := range []string{"security", "log", "security-unauthorized"} {
		if _, ok := position[name]; !ok {
			t.Fatalf("no %q middleware in the chain", name)
		}
	}
	if position["log"] < position["security"] || position["log"] > position["security-unauthorized"] {
		t.Error("log must run between security and security-unauthorized so the request's user is logged and 401s produce an audit event")
	}
}
