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

package v2

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The registry reads the access claim of tokens core issues, so its JSON
// shape must stay byte-stable across distribution versions.
func TestClaimsAccessJSON(t *testing.T) {
	var c Claims
	require.NoError(t, json.Unmarshal([]byte(`{"iss":"harbor-token-issuer","sub":"admin","aud":"harbor-registry",
		"exp":1700000000,"access":[
		{"type":"repository","name":"library/app","actions":["pull","push"]},
		{"type":"repository","class":"image","name":"p/a","actions":["*"]},
		{"type":"registry","name":"catalog","actions":[]}]}`), &c))

	require.Len(t, c.Access, 3)
	assert.Equal(t, "repository", c.Access[0].Type)
	assert.Equal(t, "library/app", c.Access[0].Name)
	assert.Equal(t, []string{"pull", "push"}, c.Access[0].Actions)
	assert.Equal(t, "image", c.Access[1].Class)
	assert.Equal(t, []string{}, c.Access[2].Actions)
	assert.Equal(t, jwt.ClaimStrings{"harbor-registry"}, c.Audience)
	assert.Equal(t, time.Unix(1700000000, 0).UTC(), c.ExpiresAt.UTC())

	out, err := json.Marshal(c.Access)
	require.NoError(t, err)
	assert.JSONEq(t, `[
		{"type":"repository","name":"library/app","actions":["pull","push"]},
		{"type":"repository","class":"image","name":"p/a","actions":["*"]},
		{"type":"registry","name":"catalog","actions":[]}]`, string(out))

	aud, err := json.Marshal(c)
	require.NoError(t, err)
	assert.Contains(t, string(aud), `"aud":"harbor-registry"`, "single audience stays a string")
}
