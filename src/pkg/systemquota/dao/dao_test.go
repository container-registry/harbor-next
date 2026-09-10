//go:build db

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

package dao

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/systemquota/model"
	htesting "github.com/goharbor/harbor/src/testing"
)

type DaoTestSuite struct {
	htesting.Suite
	dao DAO
}

func (s *DaoTestSuite) SetupSuite() {
	s.Suite.SetupSuite()
	s.ensureTable()
	s.Suite.ClearTables = []string{model.Table}
	s.dao = New()
}

// ensureTable applies the system_quota DDL from the authoritative harbor-next
// schema: the test harness only runs the numbered migrations, so harbor-next
// tables do not exist unless a test creates them. Reading the statement from
// harbor_next.sql keeps the DDL single-sourced.
func (s *DaoTestSuite) ensureTable() {
	path := filepath.Join("..", "..", "..", "..", "make", "migrations", "postgresql", "harbor_next.sql")
	schema, err := os.ReadFile(path)
	s.Require().NoError(err)
	stmt := regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS system_quota \(.*?\);`).Find(schema)
	s.Require().NotEmpty(stmt, "system_quota DDL not found in harbor_next.sql")

	o, err := orm.FromContext(s.Context())
	s.Require().NoError(err)
	_, err = o.Raw(string(stmt)).Exec()
	s.Require().NoError(err)
}

func (s *DaoTestSuite) TestSingletonLifecycle() {
	ctx := s.Context()

	_, err := s.dao.Get(ctx)
	s.True(errors.IsNotFoundErr(err))

	s.NoError(s.dao.Upsert(ctx, &model.SystemQuota{ID: 42, Hard: 1000}))
	s.NoError(s.dao.Upsert(ctx, &model.SystemQuota{Hard: 2000, Enforce: true}))

	got, err := s.dao.Get(ctx)
	s.Require().NoError(err)
	s.Equal(model.SingletonID, got.ID)
	s.Equal(int64(2000), got.Hard)
	s.True(got.Enforce)

	o, err := orm.FromContext(ctx)
	s.Require().NoError(err)
	var count int64
	s.NoError(o.Raw("SELECT COUNT(*) FROM system_quota").QueryRow(&count))
	s.Equal(int64(1), count)

	s.NoError(s.dao.Delete(ctx))
	s.NoError(s.dao.Delete(ctx))
	_, err = s.dao.Get(ctx)
	s.True(errors.IsNotFoundErr(err))
}

func (s *DaoTestSuite) TestProjectAllocation() {
	ctx := s.Context()
	o, err := orm.FromContext(ctx)
	s.Require().NoError(err)

	before, err := s.dao.ProjectAllocation(ctx)
	s.Require().NoError(err)

	// three live projects (100, 250, unlimited) and one deleted project whose
	// quota row must be ignored
	var ids []int64
	for _, name := range []string{"sq-alloc-a", "sq-alloc-b", "sq-alloc-c", "sq-alloc-deleted"} {
		var id int64
		s.Require().NoError(o.Raw(`INSERT INTO project (name, owner_id, deleted) VALUES (?, 1, ?) RETURNING project_id`,
			name, name == "sq-alloc-deleted").QueryRow(&id))
		ids = append(ids, id)
	}
	defer func() {
		_, _ = o.Raw(`DELETE FROM quota WHERE reference = 'project' AND reference_id IN (?, ?, ?, ?)`,
			fmt.Sprint(ids[0]), fmt.Sprint(ids[1]), fmt.Sprint(ids[2]), fmt.Sprint(ids[3])).Exec()
		_, _ = o.Raw(`DELETE FROM project WHERE project_id IN (?, ?, ?, ?)`, ids[0], ids[1], ids[2], ids[3]).Exec()
	}()
	for i, hard := range []string{`{"storage": 100}`, `{"storage": 250}`, `{"storage": -1}`, `{"storage": 999}`} {
		_, err = o.Raw(`INSERT INTO quota (reference, reference_id, hard) VALUES ('project', ?, ?::jsonb)`, fmt.Sprint(ids[i]), hard).Exec()
		s.Require().NoError(err)
	}

	after, err := s.dao.ProjectAllocation(ctx)
	s.Require().NoError(err)
	s.Equal(before.Allocated+350, after.Allocated)
	s.Equal(before.Unlimited+1, after.Unlimited)
}

func TestDaoTestSuite(t *testing.T) {
	suite.Run(t, &DaoTestSuite{})
}
