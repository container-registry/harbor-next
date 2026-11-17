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

package oidc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGoogleEndpoint(t *testing.T) {
	cases := []struct {
		authURL  string
		expected bool
	}{
		{"https://accounts.google.com/o/oauth2/v2/auth", true},
		{"https://ACCOUNTS.Google.COM/o/oauth2/v2/auth", true},
		{"HTTPS://accounts.google.com:443/o/oauth2/v2/auth", true},
		{"http://accounts.google.com/o/oauth2/v2/auth", false},
		{"https://accounts.google.com.attacker.example/auth", false},
		{"https://login.example.com/accounts.google.com", false},
		{"://bad", false},
	}
	for _, c := range cases {
		t.Run(c.authURL, func(t *testing.T) {
			assert.Equal(t, c.expected, isGoogleEndpoint(c.authURL))
		})
	}
}
