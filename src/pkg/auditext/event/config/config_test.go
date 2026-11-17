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
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
)

func TestConfigureEventResolver_Resolve(t *testing.T) {
	type fields struct {
		SensitiveAttributes []string
	}
	type args struct {
		ce  *commonevent.Metadata
		evt *event.Event
	}
	tests := []struct {
		name                     string
		fields                   fields
		args                     args
		wantErr                  bool
		wantOperation            string
		wantResourceType         string
		wantOperationDescription string
	}{
		{"test normal", fields{[]string{}}, args{
			ce: &commonevent.Metadata{
				Username:      "test",
				RequestURL:    "/api/v2.0/configurations",
				RequestMethod: "PUT",
			}, evt: &event.Event{}}, false, "update", "configuration", "update configuration"},
		{"test error", fields{[]string{}}, args{ce: &commonevent.Metadata{
			Username:      "test",
			RequestURL:    "/api/v2.0/configurations",
			RequestMethod: "POST",
		}, evt: &event.Event{}}, false, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &resolver{}
			if err := c.Resolve(tt.args.ce, tt.args.evt); (err != nil) != tt.wantErr {
				t.Errorf("ConfigureEventResolver.Resolve() error = %v, wantErr %v", err, tt.wantErr)
				if tt.args.evt.Data != nil {
					data := tt.args.evt.Data.(*model.CommonEvent)
					if data.Operation != tt.wantOperation {
						t.Errorf("ConfigureEventResolver.Resolve() Operation = %v, want %v", data.Operation, tt.wantOperation)
					}
					if data.ResourceType != tt.wantResourceType {
						t.Errorf("ConfigureEventResolver.Resolve() ResourceType = %v, want %v", data.ResourceType, tt.wantResourceType)
					}
					if data.OperationDescription != tt.wantOperationDescription {
						t.Errorf("ConfigureEventResolver.Resolve() OperationDescription = %v, want %v", data.OperationDescription, tt.wantOperationDescription)
					}
				}

			}
		})
	}
}

func TestConfigureEventResolver_PreCheck(t *testing.T) {
	type fields struct {
		SensitiveAttributes []string
	}
	type args struct {
		ctx    context.Context
		url    string
		method string
	}
	tests := []struct {
		name      string
		fields    fields
		args      args
		want      bool
		wantTopic string
	}{
		{"test normal", fields{[]string{}}, args{context.Background(), "/api/v2.0/configurations", "PUT"}, true, ""},
		{"test error", fields{[]string{}}, args{context.Background(), "/api/v2.0/configurations", "POST"}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &resolver{}
			got, got1 := c.PreCheck(tt.args.ctx, tt.args.url, tt.args.method)
			if got != tt.want {
				t.Errorf("ConfigureEventResolver.PreCheck() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.wantTopic {
				t.Errorf("ConfigureEventResolver.PreCheck() got1 = %v, want %v", got1, tt.wantTopic)
			}
		})
	}
}

func TestDescribeChangesShowsOldAndNew(t *testing.T) {
	before := map[string]string{"auth_mode": "db_auth", "ldap_url": "ldap://a", "self_registration": "true", "ldap_timeout": "5"}
	assert.Equal(t, []string{"auth_mode db_auth -> ldap_auth", "ldap_timeout 5 -> 10", "ldap_url ldap://a -> ldap://b", "self_registration true -> false"},
		describeChanges(`{"ldap_url":"ldap://b","auth_mode":"ldap_auth","self_registration":false,"ldap_timeout":10}`, before))
	assert.Equal(t, []string{"auth_mode db_auth -> ldap_auth"}, describeChanges(`{"auth_mode":"ldap_auth","no_such_key":1,"ldap_url":"ldap://a"}`, before),
		"unknown keys are ignored by the API and unchanged values are left out")
	assert.Equal(t, []string{"ldap_base_dn -> dc=example"}, describeChanges(`{"ldap_base_dn":"dc=example"}`, nil), "no old value read")
	assert.Equal(t, []string{`ldap_filter "" -> "(objectClass=person), x"`}, describeChanges(`{"ldap_filter":"(objectClass=person), x"}`, map[string]string{"ldap_filter": ""}))
	assert.Empty(t, describeChanges("not json", nil))
	assert.Empty(t, describeChanges(`{}`, nil))
}

func TestDescribeChangesTruncatesLongValues(t *testing.T) {
	got := describeChanges(`{"ldap_base_dn":"`+strings.Repeat("a", 500)+`"}`, map[string]string{"ldap_base_dn": "dc=x"})
	assert.Len(t, got, 1)
	assert.Equal(t, "ldap_base_dn dc=x -> "+strings.Repeat("a", maxValueLength-3)+"...", got[0])
}

func TestDescribeChangesNeverShowsSecrets(t *testing.T) {
	payload := `{"ldap_search_password":"hunter2","oidc_client_secret":"s3cr3t","robot_token_duration":30,"uaa_client_secret":"x","auth_mode":"oidc_auth"}`
	before := map[string]string{"auth_mode": "db_auth", "ldap_search_password": "old-hunter2"}
	got := describeChanges(payload, before)
	assert.Equal(t, []string{"auth_mode db_auth -> oidc_auth", "ldap_search_password changed", "oidc_client_secret changed", "robot_token_duration changed", "uaa_client_secret changed"}, got)
	joined := strings.Join(got, ", ")
	for _, secret := range []string{"hunter2", "s3cr3t", "30"} {
		assert.NotContains(t, joined, secret)
	}
}

func TestDescribeChangesNormalizesLikeTheAPI(t *testing.T) {
	assert.Empty(t, describeChanges(`{"robot_name_prefix":" robot$ "}`, map[string]string{"robot_name_prefix": "robot$"}),
		"the API trims the prefix, so nothing changed")
	assert.Equal(t, []string{"self_registration false -> true"},
		describeChanges(`{"core_url":"http://elsewhere","self_registration":true,"ldap_url":null}`, map[string]string{"self_registration": "false", "ldap_url": "ldap://a"}),
		"the update API ignores core_url and nulls")
}

func TestResolveRecordsTheRequestNotTheLiveConfig(t *testing.T) {
	evt := &event.Event{}
	ce := &commonevent.Metadata{Username: "admin", RequestURL: "/api/v2.0/configurations", RequestMethod: "PUT", ResponseCode: 200,
		RequestPayload: `{"ldap_url":"ldap://mine"}`, Before: map[string]string{"ldap_url": "ldap://a"}}
	assert.NoError(t, (&resolver{}).Resolve(ce, evt))
	assert.Equal(t, "update configuration: ldap_url ldap://a -> ldap://mine", evt.Data.(*model.CommonEvent).OperationDescription)
}

func TestDescribeChangesHidesStructuredValues(t *testing.T) {
	got := describeChanges(`{"oidc_extra_redirect_parms":"{\"client_assertion\":\"s3cr3t\"}"}`, nil)
	assert.Equal(t, []string{"oidc_extra_redirect_parms changed"}, got)
}

func TestDescribeChangesQuotesControlCharacters(t *testing.T) {
	assert.Equal(t, []string{`ldap_base_dn dc=a -> "dc=b\nadmin deleted everything"`},
		describeChanges(`{"ldap_base_dn":"dc=b\nadmin deleted everything"}`, map[string]string{"ldap_base_dn": "dc=a"}))
}

func TestSensitive(t *testing.T) {
	for _, k := range []string{"ldap_search_password", "oidc_client_secret", "ldap_verify_cert", "robot_token_duration", "some_api_key", "x_credentials", "oidc_extra_redirect_parms"} {
		assert.True(t, sensitive(k), k)
	}
	for _, k := range []string{"auth_mode", "ldap_url", "self_registration", "ldap_base_dn"} {
		assert.False(t, sensitive(k), k)
	}
}
