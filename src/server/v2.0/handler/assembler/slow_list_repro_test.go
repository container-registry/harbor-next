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

//go:build db

package assembler

import (
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"

	common_dao "github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/controller/artifact"
	"github.com/goharbor/harbor/src/lib"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	v1 "github.com/goharbor/harbor/src/pkg/scan/rest/v1"
	"github.com/goharbor/harbor/src/server/v2.0/handler/model"
)

// TestSlowArtifactListRepro is a regression guard for the artifact-list
// endpoint becoming slow (>30s TTFB) on repositories that hold many top-level
// OCI image-index artifacts whose children are not directly scannable by the
// configured scanner. Without per-request caching in scan.checker, IsScannable
// re-walks the same shared descendants once per top-level index, fanning out
// thousands of accessory/blob queries per response.
//
// The test is opt-in because it requires a database that already contains an
// affected repository. Point it at any database (a dump or a synthetic one
// produced by pushing N image-index artifacts that each reference M children)
// via the standard test env vars plus:
//
//	SLOW_REPRO_REPO=<project>/<repo>           # required
//	SLOW_REPRO_PAGE_SIZE=100                   # optional, default 100
//
//	go test -tags db -timeout 5m -count=1 -v \
//	  -run TestSlowArtifactListRepro \
//	  ./src/server/v2.0/handler/assembler/...
//
// The test measures and logs the wall-clock cost of the controller.List +
// ScanReportAssembler.Assemble path that the
// `GET /api/v2.0/projects/{p}/repositories/{r}/artifacts` handler runs and
// fails if handler-equivalent TTFB exceeds 30s — the worst-case budget before
// the standard nginx-ingress proxy_read_timeout of 60s kicks in.
func TestSlowArtifactListRepro(t *testing.T) {
	repo := os.Getenv("SLOW_REPRO_REPO")
	if repo == "" {
		t.Skip("Skipping: set SLOW_REPRO_REPO=<project>/<repo> to enable")
	}
	pageSize := int64(100)
	if v := os.Getenv("SLOW_REPRO_PAGE_SIZE"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			pageSize = n
		}
	}

	// init
	config.Init()
	common_dao.PrepareTestForPostgresSQL()

	// SQL counter via beego ORM debug
	var sqlCount int64
	prev := beegoorm.DebugLog
	beegoorm.Debug = true
	beegoorm.DebugLog = beegoorm.NewLog(countingWriter{counter: &sqlCount})
	t.Cleanup(func() {
		beegoorm.Debug = false
		beegoorm.DebugLog = prev
	})

	ctx := orm.Context()
	ctx = lib.WithAPIVersion(ctx, "v2.0")

	// step 1: List artifacts (the cheap part)
	t.Logf("Listing artifacts for %s (page_size=%d)…", repo, pageSize)
	startList := time.Now()
	atomic.StoreInt64(&sqlCount, 0)
	arts, err := artifact.Ctl.List(ctx, &q.Query{
		Keywords: map[string]any{
			"RepositoryName": repo,
		},
		PageNumber: 1,
		PageSize:   pageSize,
	}, &artifact.Option{
		WithTag:       true,
		WithLabel:     true,
		WithAccessory: true,
	})
	listElapsed := time.Since(startList)
	listSQL := atomic.LoadInt64(&sqlCount)
	if err != nil {
		t.Fatalf("artifact.Ctl.List failed after %s (%d SQL): %v", listElapsed, listSQL, err)
	}
	t.Logf("controller.List   -> %3d artifacts in %s (%d SQL)", len(arts), listElapsed, listSQL)

	if len(arts) == 0 {
		t.Fatalf("repository %q returned 0 artifacts — wrong SLOW_REPRO_REPO?", repo)
	}

	// step 2: ScanReportAssembler — this is the dominant cost.
	mimeTypes := []string{v1.MimeTypeNativeReport}
	overviewOpts := model.NewOverviewOptions(model.WithSBOM(true), model.WithVuln(true))

	// Measure assembler cost per-artifact so the hot-spot is visible.
	var totalAsm time.Duration
	var totalAsmSQL int64
	var slowest time.Duration
	var slowestID int64
	for _, a := range arts {
		am := &model.Artifact{}
		am.Artifact = *a
		startAsm := time.Now()
		atomic.StoreInt64(&sqlCount, 0)
		asm := NewScanReportAssembler(overviewOpts, mimeTypes).WithArtifacts(am)
		if err := asm.Assemble(ctx); err != nil {
			t.Logf("Assemble error for artifact %d: %v", a.ID, err)
		}
		d := time.Since(startAsm)
		n := atomic.LoadInt64(&sqlCount)
		totalAsm += d
		totalAsmSQL += n
		if d > slowest {
			slowest = d
			slowestID = a.ID
		}
	}

	// Also bench the full assembler call (all artifacts at once — same logic the
	// real handler runs).
	allModels := make([]*model.Artifact, 0, len(arts))
	for _, a := range arts {
		am := &model.Artifact{}
		am.Artifact = *a
		allModels = append(allModels, am)
	}
	startBatch := time.Now()
	atomic.StoreInt64(&sqlCount, 0)
	if err := NewScanReportAssembler(overviewOpts, mimeTypes).WithArtifacts(allModels...).Assemble(ctx); err != nil {
		t.Logf("Batch Assemble error: %v", err)
	}
	batchElapsed := time.Since(startBatch)
	batchSQL := atomic.LoadInt64(&sqlCount)

	t.Logf("ScanReportAssembler.Assemble (one-by-one)")
	t.Logf("  total: %s across %d artifacts (%d SQL)", totalAsm, len(arts), totalAsmSQL)
	t.Logf("  per-artifact avg: %s", totalAsm/time.Duration(len(arts)))
	t.Logf("  slowest single artifact: id=%d in %s", slowestID, slowest)
	t.Logf("ScanReportAssembler.Assemble (batch, same call as handler)")
	t.Logf("  total: %s across %d artifacts (%d SQL)", batchElapsed, len(arts), batchSQL)

	t.Logf("HANDLER-equivalent TTFB ≈ %s  (List=%s + AssembleBatch=%s)",
		listElapsed+batchElapsed, listElapsed, batchElapsed)

	// Regression guard: the handler path (List + batch Assemble) must stay well
	// below the nginx-ingress proxy_read_timeout default of 60s. Use 30s as the
	// guard so it fires before users hit the timeout. The cache added to
	// scan.checker reduced this from ~24s to ~1s on a 38-index-artifact repo
	// where each index referenced ~180 children.
	if listElapsed+batchElapsed > 30*time.Second {
		t.Errorf("artifact-list TTFB %s exceeds 30s budget — scan.checker caches likely regressed", listElapsed+batchElapsed)
	}
}

// countingWriter counts beego ORM SQL emissions. Each query produces one log
// line, so the line count is the SQL count.
type countingWriter struct {
	counter *int64
}

func (w countingWriter) Write(p []byte) (int, error) {
	atomic.AddInt64(w.counter, 1)
	return len(p), nil
}
