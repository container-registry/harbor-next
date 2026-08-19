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
	"testing"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/config/metadata"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	testCfg "github.com/goharbor/harbor/src/testing/lib/config"
	"github.com/goharbor/harbor/src/testing/mock"
)

func Test_verifySkipAuditLogCfg(t *testing.T) {
	config.InitWithSettings(map[string]any{common.EnableCommercialAuditLogOTLP: true})
	cfgManager := &testCfg.Manager{}
	cfgManager.On("Get", mock.Anything, common.AuditLogForwardEndpoint).
		Return(&metadata.ConfigureValue{Name: common.AuditLogForwardEndpoint, Value: ""})
	cfgManager.On("Get", mock.Anything, common.AuditLogForwardOTLPEndpoint).
		Return(&metadata.ConfigureValue{Name: common.AuditLogForwardOTLPEndpoint, Value: ""})
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
		{name: "OTLP endpoint configured", args: args{ctx: context.TODO(),
			cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "https://otel-collector:4318",
				common.SkipAuditLogDatabase: true},
			mgr: cfgManager}, wantErr: false},
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

func TestVerifySkipAuditLogCfgIgnoresDisabledOTLPEndpoint(t *testing.T) {
	config.InitWithSettings(map[string]any{common.EnableCommercialAuditLogOTLP: false})
	mgr := &testCfg.Manager{}
	mgr.On("Get", mock.Anything, common.AuditLogForwardEndpoint).
		Return(&metadata.ConfigureValue{Name: common.AuditLogForwardEndpoint, Value: ""})
	mgr.On("Get", mock.Anything, common.AuditLogForwardOTLPEndpoint).
		Return(&metadata.ConfigureValue{Name: common.AuditLogForwardOTLPEndpoint, Value: "https://collector:4318"})
	mgr.On("Get", mock.Anything, common.SkipAuditLogDatabase).
		Return(&metadata.ConfigureValue{Name: common.SkipAuditLogDatabase, Value: "false"})

	err := verifySkipAuditLogCfg(context.Background(), map[string]any{
		common.SkipAuditLogDatabase: true,
	}, mgr)
	if err == nil {
		t.Fatal("disabled OTLP endpoint must not satisfy skip-audit-database forwarding requirement")
	}
}

func TestVerifyOTLPAuditLogCfg(t *testing.T) {
	mgr := &testCfg.Manager{}
	defaults := map[string]string{
		common.AuditLogForwardOTLPEndpoint:       "https://collector:4318",
		common.AuditLogForwardOTLPAuthentication: "none",
		common.AuditLogForwardOTLPUsername:       "",
		common.AuditLogForwardOTLPPassword:       "",
	}
	for key, value := range defaults {
		mgr.On("Get", mock.Anything, key).Return(&metadata.ConfigureValue{Name: key, Value: value})
	}
	tests := []struct {
		name    string
		cfgs    map[string]any
		wantErr bool
	}{
		{name: "valid none", cfgs: map[string]any{}},
		{name: "valid basic", cfgs: map[string]any{
			common.AuditLogForwardOTLPAuthentication: "basic",
			common.AuditLogForwardOTLPUsername:       "harbor",
			common.AuditLogForwardOTLPPassword:       "secret",
		}},
		{name: "valid HTTPS URL", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "https://collector:4318"}},
		{name: "valid HTTPS URL with default port", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "https://collector"}},
		{name: "valid HTTP URL", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "http://collector:4318"}},
		{name: "valid HTTP URL with default port", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "http://collector"}},
		{name: "scheme-less endpoint", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "collector:4318"}, wantErr: true},
		{name: "invalid scheme", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "grpc://collector:4317"}, wantErr: true},
		{name: "invalid port", cfgs: map[string]any{common.AuditLogForwardOTLPEndpoint: "https://collector:70000"}, wantErr: true},
		{name: "basic missing password", cfgs: map[string]any{
			common.AuditLogForwardOTLPAuthentication: "basic",
			common.AuditLogForwardOTLPUsername:       "harbor",
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyOTLPAuditLogCfg(context.Background(), tt.cfgs, mgr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("verifyOTLPAuditLogCfg() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOTLPAuditEnabledForUpdate(t *testing.T) {
	ctx := context.Background()
	config.InitWithSettings(map[string]any{common.EnableCommercialAuditLogOTLP: false})

	if otlpAuditEnabledForUpdate(ctx, nil) {
		t.Fatal("OTLP audit must remain disabled when the feature is off")
	}
	if !otlpAuditEnabledForUpdate(ctx, map[string]any{common.EnableCommercialAuditLogOTLP: true}) {
		t.Fatal("enabling the feature in the same update must permit OTLP configuration")
	}
	if otlpAuditEnabledForUpdate(ctx, map[string]any{common.EnableCommercialAuditLogOTLP: false}) {
		t.Fatal("disabling the feature in the same update must reject OTLP configuration")
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
