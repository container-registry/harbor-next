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

package security

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/common/security"
	robotCtx "github.com/goharbor/harbor/src/common/security/robot"
	"github.com/goharbor/harbor/src/common/utils"
	robot_ctl "github.com/goharbor/harbor/src/controller/robot"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/q"
)

type robot struct{}

func (r *robot) Generate(req *http.Request) (security.Context, error) {
	log := log.G(req.Context())
	name, secret, ok := req.BasicAuth()
	if !ok {
		return nil, nil
	}
	if !strings.HasPrefix(name, config.RobotPrefix(req.Context())) {
		return nil, nil
	}
	if !queryable(name) {
		log.Debugf("robot account name is not valid UTF-8 or contains NUL, rejecting")
		return nil, nil
	}
	// The robot name can be used as the unique identifier to locate robot as it contains the project name.
	robots, err := robot_ctl.Ctl.List(req.Context(), q.New(q.KeyWords{
		"name": strings.TrimPrefix(name, config.RobotPrefix(req.Context())),
	}), &robot_ctl.Option{
		WithPermission: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list robots: %w", err)
	}
	if len(robots) == 0 {
		return nil, nil
	}

	robot := robots[0]
	if utils.Encrypt(secret, robot.Salt, utils.SHA256) != robot.Secret {
		log.Errorf("failed to authenticate robot account: %s", name)
		return nil, nil
	}
	if robot.Disabled {
		log.Errorf("failed to authenticate deactivated robot account: %s", name)
		return nil, nil
	}
	now := time.Now().Unix()
	if robot.ExpiresAt != -1 && robot.ExpiresAt <= now {
		log.Errorf("the robot account is expired: %s", name)
		return nil, nil
	}

	log.Debugf("a robot security context generated for request %s %s", req.Method, req.URL.Path)
	return robotCtx.NewSecurityContext(robot), nil
}
