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

package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/orm"
	htesting "github.com/goharbor/harbor/src/testing"
)

type rowLockTestSuite struct {
	htesting.Suite
	a, b int64
}

func (s *rowLockTestSuite) SetupTest() {
	s.a = s.createProject("rowlock-a")
	s.b = s.createProject("rowlock-b")
}

func (s *rowLockTestSuite) TearDownTest() {
	s.ExecSQL("DELETE FROM project WHERE project_id IN (?, ?)", s.a, s.b)
}

func (s *rowLockTestSuite) createProject(name string) int64 {
	var id int64
	o, err := orm.FromContext(s.Context())
	s.Require().NoError(err)
	s.Require().NoError(o.Raw("INSERT INTO project (owner_id, name) VALUES (1, ?) RETURNING project_id", name).QueryRow(&id))
	return id
}

// hold runs f under the delete lock of id in the background until release is closed
func (s *rowLockTestSuite) hold(id int64, f func(context.Context) error) (release chan struct{}, done chan error) {
	held := make(chan struct{})
	release = make(chan struct{})
	done = make(chan error, 1)
	go func() {
		done <- WithDeleteLock(s.Context(), id, func(ctx context.Context) error {
			if err := f(ctx); err != nil {
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-done:
		s.FailNowf("lock holder exited before holding the lock", "error: %v", err)
	}
	return release, done
}

func (s *rowLockTestSuite) TestDeletesOfDifferentProjectsDoNotWait() {
	release, done := s.hold(s.a, func(context.Context) error { return nil })
	defer func() { close(release); s.NoError(<-done) }()

	finished := make(chan error, 1)
	go func() {
		finished <- WithDeleteLock(s.Context(), s.b, func(context.Context) error { return nil })
	}()
	select {
	case err := <-finished:
		s.NoError(err)
	case <-time.After(2 * time.Second):
		s.Fail("delete lock on another project waited")
	}
}

func (s *rowLockTestSuite) TestExistsWaitsForDeleteOfSameProject() {
	release, done := s.hold(s.a, func(ctx context.Context) error {
		o, err := orm.FromContext(ctx)
		if err != nil {
			return err
		}
		_, err = o.Raw("UPDATE project SET deleted = true, name = name || '#' || project_id WHERE project_id = ?", s.a).Exec()
		return err
	})

	result := make(chan bool, 1)
	go func() {
		_ = orm.WithTransaction(func(ctx context.Context) error {
			exists, err := ExistsLocked(ctx, "rowlock-a")
			s.NoError(err)
			result <- exists
			return nil
		})(s.Context())
	}()
	select {
	case <-result:
		close(release)
		<-done
		s.FailNow("existence check did not wait for the delete")
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	s.Require().NoError(<-done)
	s.False(<-result, "a project deleted while the check waited must not count as existing")
}

func (s *rowLockTestSuite) TestDeleteWaitsForDefaultChangeOfSameProject() {
	locked := make(chan struct{})
	release := make(chan struct{})
	setDone := make(chan error, 1)
	go func() {
		setDone <- orm.WithTransaction(func(ctx context.Context) error {
			exists, err := ExistsLocked(ctx, "rowlock-a")
			if err != nil {
				return err
			}
			s.True(exists)
			close(locked)
			<-release
			return nil
		})(s.Context())
	}()
	select {
	case <-locked:
	case err := <-setDone:
		s.FailNowf("existence check exited before holding the lock", "error: %v", err)
	}

	deleted := make(chan error, 1)
	go func() {
		deleted <- WithDeleteLock(s.Context(), s.a, func(context.Context) error { return nil })
	}()
	select {
	case <-deleted:
		close(release)
		<-setDone
		s.FailNow("delete did not wait for the default project change")
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	s.Require().NoError(<-setDone)
	s.NoError(<-deleted)
}

func TestRowLockTestSuite(t *testing.T) {
	suite.Run(t, &rowLockTestSuite{})
}
