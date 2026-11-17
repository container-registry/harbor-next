//  Copyright Project Harbor Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package config

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	"github.com/goharbor/harbor/src/lib/errors"
	testCfg "github.com/goharbor/harbor/src/testing/lib/config"
	"github.com/goharbor/harbor/src/testing/mock"
)

func Test_verifySkipAuditLogCfg(t *testing.T) {
	cfgManager := &testCfg.Manager{}
	cfgManager.On("Get", mock.Anything, common.AuditLogForwardEndpoint).
		Return(&metadata.ConfigureValue{Name: common.AuditLogForwardEndpoint, Value: ""})
	cfgManager.On("Get", mock.Anything, common.SkipAuditLogDatabase).
		Return(&metadata.ConfigureValue{Name: common.SkipAuditLogDatabase, Value: "true"})
	type args struct {
		ctx  context.Context
		cfgs map[string]any
		mgr  config.Manager
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "both configured", args: args{ctx: context.TODO(),
			cfgs: map[string]any{common.AuditLogForwardEndpoint: "harbor-log:15041",
				common.SkipAuditLogDatabase: true},
			mgr: cfgManager}, wantErr: false},
		{name: "no forward endpoint config", args: args{ctx: context.TODO(),
			cfgs: map[string]any{common.SkipAuditLogDatabase: true},
			mgr:  cfgManager}, wantErr: true},
		{name: "none configured", args: args{ctx: context.TODO(),
			cfgs: map[string]any{},
			mgr:  cfgManager}, wantErr: false},
		{name: "enabled skip audit log database, but change log forward endpoint to empty", args: args{ctx: context.TODO(),
			cfgs: map[string]any{common.AuditLogForwardEndpoint: ""},
			mgr:  cfgManager}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := verifySkipAuditLogCfg(tt.args.ctx, tt.args.cfgs, tt.args.mgr); (err != nil) != tt.wantErr {
				t.Errorf("verifySkipAuditLogCfg() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func Test_verifyAuditLogEndpointCfg(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if err := verifyAuditLogEndpointCfg(map[string]any{common.AuditLogForwardEndpoint: l.Addr().String()}); err != nil {
		t.Errorf("reachable endpoint: unexpected error %v", err)
	}
	if err := verifyAuditLogEndpointCfg(map[string]any{common.AuditLogForwardEndpoint: "no-port"}); err == nil {
		t.Error("unreachable endpoint: expected an error")
	}
	if err := verifyAuditLogEndpointCfg(map[string]any{common.AuditLogForwardEndpoint: ""}); err != nil {
		t.Errorf("empty endpoint: unexpected error %v", err)
	}
	if err := verifyAuditLogEndpointCfg(map[string]any{}); err != nil {
		t.Errorf("no endpoint: unexpected error %v", err)
	}
}

func Test_maxValueLimitedByLength(t *testing.T) {
	type args struct {
		length int
	}
	tests := []struct {
		name string
		args args
		want int64
	}{
		{name: "negative length should return -1", args: args{0}, want: -1},
		{name: "input length 1 should return 9", args: args{1}, want: 9},
		{name: "input length 5 should return 99999", args: args{5}, want: 99999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxValueLimitedByLength(tt.args.length); got != tt.want {
				t.Errorf("maxValueLimitedByLength() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_verifyValueLengthCfg(t *testing.T) {
	type args struct {
		ctx  context.Context
		cfgs map[string]any
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{name: "valid config", args: args{context.TODO(), map[string]any{
			common.TokenExpiration:    float64(100),
			common.RobotTokenDuration: float64(100),
			common.SessionTimeout:     float64(100),
		}}, wantErr: false},
		{name: "invalid config with negative value", args: args{context.TODO(), map[string]any{
			common.TokenExpiration:    float64(-1),
			common.RobotTokenDuration: float64(100),
			common.SessionTimeout:     float64(100),
		}}, wantErr: true},
		{name: "invalid config with value over length limit", args: args{context.TODO(), map[string]any{
			common.TokenExpiration:    float64(100),
			common.RobotTokenDuration: float64(100000000000000000),
			common.SessionTimeout:     float64(100),
		}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := verifyValueLengthCfg(tt.args.ctx, tt.args.cfgs); (err != nil) != tt.wantErr {
				t.Errorf("verifyMaxLengthCfg() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyDefaultProjectNameCfg(t *testing.T) {
	existing := map[string]bool{"library": true, "a": true}
	c := &controller{projectExists: func(_ context.Context, name string) (bool, error) {
		if name == "dberr" {
			return false, errors.New("db down")
		}
		return existing[name], nil
	}}
	tests := []struct {
		name    string
		cfgs    map[string]any
		wantErr bool
	}{
		{name: "absent", cfgs: map[string]any{}, wantErr: false},
		{name: "empty disables fallback", cfgs: map[string]any{common.DefaultProjectName: ""}, wantErr: false},
		{name: "existing project", cfgs: map[string]any{common.DefaultProjectName: "library"}, wantErr: false},
		{name: "missing project", cfgs: map[string]any{common.DefaultProjectName: "missing"}, wantErr: true},
		{name: "lookup error", cfgs: map[string]any{common.DefaultProjectName: "dberr"}, wantErr: true},
		{name: "uppercase", cfgs: map[string]any{common.DefaultProjectName: "Library"}, wantErr: true},
		{name: "contains slash", cfgs: map[string]any{common.DefaultProjectName: "a/b"}, wantErr: true},
		{name: "contains space", cfgs: map[string]any{common.DefaultProjectName: "a b"}, wantErr: true},
		{name: "leading separator", cfgs: map[string]any{common.DefaultProjectName: "_lib"}, wantErr: true},
		{name: "single char", cfgs: map[string]any{common.DefaultProjectName: "a"}, wantErr: false},
		{name: "over max length", cfgs: map[string]any{common.DefaultProjectName: strings.Repeat("a", 256)}, wantErr: true},
		{name: "not a string", cfgs: map[string]any{common.DefaultProjectName: 42}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := c.verifyDefaultProjectNameCfg(context.TODO(), tt.cfgs); (err != nil) != tt.wantErr {
				t.Errorf("verifyDefaultProjectNameCfg() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultProjectPinnedByEnv(t *testing.T) {
	t.Setenv(common.DefaultProjectNameEnv, "library")
	c := &controller{}
	err := c.UpdateUserConfigs(context.TODO(), map[string]any{common.DefaultProjectName: "other"})
	if !errors.IsErr(err, errors.ForbiddenCode) {
		t.Errorf("UpdateUserConfigs() error = %v, want forbidden while %s is set", err, common.DefaultProjectNameEnv)
	}
}
