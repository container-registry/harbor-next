// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package audit

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"

	"github.com/goharbor/harbor/src/lib/metric"
	auditmodel "github.com/goharbor/harbor/src/pkg/auditext/model"
)

func TestAuditLogRecord(t *testing.T) {
	opTime := time.Date(2026, time.July, 21, 1, 2, 3, 0, time.UTC)
	observed := opTime.Add(time.Second)
	record := AuditLogRecord(&auditmodel.AuditLogExt{
		ProjectID:            42,
		Operation:            "pull",
		OperationDescription: "pull artifact",
		IsSuccessful:         false,
		ResourceType:         "artifact",
		Resource:             "library/alpine:latest",
		Username:             "anonymous",
		OpTime:               opTime,
		ClientAddress:        "192.0.2.1",
		UserAgent:            "containerd/2",
		ArtifactRepository:   "library/alpine",
		ArtifactTag:          "latest",
		ArtifactDigest:       "sha256:deadbeef",
	}, observed)

	if got, want := record.EventName(), "harbor.audit.pull_artifact"; got != want {
		t.Errorf("EventName() = %q, want %q", got, want)
	}
	if !record.Timestamp().Equal(opTime) || !record.ObservedTimestamp().Equal(observed) {
		t.Errorf("unexpected timestamps: %v %v", record.Timestamp(), record.ObservedTimestamp())
	}
	if got := record.SeverityText(); got != "WARN" {
		t.Errorf("SeverityText() = %q, want WARN", got)
	}
	if got := record.Body().AsString(); got != "pull artifact" {
		t.Errorf("Body() = %q, want pull artifact", got)
	}
	attrs := recordAttributes(record)
	for key, want := range map[string]string{
		"enduser.id":              "anonymous",
		"audit.result":            "failure",
		"client.address":          "192.0.2.1",
		"user_agent.original":     "containerd/2",
		"oci.artifact.repository": "library/alpine",
		"oci.artifact.tag":        "latest",
		"oci.manifest.digest":     "sha256:deadbeef",
	} {
		if got := attrs[key]; got != want {
			t.Errorf("attribute %q = %q, want %q", key, got, want)
		}
	}
}

func TestAuditLogRecordOmitsEmptyAttributes(t *testing.T) {
	record := AuditLogRecord(&auditmodel.AuditLogExt{Operation: "update", ResourceType: "configuration", IsSuccessful: true}, time.Now())
	attrs := recordAttributes(record)
	if _, ok := attrs["client.address"]; ok {
		t.Error("empty client.address was emitted")
	}
	if got := attrs["audit.result"]; got != "success" {
		t.Errorf("audit.result = %q, want success", got)
	}
	if !strings.Contains(record.Body().AsString(), "update configuration") {
		t.Errorf("fallback body = %q", record.Body().AsString())
	}
}

func TestAuditLogRecordNormalizesClientAddress(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{name: "IP address", address: "192.0.2.1", want: "192.0.2.1"},
		{name: "forwarded chain", address: "192.0.2.1, 198.51.100.2", want: "192.0.2.1"},
		{name: "remote address with port", address: "192.0.2.1:65123", want: "192.0.2.1"},
		{name: "IPv6 remote address with port", address: "[2001:db8::1]:65123", want: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := AuditLogRecord(&auditmodel.AuditLogExt{
				Operation:     "pull",
				ResourceType:  "artifact",
				IsSuccessful:  true,
				ClientAddress: tt.address,
			}, time.Now())
			attrs := recordAttributes(record)
			if got := attrs["client.address"]; got != tt.want {
				t.Errorf("client.address = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuditLogRecordHashesGDPRAttributes(t *testing.T) {
	record := auditLogRecord(&auditmodel.AuditLogExt{
		Operation:            "pull",
		IsSuccessful:         true,
		ResourceType:         "artifact",
		Username:             "anonymous",
		ClientAddress:        "192.0.2.1:65123",
		UserAgent:            "containerd/2",
		ArtifactRepository:   "library/alpine",
		ArtifactTag:          "latest",
		ArtifactDigest:       "sha256:deadbeef",
		OperationDescription: "pull artifact",
	}, time.Now(), true)
	attrs := recordAttributes(record)
	for key, want := range map[string]string{
		"enduser.id":              "c231edae",
		"client.address":          "db610037",
		"user_agent.original":     "b034e171",
		"oci.artifact.repository": "library/alpine",
		"oci.artifact.tag":        "latest",
		"oci.manifest.digest":     "sha256:deadbeef",
	} {
		if got := attrs[key]; got != want {
			t.Errorf("attribute %q = %q, want %q", key, got, want)
		}
	}
}

func TestValidateOTLPEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "HTTPS default port", endpoint: "https://collector"},
		{name: "HTTP default port", endpoint: "http://collector"},
		{name: "HTTPS explicit port", endpoint: "https://collector:4318"},
		{name: "HTTP explicit port", endpoint: "http://collector:4318"},
		{name: "IPv6 HTTPS default port", endpoint: "https://[2001:db8::1]"},
		{name: "scheme-less endpoint", endpoint: "collector:4318", wantErr: true},
		{name: "unsupported scheme", endpoint: "grpc://collector:4317", wantErr: true},
		{name: "invalid port", endpoint: "https://collector:70000", wantErr: true},
		{name: "credentials", endpoint: "https://user:password@collector", wantErr: true},
		{name: "path", endpoint: "https://collector/v1/logs", wantErr: true},
		{name: "query", endpoint: "https://collector?dataset=harbor", wantErr: true},
		{name: "fragment", endpoint: "https://collector#logs", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOTLPEndpoint(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateOTLPEndpoint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOTLPManagerHTTPBasicAuthAndDisable(t *testing.T) {
	requests := make(chan *collectorlogspb.ExportLogsServiceRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/logs"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("harbor:secret"))
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want %q", got, wantAuth)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var request collectorlogspb.ExportLogsServiceRequest
		if err := proto.Unmarshal(body, &request); err != nil {
			t.Errorf("decode OTLP request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- &request
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(nil)
	}))
	defer server.Close()

	manager := NewOTLPManager()
	cfg := OTLPConfig{
		Endpoint:       server.URL,
		Authentication: "basic",
		Username:       "harbor",
		Password:       "secret",
	}
	if err := manager.Reconfigure(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	state := manager.state.Load()
	state.admissions <- struct{}{}
	state.logger.Emit(context.Background(), AuditLogRecord(&auditmodel.AuditLogExt{
		Operation: "pull", ResourceType: "artifact", IsSuccessful: true, OpTime: time.Now(),
	}, time.Now()))
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-requests:
		if len(request.ResourceLogs) != 1 || len(request.ResourceLogs[0].ScopeLogs[0].LogRecords) != 1 {
			t.Fatalf("unexpected OTLP request: %v", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OTLP request")
	}
	if err := manager.Reconfigure(context.Background(), OTLPConfig{}); err != nil {
		t.Fatal(err)
	}
	if manager.state.Load() != nil {
		t.Error("empty endpoint did not disable the provider")
	}
}

func TestOTLPManagerHTTPSWithoutAuth(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			t.Error("request did not use TLS")
		}
		if got, want := r.URL.Path, "/v1/logs"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}
		requests <- struct{}{}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer server.Close()

	cfg := OTLPConfig{
		Endpoint:       server.URL,
		Authentication: "none",
	}
	state, err := newOTLPState(context.Background(), cfg, cfg.fingerprint(), otlploghttp.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	manager := NewOTLPManager()
	manager.state.Store(state)
	manager.emit(context.Background(), &auditmodel.AuditLogExt{
		Operation: "pull", ResourceType: "artifact", IsSuccessful: true, OpTime: time.Now(),
	})
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requests:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTPS OTLP request")
	}
}

func TestOTLPManagerCountsRetryExhaustion(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := OTLPConfig{
		Endpoint:       server.URL,
		Authentication: "none",
	}
	state, err := newOTLPState(context.Background(), cfg, cfg.fingerprint(), otlploghttp.WithRetry(otlploghttp.RetryConfig{
		Enabled:         true,
		InitialInterval: time.Millisecond,
		MaxInterval:     time.Millisecond,
		MaxElapsedTime:  10 * time.Millisecond,
	}))
	if err != nil {
		t.Fatal(err)
	}
	manager := NewOTLPManager()
	manager.state.Store(state)
	counter := metric.AuditLogOTLPDropped.WithLabelValues("export_error")
	before := testutil.ToFloat64(counter)
	manager.emit(context.Background(), &auditmodel.AuditLogExt{
		Operation: "pull", ResourceType: "artifact", IsSuccessful: true, OpTime: time.Now(),
	})
	_ = manager.Shutdown(context.Background())
	if got := attempts.Load(); got < 2 {
		t.Errorf("export attempts = %d, want at least 2", got)
	}
	if got := testutil.ToFloat64(counter) - before; got != 1 {
		t.Errorf("export_error drops = %v, want 1", got)
	}
}

func TestOTLPManagerQueueOverflow(t *testing.T) {
	exporter := &blockingExporter{release: make(chan struct{})}
	manager := NewOTLPManager()
	manager.state.Store(testOTLPState(exporter))
	audit := &auditmodel.AuditLogExt{Operation: "pull", ResourceType: "artifact", IsSuccessful: true}
	counter := metric.AuditLogOTLPDropped.WithLabelValues("queue_full")
	before := testutil.ToFloat64(counter)
	for range queueCapacity + 1 {
		manager.emit(context.Background(), audit)
	}
	if got := testutil.ToFloat64(counter) - before; got != 1 {
		t.Errorf("queue_full drops = %v, want 1", got)
	}
	close(exporter.release)
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCountingExporterCountsFinalExportErrors(t *testing.T) {
	admissions := make(chan struct{}, 2)
	admissions <- struct{}{}
	admissions <- struct{}{}
	exporter := &countingExporter{Exporter: errorExporter{}, admissions: admissions}
	counter := metric.AuditLogOTLPDropped.WithLabelValues("export_error")
	before := testutil.ToFloat64(counter)
	err := exporter.Export(context.Background(), make([]sdklog.Record, 2))
	if err == nil {
		t.Fatal("Export() error = nil, want error")
	}
	if got := testutil.ToFloat64(counter) - before; got != 2 {
		t.Errorf("export_error drops = %v, want 2", got)
	}
}

func TestOTLPStateCountsRecordsRemainingAtShutdown(t *testing.T) {
	state := &otelState{
		provider:   sdklog.NewLoggerProvider(),
		admissions: make(chan struct{}, 3),
	}
	for range 3 {
		state.admissions <- struct{}{}
	}
	counter := metric.AuditLogOTLPDropped.WithLabelValues("shutdown")
	before := testutil.ToFloat64(counter)
	if err := state.shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(counter) - before; got != 3 {
		t.Errorf("shutdown drops = %v, want 3", got)
	}
}

func TestOTLPManagerConcurrentReconfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	endpoint := strings.TrimPrefix(server.URL, "http://")
	manager := NewOTLPManager()
	configs := []OTLPConfig{
		{Endpoint: "http://" + endpoint, Authentication: "none"},
		{},
	}

	var wait sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := manager.Reconfigure(context.Background(), configs[i%len(configs)]); err != nil {
				errs <- err
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Reconfigure() error = %v", err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func testOTLPState(exporter sdklog.Exporter) *otelState {
	state := &otelState{admissions: make(chan struct{}, queueCapacity)}
	processor := sdklog.NewBatchProcessor(&countingExporter{Exporter: exporter, admissions: state.admissions})
	state.provider = sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(resource.Empty()),
	)
	state.logger = state.provider.Logger("test")
	return state
}

type blockingExporter struct {
	release chan struct{}
}

func (e *blockingExporter) Export(ctx context.Context, _ []sdklog.Record) error {
	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (*blockingExporter) Shutdown(context.Context) error   { return nil }
func (*blockingExporter) ForceFlush(context.Context) error { return nil }

type errorExporter struct{}

func (errorExporter) Export(context.Context, []sdklog.Record) error {
	return errors.New("retry exhausted")
}
func (errorExporter) Shutdown(context.Context) error   { return nil }
func (errorExporter) ForceFlush(context.Context) error { return nil }

func recordAttributes(record otellog.Record) map[string]string {
	attrs := make(map[string]string)
	record.WalkAttributes(func(kv otellog.KeyValue) bool {
		attrs[kv.Key] = kv.Value.AsString()
		return true
	})
	return attrs
}
