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

package project

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"time"

	ctlevent "github.com/goharbor/harbor/src/controller/event"
	"github.com/goharbor/harbor/src/controller/event/metadata/commonevent"
	"github.com/goharbor/harbor/src/controller/event/model"
	"github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/auditext/desc"
	"github.com/goharbor/harbor/src/pkg/notifier/event"
	"github.com/goharbor/harbor/src/pkg/project/models"
)

// Each URL shape is declared once and used for both resolver registration and
// project-name extraction, so the two can never drift apart.
const (
	projectPattern        = `^/api/v2\.0/projects/([^/?]+)(?:\?.*)?$`
	projectMetaPattern    = `^/api/v2\.0/projects/([^/?]+)/metadatas/(?:\?.*)?$`
	projectMetaKeyPattern = `^/api/v2\.0/projects/([^/?]+)/metadatas/([^/?]+)(?:\?.*)?$`
)

var (
	projectReg        = regexp.MustCompile(projectPattern)
	projectMetaReg    = regexp.MustCompile(projectMetaPattern)
	projectMetaKeyReg = regexp.MustCompile(projectMetaKeyPattern)
)

func init() {
	var projectEventResolver = &resolver{}
	commonevent.RegisterResolver(projectPattern, projectEventResolver)
	commonevent.RegisterResolver(projectMetaPattern, projectEventResolver)
	commonevent.RegisterResolver(projectMetaKeyPattern, projectEventResolver)
}

type resolver struct {
}

// Snapshot loads the project with its metadata before the handler runs, so
// Resolve can diff it against the stored state afterwards.
func (r *resolver) Snapshot(c *commonevent.Metadata) any {
	projStr := getProjectNameOrID(c.RequestURL)
	if projStr == "" {
		return nil
	}
	proj, err := getProject(orm.ReuseContext(c.Ctx), projStr, c.IsResourceName)
	if err != nil {
		log.Errorf("failed to snapshot project %s before %s: %v", projStr, c.RequestMethod, err)
		return nil
	}
	return proj
}

func (r *resolver) Resolve(ce *commonevent.Metadata, evt *event.Event) error {
	projStr := getProjectNameOrID(ce.RequestURL)
	if projStr == "" {
		return fmt.Errorf("failed to parse project name or ID from URL: %s", ce.RequestURL)
	}

	proj, err := getProject(orm.ReuseContext(ce.Ctx), projStr, ce.IsResourceName)
	if err != nil {
		return fmt.Errorf("failed to get project: %v", err)
	}

	e := &model.CommonEvent{}
	e.Operation = "update"
	e.Operator = ce.Username
	e.ResourceType = "project"
	e.ResourceName = proj.Name
	e.ProjectID = proj.ProjectID
	e.OccurAt = time.Now()
	if ce.ResponseCode == http.StatusOK {
		e.IsSuccessful = true
	}

	d := desc.New(ce.Username, "update", "project").Name(proj.Name)
	if before, ok := ce.Before.(*models.Project); ok && before != nil {
		after := proj.Metadata
		if !e.IsSuccessful {
			after = attemptedMetadata(ce, before.Metadata)
		}
		// TODO storage_limit (quota table) and cve_allowlist are updated by the
		// same PUT but are not diffed here yet
		diffMetadata(d, before.Metadata, after)
	}
	if !e.IsSuccessful {
		d.Error(ce.ResponseCode, ce.ResponseBody)
	}
	e.OperationDescription = d.String()

	evt.Topic = ctlevent.TopicCommonEvent
	evt.Data = e
	return nil
}

func (r *resolver) PreCheck(ctx context.Context, url string, method string) (bool, string) {
	if method != http.MethodPut && method != http.MethodPost && method != http.MethodDelete {
		return false, ""
	}

	if method == http.MethodDelete && projectReg.MatchString(url) {
		return false, ""
	}

	return config.AuditLogEventEnabled(ctx, "update_project"), ""
}

// attemptedMetadata is the metadata the request asked for, used when the
// handler rejected it and the stored state did not change.
func attemptedMetadata(ce *commonevent.Metadata, before map[string]string) map[string]string {
	after := map[string]string{}
	for k, v := range before {
		after[k] = v
	}
	url := ce.RequestURL
	switch {
	case ce.RequestMethod == http.MethodDelete && projectMetaKeyReg.MatchString(url):
		delete(after, projectMetaKeyReg.FindStringSubmatch(url)[2])
	case projectMetaKeyReg.MatchString(url), projectMetaReg.MatchString(url):
		var req map[string]any
		if json.Unmarshal([]byte(ce.RequestPayload), &req) == nil {
			for k, v := range req {
				after[k] = fmt.Sprint(v)
			}
		}
	default:
		var req struct {
			Metadata map[string]any `json:"metadata"`
			Public   *bool          `json:"public"`
		}
		if json.Unmarshal([]byte(ce.RequestPayload), &req) == nil {
			for k, v := range req.Metadata {
				after[k] = fmt.Sprint(v)
			}
			if req.Public != nil {
				after["public"] = strconv.FormatBool(*req.Public)
			}
		}
	}
	return after
}

// diffMetadata records every key that was changed, added or removed.
func diffMetadata(d *desc.Description, before, after map[string]string) {
	keys := map[string]struct{}{}
	for k := range before {
		keys[k] = struct{}{}
	}
	for k := range after {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		bv, bok := before[k]
		av, aok := after[k]
		switch {
		case bok && aok:
			d.Changed(k, typed(bv), typed(av))
		case aok:
			d.Added(k, typed(av))
		default:
			d.Removed(k, typed(bv))
		}
	}
}

// typed turns the "true"/"false" strings project metadata is stored as into
// booleans so the JSON reads naturally.
func typed(v string) any {
	if b, err := strconv.ParseBool(v); err == nil && (v == "true" || v == "false") {
		return b
	}
	return v
}

func getProjectNameOrID(url string) string {
	if m := projectMetaKeyReg.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	if m := projectMetaReg.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	if m := projectReg.FindStringSubmatch(url); len(m) > 1 {
		return m[1]
	}
	return ""
}

// getProject resolves the path segment to a project. isName comes from the
// X-Is-Resource-Name header and forces the segment to be treated as a project
// name even when it is all digits — mirroring parseProjectNameOrID in the API
// handlers.
func getProject(ctx context.Context, str string, isName bool) (*models.Project, error) {
	var projectNameOrID any = str
	if !isName {
		if v, err := strconv.ParseInt(str, 10, 64); err == nil {
			projectNameOrID = v
		}
	}
	return project.Ctl.Get(ctx, projectNameOrID)
}
