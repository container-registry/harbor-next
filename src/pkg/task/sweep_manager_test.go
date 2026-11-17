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

package task

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/jobservice/job"
	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/task/dao"
	htesting "github.com/goharbor/harbor/src/testing"
	"github.com/goharbor/harbor/src/testing/mock"
)

type sweepManagerTestSuite struct {
	htesting.Suite
	execDao *mockExecutionDAO
	mgr     *sweepManager
}

func TestSweepManager(t *testing.T) {
	suite.Run(t, &sweepManagerTestSuite{})
}

func (suite *sweepManagerTestSuite) SetupSuite() {
	suite.Suite.SetupSuite()
	suite.execDao = &mockExecutionDAO{}
	suite.mgr = &sweepManager{execDAO: suite.execDao}
}

func (suite *sweepManagerTestSuite) TestGetCandidateMaxStartTime() {
	// test error case
	suite.execDao.On("List", mock.Anything, mock.Anything).Return(nil, errors.New("failed to list executions")).Once()
	startTime, err := suite.mgr.getCandidateMaxStartTime(context.TODO(), "WEBHOOK", 1, 10)
	suite.Error(err, "should got error")
	suite.Nil(startTime)
	// test normal case
	now := time.Now()
	execs := []*dao.Execution{{ID: 1, StartTime: now}}
	suite.execDao.On("List", mock.Anything, mock.Anything).Return(execs, nil)
	startTime, err = suite.mgr.getCandidateMaxStartTime(context.TODO(), "WEBHOOK", 1, 10)
	suite.NoError(err, "should not got error")
	suite.Equal(now.String(), startTime.String())
}

func (suite *sweepManagerTestSuite) Test_sweepManager_FixDanglingStateExecution() {
	err := suite.mgr.FixDanglingStateExecution(suite.Context())
	suite.NoError(err, "should not got error")
}

func (suite *sweepManagerTestSuite) TestListOrphanArtifactExecutions() {
	ctx := suite.Context()
	execDAO := dao.NewExecutionDAO()
	// no artifact has this id
	missing := int64(2_000_000_000)
	done, err := execDAO.Create(ctx, &dao.Execution{VendorType: job.ImageScanJobVendorType, VendorID: missing,
		Status: job.SuccessStatus.String(), Trigger: "MANUAL", ExtraAttrs: "{}", StartTime: time.Now()})
	suite.Require().NoError(err)
	defer execDAO.Delete(ctx, done)
	running, err := execDAO.Create(ctx, &dao.Execution{VendorType: job.ImageScanJobVendorType, VendorID: missing + 1,
		Status: job.RunningStatus.String(), Trigger: "MANUAL", ExtraAttrs: "{}", StartTime: time.Now()})
	suite.Require().NoError(err)
	defer execDAO.Delete(ctx, running)

	// pre-2.9 rows kept a scanner id as vendor_id; the artifact lives in extra_attrs
	ormer, err := orm.FromContext(ctx)
	suite.Require().NoError(err)
	var artID int64
	suite.Require().NoError(ormer.Raw(`INSERT INTO artifact (project_id, repository_id, repository_name, digest, type, media_type, manifest_media_type, artifact_type)
		VALUES (1, 1, 'library/sweep', 'sha256:sweeptest', 'IMAGE', 'm', 'm', 'a') RETURNING id`).QueryRow(&artID))
	defer ormer.Raw("DELETE FROM artifact WHERE id = ?", artID).Exec()
	legacy, err := execDAO.Create(ctx, &dao.Execution{VendorType: job.ImageScanJobVendorType, VendorID: missing + 2,
		Status: job.SuccessStatus.String(), Trigger: "MANUAL", ExtraAttrs: fmt.Sprintf(`{"artifact":{"id":%d}}`, artID), StartTime: time.Now()})
	suite.Require().NoError(err)
	defer execDAO.Delete(ctx, legacy)

	// unconverted row whose stale vendor_id happens to match a live artifact
	stale, err := execDAO.Create(ctx, &dao.Execution{VendorType: job.ImageScanJobVendorType, VendorID: artID,
		Status: job.SuccessStatus.String(), Trigger: "MANUAL", ExtraAttrs: fmt.Sprintf(`{"artifact":{"id":%d}}`, missing), StartTime: time.Now()})
	suite.Require().NoError(err)
	defer execDAO.Delete(ctx, stale)

	// a deleted index whose execution still backs the report of a live child
	backing, err := execDAO.Create(ctx, &dao.Execution{VendorType: job.ImageScanJobVendorType, VendorID: missing + 3,
		Status: job.SuccessStatus.String(), Trigger: "MANUAL", ExtraAttrs: "{}", StartTime: time.Now()})
	suite.Require().NoError(err)
	defer execDAO.Delete(ctx, backing)
	taskDAO := dao.NewTaskDAO()
	tid, err := taskDAO.Create(ctx, &dao.Task{ExecutionID: backing, Status: job.SuccessStatus.String(),
		StatusCode: job.SuccessStatus.Code(), ExtraAttrs: `{"report_uuids":["sweep-child-report"]}`})
	suite.Require().NoError(err)
	defer taskDAO.Delete(ctx, tid)
	_, err = ormer.Raw(`INSERT INTO scan_report (uuid, digest, registration_uuid, mime_type, report) VALUES ('sweep-child-report', 'sha256:sweeptest', 'r', 'm', '{}')`).Exec()
	suite.Require().NoError(err)
	defer ormer.Raw("DELETE FROM scan_report WHERE uuid = 'sweep-child-report'").Exec()

	ids, err := suite.mgr.listOrphanArtifactExecutions(ctx, job.ImageScanJobVendorType)
	suite.Require().NoError(err)
	suite.Contains(ids, done)
	suite.NotContains(ids, running)
	suite.NotContains(ids, legacy)
	suite.Contains(ids, stale)
	suite.NotContains(ids, backing)
}
