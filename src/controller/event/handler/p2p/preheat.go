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

package p2p

import (
	"context"
	"fmt"
	"time"

	"github.com/goharbor/harbor/src/controller/artifact"
	"github.com/goharbor/harbor/src/controller/artifact/processor/image"
	"github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/p2p/preheat"
	"github.com/goharbor/harbor/src/controller/scan"
	"github.com/goharbor/harbor/src/controller/tag"
	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
	libredis "github.com/goharbor/harbor/src/lib/redis"
	"github.com/goharbor/harbor/src/pkg"
	pkgArt "github.com/goharbor/harbor/src/pkg/artifact"
	scanModel "github.com/goharbor/harbor/src/pkg/scan/dao/scan"
	v1 "github.com/goharbor/harbor/src/pkg/scan/rest/v1"
)

const preheatClaimExpiration = 24 * time.Hour

// Handler ...
type Handler struct {
	// for UT mock
	artMgr  pkgArt.Manager
	scanCtl scan.Controller
	claim   func(ctx context.Context, key string) (bool, error)
}

// Name ...
func (p *Handler) Name() string {
	return "P2PPreheat"
}

// Handle ...
func (p *Handler) Handle(ctx context.Context, value any) error {
	switch v := value.(type) {
	case *event.PushArtifactEvent:
		return p.handlePushArtifact(ctx, v)
	case *event.ScanImageEvent:
		return p.handleImageScanned(ctx, v)
	case *event.ArtifactLabeledEvent:
		return p.handleArtifactLabeled(ctx, v)
	default:
		return errors.New("unsupported type")
	}
}

// IsStateful ...
func (p *Handler) IsStateful() bool {
	return false
}

func (p *Handler) handlePushArtifact(ctx context.Context, event *event.PushArtifactEvent) error {
	if event.Artifact.Type != image.ArtifactTypeImage {
		return nil
	}

	// NOTES: So far, we only support artifact with tags
	if len(event.Tags) == 0 {
		return nil
	}

	log.Debugf("preheat: artifact pushed %s:%s@%s", event.Artifact.RepositoryName, event.Tags, event.Artifact.Digest)

	art, err := artifact.Ctl.Get(ctx, event.Artifact.ID, &artifact.Option{
		WithTag:   true,
		WithLabel: true,
	})
	if err != nil {
		return err
	}

	// Only with the pushed tags, ignore other tags
	pt := make([]*tag.Tag, 0)
	for _, tg := range art.Tags {
		if tg.Name == event.Tags[0] {
			pt = append(pt, tg)
			break
		}
	}
	art.Tags = pt

	_, err = preheat.Enf.PreheatArtifact(ctx, art)
	return err
}

func (p *Handler) handleImageScanned(ctx context.Context, event *event.ScanImageEvent) error {
	log.Debugf("preheat: image scanned %s:%s", event.Artifact.Repository, event.Artifact.Tag)
	art, err := artifact.Ctl.GetByReference(ctx, event.Artifact.Repository, event.Artifact.Digest,
		&artifact.Option{
			WithTag:   true,
			WithLabel: true,
		})
	if err != nil {
		return err
	}

	var errs errors.Errors
	if len(art.Tags) > 0 && isScanCompleted(event) {
		if _, err := preheat.Enf.PreheatArtifact(ctx, art); err != nil {
			errs = append(errs, err)
		}
	}

	// Scanning an image index scans its children, which are usually untagged and would be dropped
	// by the tag filter. Preheat the tagged index that references the child instead, which is also
	// what a push of the index preheats. Only vulnerability scans are handled, as they are what the
	// vulnerability filter of the preheat policy is evaluated against.
	if event.ScanType == "" || event.ScanType == v1.ScanTypeVulnerability {
		if err := p.preheatParents(ctx, art); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

func isScanCompleted(e *event.ScanImageEvent) bool {
	return e.EventType == event.TopicScanningCompleted
}

// preheatParents preheats the tagged image indexes referencing the given artifact once the scans
// of all their children are finished. Every child fires its own scan event, and once the scans are
// all finished, any number of these events can find so, as they are handled concurrently and maybe
// by different core instances. So the preheat of the index is claimed atomically for the finished
// scans, and only the event winning the claim preheats the index.
func (p *Handler) preheatParents(ctx context.Context, child *artifact.Artifact) error {
	artMgr := pkg.ArtifactMgr
	// for UT mock
	if p.artMgr != nil {
		artMgr = p.artMgr
	}
	scanCtl := scan.DefaultController
	// for UT mock
	if p.scanCtl != nil {
		scanCtl = p.scanCtl
	}
	claim := claimPreheat
	if p.claim != nil {
		claim = p.claim
	}

	references, err := artMgr.ListReferences(ctx, q.New(q.KeyWords{"ChildID": child.ID}))
	if err != nil {
		return err
	}

	var errs errors.Errors
	seen := make(map[int64]bool, len(references))
	for _, ref := range references {
		if seen[ref.ParentID] {
			continue
		}
		seen[ref.ParentID] = true

		parent, err := artifact.Ctl.Get(ctx, ref.ParentID, &artifact.Option{
			WithTag:   true,
			WithLabel: true,
		})
		if err != nil {
			if !errors.IsNotFoundErr(err) {
				errs = append(errs, err)
			}
			continue
		}
		if len(parent.Tags) == 0 {
			continue
		}

		finishedAt, finished, err := scansFinished(ctx, scanCtl, parent)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !finished {
			log.Debugf("preheat: skip %s@%s for its scanned child %s, the scans of its children are not all finished yet", parent.RepositoryName, parent.Digest, child.Digest)
			continue
		}

		claimed, err := claim(ctx, fmt.Sprintf("p2p:preheat:scanned_index:%d:%d", parent.ID, finishedAt.UnixNano()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !claimed {
			log.Debugf("preheat: skip %s@%s for its scanned child %s, it is preheated for the same scans already", parent.RepositoryName, parent.Digest, child.Digest)
			continue
		}

		log.Debugf("preheat: preheat image index %s@%s for its scanned child %s", parent.RepositoryName, parent.Digest, child.Digest)
		if _, err := preheat.Enf.PreheatArtifact(ctx, parent); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// scansFinished reports whether the vulnerability scans of all the scannable children of the image
// index are finished, and when the last of them finished.
//
// The reports are fetched per child instead of for the whole index, as the reports of an index are
// empty as long as any child without capability of the scanner is referenced, and such a child is
// never scanned.
func scansFinished(ctx context.Context, scanCtl scan.Controller, index *artifact.Artifact) (time.Time, bool, error) {
	var (
		last     *scanModel.Report
		finished = true
	)
	walkFn := func(a *artifact.Artifact) error {
		if a.IsImageIndex() {
			return nil
		}

		reports, err := scanCtl.GetReport(ctx, a, nil)
		if err != nil {
			if errors.IsNotFoundErr(err) {
				return nil
			}
			return err
		}
		if len(reports) == 0 {
			finished = false
			return artifact.ErrBreak
		}

		for _, r := range reports {
			if !job.Status(r.Status).Final() {
				finished = false
				return artifact.ErrBreak
			}
			if last == nil || r.EndTime.After(last.EndTime) {
				last = r
			}
		}
		return nil
	}
	if err := artifact.Ctl.Walk(ctx, index, walkFn, nil); err != nil {
		return time.Time{}, false, err
	}

	if !finished || last == nil {
		return time.Time{}, false, nil
	}
	return last.EndTime, true, nil
}

func claimPreheat(ctx context.Context, key string) (bool, error) {
	client, err := libredis.GetHarborClient()
	if err != nil {
		return false, err
	}
	return client.SetNX(ctx, key, 1, preheatClaimExpiration).Result()
}

func (p *Handler) handleArtifactLabeled(ctx context.Context, event *event.ArtifactLabeledEvent) error {
	art, err := artifact.Ctl.Get(ctx, event.ArtifactID, &artifact.Option{
		WithTag:   true,
		WithLabel: true,
	})

	if err != nil {
		return err
	}

	// Only care image at this moment
	if art.Type != image.ArtifactTypeImage {
		return nil
	}
	log.Debugf("preheat: artifact labeled %s:%s", art.Artifact.RepositoryName, art.Artifact.Digest)

	_, err = preheat.Enf.PreheatArtifact(ctx, art)
	return err
}
