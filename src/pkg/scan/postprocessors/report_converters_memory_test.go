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

package postprocessors

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/scan/dao/scan"
	"github.com/goharbor/harbor/src/pkg/scan/report"
	"github.com/goharbor/harbor/src/pkg/scan/vuln"
)

// Converting one report used to parse it twice and keep both trees alive at once, which is what
// puts jobservice over its memory limit during a Scan All (#478). The database is faked out so the
// measurement is the conversion itself and nothing else.

// Both fakes embed the interface rather than implementing it in full: only the methods the
// conversion path actually calls are defined, and anything else panics loudly instead of passing
// silently. Hand-rolled rather than mockery, whose argument capture would be most of what the
// allocation numbers below measure.
type fakeVulnRecordDao struct {
	scan.VulnerabilityRecordDao
	nextID int64
}

func (d *fakeVulnRecordDao) List(_ context.Context, _ *q.Query) ([]*scan.VulnerabilityRecord, error) {
	return nil, nil
}

func (d *fakeVulnRecordDao) Create(_ context.Context, _ *scan.VulnerabilityRecord) (int64, error) {
	d.nextID++
	return d.nextID, nil
}

func (d *fakeVulnRecordDao) SyncForReport(_ context.Context, _ string, _ ...int64) error { return nil }

type fakeReportMgr struct {
	report.Manager
	rp *scan.Report
}

func (m *fakeReportMgr) List(_ context.Context, _ *q.Query) ([]*scan.Report, error) {
	return []*scan.Report{m.rp}, nil
}

func (m *fakeReportMgr) Update(_ context.Context, _ *scan.Report, _ ...string) error { return nil }

// syntheticReport builds a report of n vulnerabilities shaped like what the Trivy adapter returns:
// a long description, several links and CVSS details are what make a real report tens of MB.
func syntheticReport(n int) string {
	const description = "A flaw was found in the way the package handles untrusted input. " +
		"An attacker able to reach the affected code path can cause a crash or, depending on the " +
		"allocator, achieve memory corruption. This entry exists to give the synthetic report the " +
		"same per-item weight a real scanner report has."

	items := make([]*vuln.VulnerabilityItem, 0, n)
	for i := range n {
		score := 7.5
		items = append(items, &vuln.VulnerabilityItem{
			ID:          fmt.Sprintf("CVE-2026-%06d", i),
			Package:     fmt.Sprintf("package-%d", i%512),
			Version:     "1.2.3-4",
			FixVersion:  "1.2.4-1",
			Severity:    vuln.High,
			Description: description,
			Links: []string{
				fmt.Sprintf("https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2026-%06d", i),
				fmt.Sprintf("https://security-tracker.example.org/tracker/CVE-2026-%06d", i),
				fmt.Sprintf("https://git.example.org/project/commit/%040d", i),
			},
			CWEIds:      []string{"CWE-787", "CWE-125"},
			CVSSDetails: vuln.CVSS{ScoreV3: &score, VectorV3: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
		})
	}

	data, err := json.Marshal(&vuln.Report{
		GeneratedAt:     "2026-09-14T00:00:00Z",
		Severity:        vuln.High,
		Vulnerabilities: items,
	})
	if err != nil {
		panic(err)
	}
	return string(data)
}

func newFakeConverter() NativeScanReportConverter {
	return &nativeToRelationalSchemaConverter{dao: &fakeVulnRecordDao{}}
}

// peakLiveHeap reports the high-water mark of the live heap while fn runs, over the heap already
// in use before it started.
//
// Peak live heap, not total allocation: an OOM kill is decided by how much is resident at one
// moment, and total allocation counts garbage the collector would have reclaimed anyway. Running
// at GOGC=1 keeps HeapAlloc close to genuinely reachable bytes, which is also how the process
// behaves once it is up against GOMEMLIMIT or a cgroup ceiling.
func peakLiveHeap(fn func()) uint64 {
	restore := debug.SetGCPercent(1)
	defer debug.SetGCPercent(restore)

	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc

	var peak atomic.Uint64
	done := make(chan struct{})
	var sampler sync.WaitGroup
	sampler.Add(1)
	go func() {
		defer sampler.Done()
		var s runtime.MemStats
		for {
			select {
			case <-done:
				return
			default:
			}
			runtime.ReadMemStats(&s) // stops the world, so this samples rather than traces
			for {
				seen := peak.Load()
				if s.HeapAlloc <= seen || peak.CompareAndSwap(seen, s.HeapAlloc) {
					break
				}
			}
		}
	}()

	fn()
	close(done)
	sampler.Wait()

	if peak.Load() < base {
		return 0
	}
	return peak.Load() - base
}

func TestToRelationalSchemaDoesNotParseTheReportTwice(t *testing.T) {
	const vulnerabilities = 5000 // the size of report that OOM-killed jobservice in #478

	raw := syntheticReport(vulnerabilities)
	restore := report.Mgr
	report.Mgr = &fakeReportMgr{rp: &scan.Report{UUID: "report-uuid"}}
	t.Cleanup(func() { report.Mgr = restore })

	c := newFakeConverter()
	ctx := context.Background()

	// Warm up, so lazily initialised package state is not billed to the measured run.
	_, _, err := c.ToRelationalSchema(ctx, "report-uuid", "registration-uuid", "sha256:cafe", raw)
	require.NoError(t, err)

	var summary string
	peak := peakLiveHeap(func() {
		_, summary, err = c.ToRelationalSchema(ctx, "report-uuid", "registration-uuid", "sha256:cafe", raw)
	})
	require.NoError(t, err)

	// The summary is the report with its vulnerability list dropped, which is what gets persisted.
	var parsed vuln.Report
	require.NoError(t, json.Unmarshal([]byte(summary), &parsed))
	require.Empty(t, parsed.Vulnerabilities)

	ratio := float64(peak) / float64(len(raw))
	t.Logf("report %d vulnerabilities, %.1f MiB raw; conversion peaked at %.1f MiB live heap (%.1fx the report)",
		vulnerabilities, float64(len(raw))/(1<<20), float64(peak)/(1<<20), ratio)

	// Holding the report once, as parsed structs plus the records built from them, peaks at about 1.2x.
	// Holding a second complete parse at the same time takes it to about 3.1x. The bound sits
	// between the two to catch a second full copy being reintroduced, not to pin an exact figure.
	require.Less(t, ratio, 2.0,
		"conversion peaked at %.1fx the raw report in live heap; the report is being held more times than it needs to be (#478)", ratio)
}

func BenchmarkToRelationalSchema(b *testing.B) {
	for _, vulnerabilities := range []int{500, 5000} {
		b.Run(fmt.Sprintf("vulns=%d", vulnerabilities), func(b *testing.B) {
			raw := syntheticReport(vulnerabilities)
			restore := report.Mgr
			report.Mgr = &fakeReportMgr{rp: &scan.Report{UUID: "report-uuid"}}
			b.Cleanup(func() { report.Mgr = restore })

			c := newFakeConverter()
			ctx := context.Background()
			b.SetBytes(int64(len(raw)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, _, err := c.ToRelationalSchema(ctx, "report-uuid", "registration-uuid", "sha256:cafe", raw); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
