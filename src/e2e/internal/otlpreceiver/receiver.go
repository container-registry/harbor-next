//go:build e2e

// Package otlpreceiver provides an in-process OTLP/HTTP logs collector for
// deployed-stack end-to-end tests.
package otlpreceiver

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"

	collectorlogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/protobuf/proto"
)

const maxRequestBytes = 4 << 20

// Record is the subset of an OTLP log record used by the audit scenarios.
type Record struct {
	EventName  string
	Attributes map[string]string
}

// Receiver accepts OTLP/HTTP protobuf exports and exposes decoded records.
type Receiver struct {
	server  *httptest.Server
	records chan Record
}

// New starts a receiver on all interfaces. advertisedHost is the hostname or
// IP Harbor should use to call back to the test process.
func New(advertisedHost string) (*Receiver, error) {
	r := &Receiver{records: make(chan Record, 128)}
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("bind OTLP receiver: %w", err)
	}

	r.server = httptest.NewUnstartedServer(http.HandlerFunc(r.handleExport))
	_ = r.server.Listener.Close()
	r.server.Listener = listener
	r.server.Start()

	if advertisedHost != "" {
		port := listener.Addr().(*net.TCPAddr).Port
		r.server.URL = "http://" + net.JoinHostPort(advertisedHost, strconv.Itoa(port))
	}
	return r, nil
}

// Endpoint returns the base OTLP/HTTP endpoint advertised to Harbor.
func (r *Receiver) Endpoint() string { return r.server.URL }

// Records returns the stream of decoded log records.
func (r *Receiver) Records() <-chan Record { return r.records }

// Close stops the receiver.
func (r *Receiver) Close() { r.server.Close() }

func (r *Receiver) handleExport(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.URL.Path != "/v1/logs" {
		http.NotFound(w, req)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxRequestBytes))
	if err != nil {
		http.Error(w, "read OTLP request", http.StatusBadRequest)
		return
	}
	var export collectorlogspb.ExportLogsServiceRequest
	if err := proto.Unmarshal(body, &export); err != nil {
		http.Error(w, "decode OTLP request", http.StatusBadRequest)
		return
	}
	for _, resourceLogs := range export.ResourceLogs {
		for _, scopeLogs := range resourceLogs.ScopeLogs {
			for _, logRecord := range scopeLogs.LogRecords {
				attrs := make(map[string]string, len(logRecord.Attributes))
				for _, attr := range logRecord.Attributes {
					attrs[attr.Key] = attr.Value.GetStringValue()
				}
				select {
				case r.records <- Record{EventName: logRecord.EventName, Attributes: attrs}:
				default:
					http.Error(w, "OTLP receiver queue full", http.StatusServiceUnavailable)
					return
				}
			}
		}
	}

	response, err := proto.Marshal(&collectorlogspb.ExportLogsServiceResponse{})
	if err != nil {
		http.Error(w, "encode OTLP response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}
