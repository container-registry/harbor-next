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

package auditlog

import (
	"context"
	"strings"

	"github.com/goharbor/harbor/src/common/security"
	robotsec "github.com/goharbor/harbor/src/common/security/robot"
	robotctl "github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/auditext/collapse"
)

// listRobots is robotctl.Ctl.List, swapped in tests.
var listRobots = func(ctx context.Context, query *q.Query) ([]*robotctl.Robot, error) {
	return robotctl.Ctl.List(ctx, query, &robotctl.Option{})
}

// IsScannerPrincipal reports whether the request behind ctx is authenticated
// as the hidden robot a scan job creates. Basic auth carries the robot from
// the database; a bearer token carries only its name, so the robot is looked
// up, and only when the name has the scanner prefix.
func IsScannerPrincipal(ctx context.Context) bool {
	sc, ok := security.FromContext(ctx)
	if !ok || !sc.IsAuthenticated() {
		return false
	}
	if r, ok := sc.(*robotsec.SecurityContext); ok {
		return r.User() != nil && collapse.IsScannerRobot(ctx, r.User().Name, r.User().Visible)
	}
	name := sc.GetUsername()
	if !collapse.IsScannerRobot(ctx, name, false) {
		return false
	}
	robots, err := listRobots(ctx, q.New(q.KeyWords{"name": strings.TrimPrefix(name, config.RobotPrefix(ctx))}))
	if err != nil {
		log.Debugf("failed to look up robot %s for audit: %v", name, err)
		return false
	}
	return len(robots) == 1 && !robots[0].Visible
}
