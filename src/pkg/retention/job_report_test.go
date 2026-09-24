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

package retention

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/selector"
)

// captureLogger records what logResults renders instead of printing it.
type captureLogger struct {
	lines []string
}

func (l *captureLogger) Debug(...any)          {}
func (l *captureLogger) Debugf(string, ...any) {}
func (l *captureLogger) Info(...any)           {}
func (l *captureLogger) Infof(format string, v ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, v...))
}
func (l *captureLogger) Warning(...any)          {}
func (l *captureLogger) Warningf(string, ...any) {}
func (l *captureLogger) Error(...any)            {}
func (l *captureLogger) Errorf(string, ...any)   {}
func (l *captureLogger) Fatal(...any)            {}
func (l *captureLogger) Fatalf(string, ...any)   {}

// TestLogResultsTable pins the retention report. Nothing else asserts it, so a
// change in how tablewriter renders would otherwise pass unnoticed.
func TestLogResultsTable(t *testing.T) {
	retained := &selector.Candidate{
		NamespaceID:  1,
		Namespace:    "library",
		Repository:   "busybox",
		Kind:         "image",
		Digest:       "sha256:aaa",
		Tags:         []string{"latest", "1.36"},
		Labels:       []string{"prod"},
		PushedTime:   1600000000,
		PulledTime:   1600000001,
		CreationTime: 1600000002,
	}
	deleted := &selector.Candidate{
		NamespaceID:  1,
		Namespace:    "library",
		Repository:   "busybox",
		Kind:         "image",
		Digest:       "sha256:bbb",
		Tags:         []string{"old"},
		PushedTime:   1600000000,
		PulledTime:   1600000001,
		CreationTime: 1600000002,
	}

	logger := &captureLogger{}
	logResults(logger, []*selector.Candidate{retained, deleted}, []*selector.Result{{Target: deleted}})

	require.Len(t, logger.lines, 1, "logResults logs the table once")

	pushed := t2s(retained.PushedTime)
	pulled := t2s(retained.PulledTime)
	created := t2s(retained.CreationTime)

	// Borders on the left and right only, "|" as the separator, and headers
	// kept as written rather than upper-cased.
	expected := fmt.Sprintf("\n"+
		"|   Digest   |     Tag     | Kind  | Labels |     PushedTime      |     PulledTime      |     CreatedTime     | Retention |\n"+
		"|------------|-------------|-------|--------|---------------------|---------------------|---------------------|-----------|\n"+
		"| sha256:aaa | latest,1.36 | image | prod   | %s | %s | %s | RETAIN    |\n"+
		"| sha256:bbb | old         | image |        | %s | %s | %s | DEL       |\n",
		pushed, pulled, created, pushed, pulled, created)

	assert.Equal(t, expected, logger.lines[0])
}

// t2s reaches the package-level t() that logResults uses for the timestamp
// columns, which the *testing.T parameter shadows inside the test.
func t2s(tm int64) string {
	return t(tm)
}
