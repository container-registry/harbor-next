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
	"fmt"
	"hash/crc32"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otellog "go.opentelemetry.io/otel/log"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/metric"
	auditmodel "github.com/goharbor/harbor/src/pkg/auditext/model"
	"github.com/goharbor/harbor/src/pkg/commercial"
)

const (
	auditLoggerName = "harbor.audit"
	queueCapacity   = 2048
	shutdownTimeout = 30 * time.Second
)

// OTLPConfig contains the effective OTLP/HTTP audit forwarding configuration.
type OTLPConfig struct {
	Endpoint       string
	Authentication string
	Username       string
	Password       string
}

// ValidateOTLPConfig validates the supported OTLP/HTTP transport and authentication settings.
func ValidateOTLPConfig(cfg OTLPConfig) error {
	if cfg.Endpoint == "" {
		return nil
	}
	if err := validateOTLPEndpoint(cfg.Endpoint); err != nil {
		return err
	}
	switch cfg.Authentication {
	case "none":
		if cfg.Username != "" || cfg.Password != "" {
			return errors.New("OTLP audit log username and password must be empty when authentication is none")
		}
	case "basic":
		if cfg.Username == "" || cfg.Password == "" {
			return errors.New("OTLP audit log username and password are required for basic authentication")
		}
	default:
		return errors.New("OTLP audit log authentication must be none or basic")
	}
	return nil
}

func validateOTLPEndpoint(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" {
		return errors.New("OTLP audit log endpoint must be an HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("OTLP audit log endpoint URL scheme must be http or https")
	}
	if port := parsed.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return errors.New("OTLP audit log endpoint port must be between 1 and 65535")
		}
	}
	if parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("OTLP audit log endpoint URL must not include credentials, a path, query, or fragment")
	}
	return nil
}

func otlpConfig(ctx context.Context) OTLPConfig {
	return OTLPConfig{
		Endpoint:       config.AuditLogForwardOTLPEndpoint(ctx),
		Authentication: config.AuditLogForwardOTLPAuthentication(ctx),
		Username:       config.AuditLogForwardOTLPUsername(ctx),
		Password:       config.AuditLogForwardOTLPPassword(ctx),
	}
}

func (c OTLPConfig) fingerprint() string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s", c.Endpoint, c.Authentication, c.Username, c.Password)
}

// OTLPLogMgr forwards extended audit events through an independent OTLP Logs pipeline.
var OTLPLogMgr = NewOTLPManager()

// OTLPManager atomically reconfigures and owns the audit LoggerProvider.
type OTLPManager struct {
	state atomic.Pointer[otelState]
	mu    sync.Mutex
}

type otelState struct {
	fingerprint string
	provider    *sdklog.LoggerProvider
	logger      otellog.Logger
	admissions  chan struct{}
	mu          sync.RWMutex
	closed      bool
}

// NewOTLPManager returns a disabled OTLP audit manager.
func NewOTLPManager() *OTLPManager {
	return &OTLPManager{}
}

// Forward admits an audit event without blocking user-facing operations.
func (m *OTLPManager) Forward(ctx context.Context, audit *auditmodel.AuditLogExt) {
	if audit == nil || !commercial.Enabled(ctx, commercial.AuditLogOTLP) {
		return
	}
	if err := m.Reconfigure(ctx, otlpConfig(ctx)); err != nil {
		log.Errorf("failed to configure OTLP audit log forwarder: %v", err)
		return
	}
	m.emit(ctx, audit)
}

func (m *OTLPManager) emit(ctx context.Context, audit *auditmodel.AuditLogExt) {
	state := m.state.Load()
	if state == nil {
		return
	}

	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.closed {
		metric.AuditLogOTLPDropped.WithLabelValues("shutdown").Inc()
		return
	}
	select {
	case state.admissions <- struct{}{}:
		state.logger.Emit(ctx, auditLogRecord(audit, time.Now(), auditLogGDPR(ctx)))
	default:
		metric.AuditLogOTLPDropped.WithLabelValues("queue_full").Inc()
		log.Error("OTLP audit log queue is full; dropping record")
	}
}

// Reconfigure replaces the active provider when the effective configuration changes.
func (m *OTLPManager) Reconfigure(ctx context.Context, cfg OTLPConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fingerprint := cfg.fingerprint()
	if current := m.state.Load(); current != nil && current.fingerprint == fingerprint {
		return nil
	}
	if cfg.Endpoint == "" {
		shutdownState(m.state.Swap(nil))
		return nil
	}

	state, err := newOTLPState(ctx, cfg, fingerprint)
	if err != nil {
		return err
	}
	shutdownState(m.state.Swap(state))
	return nil
}

// Shutdown flushes and disables the current OTLP audit provider.
func (m *OTLPManager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.state.Swap(nil)
	if state == nil {
		return nil
	}
	return state.shutdown(ctx)
}

func newOTLPState(ctx context.Context, cfg OTLPConfig, fingerprint string, extraOptions ...otlploghttp.Option) (*otelState, error) {
	if err := ValidateOTLPConfig(cfg); err != nil {
		return nil, err
	}
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpointURL(cfg.Endpoint),
		otlploghttp.WithURLPath("/v1/logs"),
	}
	if cfg.Authentication == "basic" {
		credentials := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
		opts = append(opts, otlploghttp.WithHeaders(map[string]string{"Authorization": "Basic " + credentials}))
	}
	opts = append(opts, extraOptions...)

	exporter, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP audit exporter: %w", err)
	}
	state := &otelState{fingerprint: fingerprint, admissions: make(chan struct{}, queueCapacity)}
	processor := sdklog.NewBatchProcessor(&countingExporter{Exporter: exporter, admissions: state.admissions})
	state.provider = sdklog.NewLoggerProvider(
		sdklog.WithProcessor(processor),
		sdklog.WithResource(resource.NewSchemaless(attribute.String("service.name", "harbor-core"))),
	)
	state.logger = state.provider.Logger(auditLoggerName)
	return state, nil
}

func shutdownState(state *otelState) {
	if state == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := state.shutdown(ctx); err != nil {
		log.Errorf("failed to shut down OTLP audit log provider: %v", err)
	}
}

func (s *otelState) shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	err := s.provider.Shutdown(ctx)
	remaining := len(s.admissions)
	if remaining > 0 {
		metric.AuditLogOTLPDropped.WithLabelValues("shutdown").Add(float64(remaining))
		for range remaining {
			<-s.admissions
		}
	}
	return err
}

type countingExporter struct {
	sdklog.Exporter
	admissions chan struct{}
}

func (e *countingExporter) Export(ctx context.Context, records []sdklog.Record) error {
	err := e.Exporter.Export(ctx, records)
	for range records {
		<-e.admissions
	}
	if err != nil {
		metric.AuditLogOTLPDropped.WithLabelValues("export_error").Add(float64(len(records)))
		log.Errorf("failed to export %d OTLP audit log records: %v", len(records), err)
	}
	return err
}

// AuditLogRecord maps an extended Harbor audit event to an OTel LogRecord.
func AuditLogRecord(audit *auditmodel.AuditLogExt, observed time.Time) otellog.Record {
	return auditLogRecord(audit, observed, false)
}

func auditLogRecord(audit *auditmodel.AuditLogExt, observed time.Time, gdpr bool) otellog.Record {
	var record otellog.Record
	record.SetTimestamp(audit.OpTime)
	record.SetObservedTimestamp(observed)
	record.SetEventName(fmt.Sprintf("harbor.audit.%s_%s", strings.ToLower(audit.Operation), strings.ToLower(audit.ResourceType)))
	severity := otellog.SeverityInfo
	severityText := "INFO"
	result := "success"
	if !audit.IsSuccessful {
		severity = otellog.SeverityWarn
		severityText = "WARN"
		result = "failure"
	}
	record.SetSeverity(severity)
	record.SetSeverityText(severityText)
	body := audit.OperationDescription
	if body == "" {
		body = fmt.Sprintf("%s %s %s", audit.Operation, audit.ResourceType, audit.Resource)
	}
	record.SetBody(otellog.StringValue(strings.TrimSpace(body)))

	attrs := make([]otellog.KeyValue, 0, 11)
	addString := func(key, value string) {
		if value != "" {
			if gdpr && shouldHashAuditAttribute(key) {
				value = auditAttributeChecksum(value)
			}
			attrs = append(attrs, otellog.String(key, value))
		}
	}
	addString("enduser.id", audit.Username)
	addString("audit.action", audit.Operation)
	addString("audit.resource", audit.Resource)
	addString("audit.resource_type", audit.ResourceType)
	if audit.ProjectID != 0 {
		attrs = append(attrs, otellog.Int64("harbor.project.id", audit.ProjectID))
	}
	addString("audit.result", result)
	addString("client.address", clientAddressAttribute(audit.ClientAddress))
	addString("user_agent.original", audit.UserAgent)
	addString("oci.artifact.repository", audit.ArtifactRepository)
	addString("oci.artifact.tag", audit.ArtifactTag)
	addString("oci.manifest.digest", audit.ArtifactDigest)
	record.AddAttributes(attrs...)
	return record
}

func auditLogGDPR(ctx context.Context) bool {
	return config.DefaultMgr().Get(ctx, common.GDPRAuditLogs).GetBool()
}

func shouldHashAuditAttribute(key string) bool {
	switch key {
	case "enduser.id", "client.address", "user_agent.original":
		return true
	default:
		return false
	}
}

func auditAttributeChecksum(value string) string {
	return fmt.Sprintf("%08x", crc32.Checksum([]byte(value), crc32.IEEETable))
}

func clientAddressAttribute(address string) string {
	address = strings.TrimSpace(strings.Split(address, ",")[0])
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return strings.Trim(address, "[]")
}
