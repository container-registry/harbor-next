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

package desc

import (
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestDescriptionTemplate(t *testing.T) {
	d := New("admin", "update", "project").Name("myproj").
		Changed("description", "hello world", "hello everyone").
		Changed("public", false, true).
		Changed("severity", "low", "low").
		Secret("ldap_search_password").
		Added("auto_scan", "true").
		Removed("retention_id", "7")
	assert.Equal(t, `admin updated project "myproj": description "hello world" -> "hello everyone", public false -> true, `+
		`ldap_search_password changed, auto_scan set "true", retention_id removed (was "7")`, d.String())
}

func TestDescriptionFactsDropEmptyKeepZero(t *testing.T) {
	d := New("admin", "delete", "project").Name("").
		Set("id", 5).Set("public", false).Set("repositories", 0).
		Set("owner", "").Set("tags", []string{}).Set("meta", map[string]string{}).Set("nothing", nil).
		Set("labels", []string{"prod", "eu"})
	assert.Equal(t, `admin deleted project: id 5, public false, repositories 0, labels ["prod","eu"]`, d.String())
}

func TestDescriptionProjectAndLogin(t *testing.T) {
	d := New("admin", "update", "member").Name("alice").Set("project", "myproj").Changed("role", "developer", "projectAdmin")
	assert.Equal(t, `admin updated member "alice" in project "myproj": role "developer" -> "projectAdmin"`, d.String())
	assert.Equal(t, `alice logged in: auth_mode "ldap_auth"`, New("alice", "login", "user").Name("alice").Set("auth_mode", "ldap_auth").String())
	assert.Equal(t, `alice logged out`, New("alice", "logout", "user").Name("alice").String())
	assert.Equal(t, `admin updated configuration: auth_mode "db_auth" -> "ldap_auth"`, New("admin", "update", "configuration").Changed("auth_mode", "db_auth", "ldap_auth").String())
}

func TestDescriptionErrorLast(t *testing.T) {
	d := New("admin", "update", "user").Name("bob").Changed("sysadmin", false, true)
	d.Error(403, `{"errors":[{"code":"FORBIDDEN","message":"forbidden"}]}`)
	d.Set("late", "x")
	assert.Equal(t, `admin updated user "bob": sysadmin false -> true, late "x" (failed: 403 forbidden)`, d.String())
	assert.Equal(t, `alice logged in: auth_mode "db_auth" (failed: 401 unauthorized)`, New("alice", "login", "user").Set("auth_mode", "db_auth").Error(401, "").String())
	// error is reserved: Set cannot claim it
	assert.False(t, New("a", "b", "c").Set(KeyError, "x").Has(KeyError))
}

func TestChangedCrossTypeEquality(t *testing.T) {
	d := New("admin", "update", "configuration").
		Changed("token_expiration", 30, float64(30)).
		Changed("self_registration", true, "true").
		Changed("ldap_url", "", "ldaps://ldap.corp")
	assert.Equal(t, `admin updated configuration: ldap_url "" -> "ldaps://ldap.corp"`, d.String())
}

func TestQuotingAndEscaping(t *testing.T) {
	d := New("admin", "update", "configuration").Changed("oidc_endpoint", "https://a/?x=1&y=2", `https://b/<c> "q"`)
	assert.Equal(t, `admin updated configuration: oidc_endpoint "https://a/?x=1&y=2" -> "https://b/<c> \"q\""`, d.String())
	assert.Equal(t, `admin created robot "robot$ci": permissions {"project/myproj":["pull","push"]}`,
		New("admin", "create", "robot").Name("robot$ci").Set("permissions", map[string][]string{"project/myproj": {"pull", "push"}}).String())
}

func TestReason(t *testing.T) {
	assert.Equal(t, "404 project myproj not found", Reason(404, `{"errors":[{"code":"NOT_FOUND","message":"project myproj not found"}]}`))
	assert.Equal(t, "401 unauthorized", Reason(401, "Invalid credentials"))
	assert.Equal(t, "409 conflict", Reason(409, ""))
	assert.Equal(t, "599", Reason(599, ""))
}

func TestDescriptionEdgeCases(t *testing.T) {
	t.Run("no details means no colon", func(t *testing.T) {
		assert.Equal(t, `admin updated project "p"`, New("admin", "update", "project").Name("p").String())
		assert.Equal(t, `admin updated project "p"`, New("admin", "update", "project").Name("p").Changed("public", true, true).String())
	})
	t.Run("nil old value reads as set, nil new value as removed", func(t *testing.T) {
		assert.Equal(t, `admin updated project "p": a set "x", b removed (was "y")`,
			New("admin", "update", "project").Name("p").Changed("a", nil, "x").Changed("b", "y", nil).String())
	})
	t.Run("secrets never carry a value, even on failure", func(t *testing.T) {
		d := New("admin", "update", "configuration").Secret("ldap_search_password").Changed("oidc_client_secret", "old", "new")
		d.Secret("oidc_client_secret").Error(http.StatusBadRequest, "")
		got := d.String()
		assert.Equal(t, `admin updated configuration: ldap_search_password changed, oidc_client_secret changed (failed: 400 bad request)`, got)
		assert.NotContains(t, got, "old")
		assert.NotContains(t, got, "new")
	})
	t.Run("unknown action keeps its name, empty actor still renders", func(t *testing.T) {
		assert.Equal(t, ` scan artifact "a"`, New("", "scan", "artifact").Name("a").String())
	})
	t.Run("the error key cannot be set as a fact", func(t *testing.T) {
		assert.Equal(t, `admin deleted user "u"`, New("admin", "delete", "user").Name("u").Set(KeyError, "boom").String())
	})
}

func TestDescriptionFitsColumn(t *testing.T) {
	long := strings.Repeat("v", 2000)
	got := New("admin", "update", "project").Name("p").Changed("description", "short", long).String()
	assert.Len(t, []rune(got), MaxLen)
	assert.True(t, strings.HasSuffix(got, "..."))
	assert.True(t, strings.HasPrefix(got, `admin updated project "p": description "short" -> "vvv`))

	// multi-byte runes are never split
	got = New("admin", "update", "project").Name("p").Set("description", strings.Repeat("é", 1500)).String()
	assert.Len(t, []rune(got), MaxLen)
	assert.True(t, utf8.ValidString(got))

	// exactly MaxLen stays untouched
	exact := strings.Repeat("x", MaxLen)
	assert.Equal(t, exact, truncate(exact, MaxLen))
	assert.Equal(t, "ab", truncate("ab", MaxLen))
}
