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

package metadata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/lib/orm"
	htesting "github.com/goharbor/harbor/src/testing"
)

type managerTestSuite struct {
	htesting.Suite
	mgr       Manager
	projectID int64
}

func (s *managerTestSuite) SetupSuite() {
	s.Suite.SetupSuite()
	s.mgr = New()
}

func (s *managerTestSuite) SetupTest() {
	o, err := orm.FromContext(s.Context())
	s.Require().NoError(err)
	s.Require().NoError(o.Raw("INSERT INTO project (owner_id, name) VALUES (1, ?) RETURNING project_id", "metadata-validated-update").QueryRow(&s.projectID))
	s.Require().NoError(s.mgr.Add(s.Context(), s.projectID, map[string]string{"pattern": "a", "kind": "x"}))
}

func (s *managerTestSuite) TearDownTest() {
	s.ExecSQL("DELETE FROM project_metadata WHERE project_id = ?", s.projectID)
	s.ExecSQL("DELETE FROM project WHERE project_id = ?", s.projectID)
}

func (s *managerTestSuite) TestValidatorSeesStoredMetadata() {
	var seen map[string]string
	err := s.mgr.UpdateWithValidation(s.Context(), s.projectID, map[string]string{"pattern": "b"}, func(m map[string]string) error {
		seen = m
		return nil
	})
	s.Require().NoError(err)
	s.Equal(map[string]string{"pattern": "b", "kind": "x"}, seen)

	stored, err := s.mgr.Get(s.Context(), s.projectID)
	s.Require().NoError(err)
	s.Equal("b", stored["pattern"])
}

func (s *managerTestSuite) TestRejectedUpdateIsNotStored() {
	invalid := errors.New("invalid")
	err := s.mgr.UpdateWithValidation(s.Context(), s.projectID, map[string]string{"pattern": "b"}, func(map[string]string) error {
		return invalid
	})
	s.ErrorIs(err, invalid)

	stored, err := s.mgr.Get(s.Context(), s.projectID)
	s.Require().NoError(err)
	s.Equal("a", stored["pattern"])
}

func (s *managerTestSuite) TestUpdateCreatesMissingKey() {
	err := s.mgr.UpdateWithValidation(s.Context(), s.projectID, map[string]string{"extra": "c"}, func(map[string]string) error {
		return nil
	})
	s.Require().NoError(err)

	stored, err := s.mgr.Get(s.Context(), s.projectID)
	s.Require().NoError(err)
	s.Equal(map[string]string{"pattern": "a", "kind": "x", "extra": "c"}, stored)
}

func (s *managerTestSuite) TestAddWithValidation() {
	var seen map[string]string
	err := s.mgr.AddWithValidation(s.Context(), s.projectID, map[string]string{"extra": "c"}, func(m map[string]string) error {
		seen = m
		return nil
	})
	s.Require().NoError(err)
	s.Equal(map[string]string{"pattern": "a", "kind": "x", "extra": "c"}, seen)

	invalid := errors.New("invalid")
	err = s.mgr.AddWithValidation(s.Context(), s.projectID, map[string]string{"other": "d"}, func(map[string]string) error {
		return invalid
	})
	s.ErrorIs(err, invalid)

	err = s.mgr.AddWithValidation(s.Context(), s.projectID, map[string]string{"pattern": "b"}, func(map[string]string) error {
		return nil
	})
	s.Error(err, "adding an existing key must conflict")

	stored, err := s.mgr.Get(s.Context(), s.projectID)
	s.Require().NoError(err)
	s.Equal(map[string]string{"pattern": "a", "kind": "x", "extra": "c"}, stored)
}

func (s *managerTestSuite) TestDeleteWithValidation() {
	var seen map[string]string
	invalid := errors.New("invalid")
	err := s.mgr.DeleteWithValidation(s.Context(), s.projectID, func(m map[string]string) error {
		seen = m
		return invalid
	}, "kind")
	s.ErrorIs(err, invalid)
	s.Equal(map[string]string{"pattern": "a"}, seen)

	err = s.mgr.DeleteWithValidation(s.Context(), s.projectID, func(map[string]string) error { return nil }, "kind")
	s.Require().NoError(err)

	stored, err := s.mgr.Get(s.Context(), s.projectID)
	s.Require().NoError(err)
	s.Equal(map[string]string{"pattern": "a"}, stored)
}

func (s *managerTestSuite) TestDeleteAllWithValidation() {
	var seen map[string]string
	err := s.mgr.DeleteWithValidation(s.Context(), s.projectID, func(m map[string]string) error {
		seen = m
		return nil
	})
	s.Require().NoError(err)
	s.Empty(seen)

	stored, err := s.mgr.Get(s.Context(), s.projectID)
	s.Require().NoError(err)
	s.Empty(stored)
}

func (s *managerTestSuite) TestMissingProject() {
	err := s.mgr.UpdateWithValidation(s.Context(), s.projectID+100000, map[string]string{"pattern": "b"}, func(map[string]string) error {
		return nil
	})
	s.Error(err)
}

// A concurrent validated update waits for the first one and validates against its write.
func (s *managerTestSuite) TestConcurrentUpdatesSerialize() {
	held := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- s.mgr.UpdateWithValidation(s.Context(), s.projectID, map[string]string{"kind": "y"}, func(map[string]string) error {
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-first:
		s.FailNowf("lock holder exited before holding the lock", "error: %v", err)
	}

	var seen map[string]string
	second := make(chan error, 1)
	go func() {
		second <- s.mgr.UpdateWithValidation(s.Context(), s.projectID, map[string]string{"pattern": "b"}, func(m map[string]string) error {
			seen = m
			return nil
		})
	}()
	select {
	case err := <-second:
		close(release)
		<-first
		s.FailNowf("second update did not wait for the first", "error: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	s.Require().NoError(<-first)
	s.Require().NoError(<-second)
	s.Equal(map[string]string{"pattern": "b", "kind": "y"}, seen)
}

// Two transactions that each insert a metadata row before a validated write
// both hold KEY SHARE on the project row; the validated lock must not deadlock.
func (s *managerTestSuite) TestValidatedWritesAfterInsertsDoNotDeadlock() {
	inserted := make(chan struct{}, 2)
	proceed := make(chan struct{})
	errs := make(chan error, 2)
	for _, key := range []string{"x", "y"} {
		go func() {
			errs <- orm.WithTransaction(func(ctx context.Context) error {
				if err := s.mgr.Add(ctx, s.projectID, map[string]string{key: "1"}); err != nil {
					return err
				}
				inserted <- struct{}{}
				<-proceed
				return s.mgr.UpdateWithValidation(ctx, s.projectID, map[string]string{"pattern": key}, func(map[string]string) error {
					return nil
				})
			})(s.Context())
		}()
	}
	for range 2 {
		select {
		case <-inserted:
		case err := <-errs:
			s.FailNowf("transaction exited before inserting", "error: %v", err)
		}
	}
	close(proceed)
	s.NoError(<-errs)
	s.NoError(<-errs)
}

func TestManager(t *testing.T) {
	suite.Run(t, &managerTestSuite{})
}
