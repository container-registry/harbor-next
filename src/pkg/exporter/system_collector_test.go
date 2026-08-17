//go:build db

package exporter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/goharbor/harbor/src/pkg/version"
)

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
				backend: NewRESTBackend(),
			},
			want: &SystemInfoCollector{
				backend: NewRESTBackend(),
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
	type fields struct {
		HarborClient *HarborClient
	}
	type args struct {
		c chan *prometheus.Desc
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *prometheus.Desc
	}{
		{
			name: "test describe",
			fields: fields{
				HarborClient: &HarborClient{},
			},
			args: args{
				c: make(chan *prometheus.Desc),
			},
			want: harborSysInfo.Desc(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitHarborClient(tt.fields.HarborClient)
			hc := &SystemInfoCollector{
				backend: NewRESTBackend(),
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
	type fields struct {
		HarborClient *HarborClient
	}
	type args struct {
		c chan prometheus.Metric
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "test collect",
			fields: fields{
				HarborClient: &HarborClient{},
			},
			args: args{
				c: make(chan prometheus.Metric),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitHarborClient(tt.fields.HarborClient)
			hc := &SystemInfoCollector{
				backend: NewRESTBackend(),
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
	type fields struct {
		HarborClient *HarborClient
	}
	// The cache is a package global and an earlier test seeds it under this same
	// key, which would short-circuit getSysInfo and leave the REST path
	// unexercised. Drop the entry so this test actually hits the backend.
	if CacheEnabled() {
		CacheDelete(systemInfoCollectorName)
	}
	// harbor_version is rendered from the version package (ldflags), not from
	// the API response, so the expectation must follow that source.
	wantVersion := version.ReleaseVersion
	if version.GitCommit != "" {
		wantVersion = fmt.Sprintf("%s-%s", version.ReleaseVersion, version.GitCommit)
	}
	data := []prometheus.Metric{
		prometheus.MustNewConstMetric(harborSysInfo.Desc(), prometheus.GaugeValue, 1, "ldap_auth", wantVersion, "true"),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2.0/systeminfo" {
			w.Write([]byte(`{"auth_mode":"ldap_auth","harbor_version":"v2.0.0","self_registration":true}`))
			w.WriteHeader(http.StatusOK)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	parse, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(parse.Port())
	tests := []struct {
		name   string
		fields fields
		want   []prometheus.Metric
	}{
		{
			name: "test get system info",
			fields: fields{
				HarborClient: &HarborClient{
					HarborScheme: "http",
					HarborHost:   parse.Hostname(),
					HarborPort:   port,
				},
			},
			want: data,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitHarborClient(tt.fields.HarborClient)
			hc := &SystemInfoCollector{
				backend: NewRESTBackend(),
			}
			if got := hc.getSysInfo(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("SystemInfoCollector.getSysInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}
