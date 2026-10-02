//  Copyright Project Harbor Authors
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

//go:build db

package usergroup

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/usergroup/model"
	htesting "github.com/goharbor/harbor/src/testing"
)

type ManagerTestSuite struct {
	htesting.Suite
	mgr Manager
}

func (s *ManagerTestSuite) SetupSuite() {
	s.Suite.SetupSuite()
	s.Suite.ClearTables = []string{"user_group"}
	s.mgr = newManager()
}

func (s *ManagerTestSuite) TestOnboardGroup() {
	ctx := s.Context()
	ug := &model.UserGroup{
		GroupName:   "harbor_dev",
		GroupType:   1,
		LdapGroupDN: "cn=harbor_dev,ou=groups,dc=example,dc=com",
	}
	err := s.mgr.Onboard(ctx, ug)
	s.Nil(err)
	ugs, err := s.mgr.List(ctx, q.New(q.KeyWords{"GroupType": 1, "LdapGroupDN": "cn=harbor_dev,ou=groups,dc=example,dc=com"}))
	s.Nil(err)
	s.True(len(ugs) > 0)
}

func (s *ManagerTestSuite) TestOnboardGroupWithDuplicatedName() {
	ctx := s.Context()
	ugs := []*model.UserGroup{
		{
			GroupName:   "harbor_dev",
			GroupType:   1,
			LdapGroupDN: "cn=harbor_dev,ou=groups,dc=example,dc=com",
		},
		{
			GroupName:   "harbor_dev",
			GroupType:   1,
			LdapGroupDN: "cn=harbor_dev,ou=groups,dc=example2,dc=com",
		},
		{
			GroupName:   "harbor_dev",
			GroupType:   1,
			LdapGroupDN: "cn=harbor_dev,ou=groups,dc=example3,dc=com,dc=verylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcname",
		},
	}
	for _, ug := range ugs {
		err := s.mgr.Onboard(ctx, ug)
		s.Nil(err)
	}
	// both user group should be onboard to user group
	ugs, err := s.mgr.List(ctx, q.New(q.KeyWords{"GroupType": 1, "LdapGroupDN": "cn=harbor_dev,ou=groups,dc=example,dc=com"}))
	s.Nil(err)
	s.True(len(ugs) > 0)

	ugs, err = s.mgr.List(ctx, q.New(q.KeyWords{"GroupType": 1, "LdapGroupDN": "cn=harbor_dev,ou=groups,dc=example2,dc=com"}))
	s.Nil(err)
	s.True(len(ugs) > 0)
	s.Equal("cn=harbor_dev,ou=groups,dc=example2,dc=com", ugs[0].GroupName)

	ugs, err = s.mgr.List(ctx, q.New(q.KeyWords{"GroupType": 1, "LdapGroupDN": "cn=harbor_dev,ou=groups,dc=example3,dc=com,dc=verylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcname"}))
	s.Nil(err)
	s.True(len(ugs) > 0)
	s.Equal("cn=harbor_dev,ou=groups,dc=example3,dc=com,dc=verylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcnameverylongdcna", ugs[0].GroupName)

}

func (s *ManagerTestSuite) TestPopulateGroup() {
	ctx := s.Context()
	ugs := []model.UserGroup{
		{
			GroupName:   "harbor_dev",
			GroupType:   1,
			LdapGroupDN: "cn=harbor_dev,ou=groups,dc=example,dc=com",
		},
		{
			GroupName: "myhttp_group",
			GroupType: 2,
		},
	}
	ids, err := s.mgr.Populate(ctx, ugs)
	s.Nil(err)
	s.True(len(ids) > 0)
	for _, i := range ids {
		s.True(i > 0)
	}
}

// TestPopulateConcurrentOverlappingGroups mirrors concurrent logins that onboard the
// same new groups in different orders, each inside its own request transaction.
func (s *ManagerTestSuite) TestPopulateConcurrentOverlappingGroups() {
	const (
		rounds     = 10
		logins     = 8
		groupCount = 6
	)
	for r := 0; r < rounds; r++ {
		names := make([]string, groupCount)
		for i := range names {
			names[i] = "concurrent_" + s.RandString(8)
		}

		var wg sync.WaitGroup
		got := make([][]int, logins)
		errs := make([]error, logins)
		for l := 0; l < logins; l++ {
			groups := make([]model.UserGroup, groupCount)
			for i, p := range rand.Perm(groupCount) {
				groups[i] = model.UserGroup{GroupName: names[p], GroupType: common.HTTPGroupType}
			}
			wg.Add(1)
			go func(l int) {
				defer wg.Done()
				errs[l] = orm.WithTransaction(func(ctx context.Context) error {
					ids, err := s.mgr.Populate(ctx, groups)
					got[l] = ids
					// The rest of the request keeps the transaction, and its locks, open.
					time.Sleep(20 * time.Millisecond)
					return err
				})(orm.Context())
			}(l)
		}
		wg.Wait()

		for l := 0; l < logins; l++ {
			s.Require().NoError(errs[l])
			s.Require().Lenf(got[l], groupCount, "round %d login %d lost a group membership", r, l)
		}
	}
}

func TestManagerTestSuite(t *testing.T) {
	suite.Run(t, &ManagerTestSuite{})
}
