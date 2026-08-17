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

package model

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/lib/q"
)

func init() {
	orm.RegisterModel(
		new(RepoRecord),
	)
}

// RepoRecord holds the record of an repository in DB, all the infos are from the registry notification event.
type RepoRecord struct {
	RepositoryID int64     `orm:"pk;auto;column(repository_id)" json:"repository_id"`
	Name         string    `orm:"column(name)" json:"name"`
	ProjectID    int64     `orm:"column(project_id)"  json:"project_id"`
	Description  string    `orm:"column(description)" json:"description"`
	PullCount    int64     `orm:"column(pull_count)" json:"pull_count"`
	StarCount    int64     `orm:"column(star_count)" json:"star_count"`
	CreationTime time.Time `orm:"column(creation_time);auto_now_add" json:"creation_time"`
	UpdateTime   time.Time `orm:"column(update_time);auto_now" json:"update_time"`
}

// FilterByBlobDigest filters the repositories by the blob digest
func (r *RepoRecord) FilterByBlobDigest(_ context.Context, qs orm.QuerySeter, _ string, value any) orm.QuerySeter {
	digest, ok := value.(string)
	if !ok || len(digest) == 0 {
		return qs
	}

	sql := fmt.Sprintf(`select distinct(a.repository_id)
				from artifact as a
				join artifact_blob as ab
				on a.digest = ab.digest_af
				where ab.digest_blob = %s`, orm.QuoteLiteral(digest))
	return qs.FilterRaw("repository_id", fmt.Sprintf("in (%s)", sql))
}

// FilterByName owns ALL repository name fuzzy searches (the q "Name" keyword
// dispatches here). It preserves the existing image-repo substring match on the
// raw value and additionally matches the readable multiformat storage tree: a native
// coordinate typed with its native separators ("com.acme:widget2",
// "org.springframework.boot") never substring-matches the stored slash-delimited
// tree path ("maven/com/acme/widget2"), so a second LIKE is OR'd with the maven
// group/artifact separators ('.' and ':') rewritten to '/'.
func (r *RepoRecord) FilterByName(_ context.Context, qs orm.QuerySeter, _ string, value any) orm.QuerySeter {
	// A "name=~<v>" query dispatches here with a *q.FuzzyMatchValue, while an exact
	// "name=<v>" delivers a plain string; accept both.
	var name string
	switch v := value.(type) {
	case *q.FuzzyMatchValue:
		name = v.Value
	case string:
		name = v
	}
	if len(name) == 0 {
		return qs
	}

	// Raw value preserves today's image-repo matches (e.g. "nginx").
	raw := orm.QuoteLiteral("%" + orm.Escape(name) + "%")
	cond := fmt.Sprintf("like %s", raw)

	// Only attempt the readable-tree variant when the value carries a native
	// separator; plain OCI searches skip the extra branch.
	if strings.ContainsAny(name, ".:") {
		tree := strings.NewReplacer(".", "/", ":", "/").Replace(name)
		if tree != name {
			treeLit := orm.QuoteLiteral("%" + orm.Escape(tree) + "%")
			cond = fmt.Sprintf("%s or name like %s", cond, treeLit)
		}
	}

	return qs.FilterRaw("name", cond)
}

// TableName is required by beego orm to map RepoRecord to table repository
func (r *RepoRecord) TableName() string {
	return "repository"
}

// GetDefaultSorts specifies the default sorts
func (r *RepoRecord) GetDefaultSorts() []*q.Sort {
	return []*q.Sort{
		{
			Key:  "CreationTime",
			DESC: true,
		},
		{
			Key:  "RepositoryID",
			DESC: true,
		},
	}
}
