//go:build db

package exporter

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/goharbor/harbor/src/pkg/version"
)

type fakeSysInfoBackend struct {
	info *responseSysInfo
}

func (f *fakeSysInfoBackend) Health(context.Context) (*responseHealth, error) { return nil, nil }

func (f *fakeSysInfoBackend) SystemInfo(context.Context) (*responseSysInfo, error) {
	return f.info, nil
}

func (f *fakeSysInfoBackend) JobService(context.Context) (*JobServiceBackend, error) {
	return nil, nil
}

func TestNewSystemInfoCollector(t *testing.T) {
	type args struct {
		backend Backend
	}
	tests := []struct {
		name string
		args args
		want *SystemInfoCollector
	}{
		{
			name: "test new system info collector",
			args: args{
				backend: &fakeSysInfoBackend{},
			},
			want: &SystemInfoCollector{
				backend: &fakeSysInfoBackend{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewSystemInfoCollector(tt.args.backend); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewSystemInfoCollector() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemInfoCollector_Describe(t *testing.T) {
	type args struct {
		c chan *prometheus.Desc
	}
	tests := []struct {
		name string
		args args
		want *prometheus.Desc
	}{
		{
			name: "test describe",
			args: args{
				c: make(chan *prometheus.Desc),
			},
			want: harborSysInfo.Desc(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := &SystemInfoCollector{
				backend: &fakeSysInfoBackend{},
			}
			go hc.Describe(tt.args.c)
			desc := <-tt.args.c
			if !reflect.DeepEqual(tt.want, desc) {
				t.Errorf("SystemInfoCollector.Describe() = %v, want %v", desc, harborSysInfo.Desc())
			}
		})
	}
}

func TestSystemInfoCollector_Collect(t *testing.T) {
	CacheInit(&Opt{
		CacheDuration: 60,
	})
	data := []prometheus.Metric{
		prometheus.MustNewConstMetric(harborSysInfo.Desc(), prometheus.GaugeValue, 1, "ldap_auth", "v2.0.0", "true"),
	}
	CachePut(systemInfoCollectorName, data)
	type args struct {
		c chan prometheus.Metric
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "test collect",
			args: args{
				c: make(chan prometheus.Metric),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := &SystemInfoCollector{
				backend: &fakeSysInfoBackend{},
			}
			go hc.Collect(tt.args.c)
			metric := <-tt.args.c
			if !reflect.DeepEqual(metric, data[0]) {
				t.Errorf("SystemInfoCollector.Collect() = %v, want %v", metric, data[0])
			}
		})
	}
}

func TestSystemInfoCollector_getSysInfo(t *testing.T) {
	// The cache is a package global and an earlier test seeds it under this same
	// key, which would short-circuit getSysInfo and leave the backend unexercised.
	if CacheEnabled() {
		CacheDelete(systemInfoCollectorName)
	}
	// harbor_version is rendered from the version package (ldflags), not from
	// the backend response, so the expectation must follow that source.
	wantVersion := version.ReleaseVersion
	if version.GitCommit != "" {
		wantVersion = fmt.Sprintf("%s-%s", version.ReleaseVersion, version.GitCommit)
	}
	want := []prometheus.Metric{
		prometheus.MustNewConstMetric(harborSysInfo.Desc(), prometheus.GaugeValue, 1, "ldap_auth", wantVersion, "true"),
	}
	hc := &SystemInfoCollector{
		backend: &fakeSysInfoBackend{info: &responseSysInfo{
			AuthMode:         "ldap_auth",
			HarborVersion:    "v2.0.0",
			SelfRegistration: true,
		}},
	}
	if got := hc.getSysInfo(); !reflect.DeepEqual(got, want) {
		t.Errorf("SystemInfoCollector.getSysInfo() = %v, want %v", got, want)
	}
}
