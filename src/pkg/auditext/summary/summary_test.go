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

package summary

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/pkg/auditext/model"
)

const dgst = "sha256:d12c53014cd86ee9ab8dfabf15c2a19a6d10f6ec64e8860e0f2945881ecffc52"

func TestDescribe(t *testing.T) {
	cases := []struct {
		name string
		log  model.AuditLogExt
		want string
	}{
		{
			name: "push by tag keeps the digest",
			log:  model.AuditLogExt{Username: "alice", Operation: "create", ResourceType: "artifact", Resource: "lib/nginx:1.25", OperationDescription: "push artifact: lib/nginx@" + dgst, IsSuccessful: true},
			want: "alice pushed artifact lib/nginx:1.25 (" + dgst + ")",
		},
		{
			name: "pull by digest says nothing twice",
			log:  model.AuditLogExt{Username: "anonymous", Operation: "pull", ResourceType: "artifact", Resource: "lib/nginx@" + dgst, OperationDescription: "pull artifact: lib/nginx@" + dgst, IsSuccessful: true},
			want: "anonymous pulled artifact lib/nginx@" + dgst,
		},
		{
			name: "parenthesised detail after the resource",
			log:  model.AuditLogExt{Username: "alice", Operation: "pull", ResourceType: "artifact", Resource: "lib/nginx:1.25", OperationDescription: "pull artifact: lib/nginx:1.25 (multi-arch index, 8 platforms)", IsSuccessful: true},
			want: "alice pulled artifact lib/nginx:1.25 (multi-arch index, 8 platforms)",
		},
		{
			name: "project create",
			log:  model.AuditLogExt{Username: "admin", Operation: "create", ResourceType: "project", Resource: "demo", OperationDescription: "create project: demo", IsSuccessful: true},
			want: "admin created project demo",
		},
		{
			name: "api event with name",
			log:  model.AuditLogExt{Username: "admin", Operation: "delete", ResourceType: "user", Resource: "bob", OperationDescription: "delete user with name: bob", IsSuccessful: true},
			want: "admin deleted user bob",
		},
		{
			name: "user update keeps what changed",
			log:  model.AuditLogExt{Username: "admin", Operation: "update", ResourceType: "user", Resource: "bob", OperationDescription: "update user with name: bob, change user password", IsSuccessful: true},
			want: "admin updated user bob (change user password)",
		},
		{
			name: "a name with spaces stays whole",
			log:  model.AuditLogExt{Username: "alice smith", Operation: "update", ResourceType: "user", Resource: "alice smith", OperationDescription: "update user with name: alice smith, change user password", IsSuccessful: true},
			want: "alice smith updated user alice smith (change user password)",
		},
		{
			name: "configuration lists old and new values",
			log:  model.AuditLogExt{Username: "admin", Operation: "update", ResourceType: "configuration", Resource: "configuration", OperationDescription: "update configuration: auth_mode db_auth -> ldap_auth, ldap_url ldap://a -> ldap://b", IsSuccessful: true},
			want: "admin updated configuration: auth_mode db_auth -> ldap_auth, ldap_url ldap://a -> ldap://b",
		},
		{
			name: "configuration secret without values",
			log:  model.AuditLogExt{Username: "admin", Operation: "update", ResourceType: "configuration", Resource: "configuration", OperationDescription: "update configuration: ldap_search_password changed", IsSuccessful: true},
			want: "admin updated configuration: ldap_search_password changed",
		},
		{
			name: "configuration without changes",
			log:  model.AuditLogExt{Username: "admin", Operation: "update", ResourceType: "configuration", Resource: "configuration", OperationDescription: "update configuration", IsSuccessful: true},
			want: "admin updated configuration",
		},
		{
			name: "member keeps its own description as detail",
			log:  model.AuditLogExt{Username: "admin", Operation: "create", ResourceType: "member", Resource: "alice", OperationDescription: "user in project demo", IsSuccessful: true},
			want: "admin created member alice (user in project demo)",
		},
		{
			name: "login",
			log:  model.AuditLogExt{Username: "admin", Operation: "login", ResourceType: "user", Resource: "admin", OperationDescription: "login", IsSuccessful: true},
			want: "admin logged in",
		},
		{
			name: "failed login",
			log:  model.AuditLogExt{Username: "admin", Operation: "login", ResourceType: "user", Resource: "admin", OperationDescription: "login"},
			want: "admin failed to log in",
		},
		{
			name: "failed delete",
			log:  model.AuditLogExt{Username: "dev", Operation: "delete", ResourceType: "robot", Resource: "robot$demo+ci", OperationDescription: "delete robot with name: robot$demo+ci"},
			want: "dev failed to delete robot robot$demo+ci",
		},
		{
			name: "no operator",
			log:  model.AuditLogExt{Operation: "delete", ResourceType: "tag", Resource: "lib/nginx:1.25", OperationDescription: "delete tag: lib/nginx:1.25", IsSuccessful: true},
			want: "unknown deleted tag lib/nginx:1.25",
		},
		{
			name: "operation word must end at a boundary",
			log:  model.AuditLogExt{Username: "admin", Operation: "update", ResourceType: "project", Resource: "demo", OperationDescription: "updated metadata", IsSuccessful: true},
			want: "admin updated project demo (updated metadata)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, Describe(&c.log))
		})
	}
}

func TestReplaceActor(t *testing.T) {
	assert.Equal(t, "1a2b3c4d pushed artifact alice/app@sha256:x", ReplaceActor("alice pushed artifact alice/app@sha256:x", "alice", "1a2b3c4d"))
	assert.Equal(t, "push artifact: alice/app", ReplaceActor("push artifact: alice/app", "alice", "1a2b3c4d"), "no leading actor")
}

func TestDescribeNil(t *testing.T) {
	assert.Equal(t, "", Describe(nil))
}

func TestDescribeFitsTheColumn(t *testing.T) {
	log := &model.AuditLogExt{Username: "admin", Operation: "update", ResourceType: "configuration", Resource: "configuration",
		OperationDescription: "update configuration: " + strings.Repeat("ä", 2*MaxLength), IsSuccessful: true}
	got := Describe(log)
	assert.Equal(t, MaxLength, utf8.RuneCountInString(got))
	assert.True(t, strings.HasSuffix(got, "..."))
	assert.True(t, utf8.ValidString(got))
}
