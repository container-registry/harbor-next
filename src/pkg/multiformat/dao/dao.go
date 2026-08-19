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

// Package dao is the PostgreSQL projection access layer for multiformat. It is a
// rebuildable derived view over OCI (reconcilable from the `_index`); it never
// holds authoritative mutable state. Access goes through src/lib/orm so the
// operations participate in the caller's transaction (orm.WithTransaction) when
// one is active.
package dao

import (
	"context"
	"encoding/json"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"

	"github.com/goharbor/harbor/src/lib/errors"
	"github.com/goharbor/harbor/src/lib/orm"
	"github.com/goharbor/harbor/src/pkg/multiformat/model"
)

// DAO is the projection access layer.
type DAO interface {
	// UpsertVersion ensures the package, upserts the version row, replaces
	// dist-tags, and bumps proj_version atomically. lastIndexDigest records the
	// _index revision this projection now reflects. Returns the new proj_version.
	// It must run inside an orm.WithTransaction scope.
	UpsertVersion(ctx context.Context, projectID int64, format, name string, v model.Version, distTags map[string]string, lastIndexDigest string) (int64, error)
	// SetMutableState updates a package's dist-tags projection and bumps
	// proj_version atomically, without touching version rows.
	SetMutableState(ctx context.Context, projectID int64, format, name string, distTags map[string]string, lastIndexDigest string) (int64, error)
	// DeleteVersion removes one projected version, updates mutable state, and
	// removes the package row when no versions remain.
	DeleteVersion(ctx context.Context, projectID int64, format, name, version string, distTags map[string]string, lastIndexDigest string) (int64, error)
	// LoadState reads the full typed PackageState. ok==false means the package
	// is unknown to the projection (cold).
	LoadState(ctx context.Context, projectID int64, format, name string) (model.PackageState, bool, error)
	// ProjVersion reads the current proj_version (freshness is PG-sourced).
	ProjVersion(ctx context.Context, projectID int64, format, name string) (int64, bool, error)
	// WipeAll truncates the projection (reconcile gate / tests).
	WipeAll(ctx context.Context) error
	// AdvisoryLock acquires a session-level pg advisory lock keyed by name and
	// returns an unlock func. The lock is held on a dedicated connection so the
	// caller can perform registry round-trips without pinning a pooled
	// transaction connection.
	AdvisoryLock(ctx context.Context, key string) (unlock func(), err error)
}

// New returns a DAO.
func New() DAO {
	return &dao{}
}

type dao struct{}

func (d *dao) UpsertVersion(ctx context.Context, projectID int64, format, name string, v model.Version, distTags map[string]string, lastIndexDigest string) (int64, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}
	mutable, err := json.Marshal(map[string]any{"dist-tags": distTags})
	if err != nil {
		return 0, err
	}
	var pkgID, projVersion int64
	err = o.Raw(`
         INSERT INTO multi_format_package (project_id, format, native_name, proj_version, mutable_state, last_index_digest)
         VALUES (?,?,?,1,?,?)
         ON CONFLICT (project_id, format, native_name)
         DO UPDATE SET proj_version = multi_format_package.proj_version + 1,
                       mutable_state = ?,
                       last_index_digest = ?,
                       update_time = now()
         RETURNING id, proj_version`,
		projectID, format, name, string(mutable), lastIndexDigest, string(mutable), lastIndexDigest).
		QueryRow(&pkgID, &projVersion)
	if err != nil {
		return 0, errors.Wrap(err, "upsert multi_format_package")
	}
	meta := v.Meta
	if len(meta) == 0 {
		meta = []byte("{}")
	}
	if _, err = o.Raw(`
         INSERT INTO multi_format_version (package_id, version, payload_digest, payload_size, yanked, created_at, meta)
         VALUES (?,?,?,?,?,?,?)
         ON CONFLICT (package_id, version)
         DO UPDATE SET payload_digest = EXCLUDED.payload_digest,
                       payload_size   = EXCLUDED.payload_size,
                       meta           = EXCLUDED.meta`,
		pkgID, v.Version, v.PayloadDigest, v.PayloadSize, v.Yanked, v.Created, string(meta)).Exec(); err != nil {
		return 0, errors.Wrap(err, "upsert multi_format_version")
	}
	return projVersion, nil
}

func (d *dao) SetMutableState(ctx context.Context, projectID int64, format, name string, distTags map[string]string, lastIndexDigest string) (int64, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}
	mutable, err := json.Marshal(map[string]any{"dist-tags": distTags})
	if err != nil {
		return 0, err
	}
	var pv int64
	err = o.Raw(`
         UPDATE multi_format_package
         SET proj_version = proj_version + 1,
             mutable_state = ?,
             last_index_digest = ?,
             update_time = now()
         WHERE project_id=? AND format=? AND native_name=?
         RETURNING proj_version`,
		string(mutable), lastIndexDigest, projectID, format, name).QueryRow(&pv)
	if errors.Is(err, orm.ErrNoRows) {
		return 0, errors.NotFoundError(nil).WithMessage("package not found in projection")
	}
	return pv, err
}

func (d *dao) DeleteVersion(ctx context.Context, projectID int64, format, name, version string, distTags map[string]string, lastIndexDigest string) (int64, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return 0, err
	}
	var pkgID int64
	if err := o.Raw(`SELECT id FROM multi_format_package WHERE project_id=? AND format=? AND native_name=?`,
		projectID, format, name).QueryRow(&pkgID); err != nil {
		if errors.Is(err, orm.ErrNoRows) {
			return 0, errors.NotFoundError(nil).WithMessage("package not found")
		}
		return 0, err
	}
	res, err := o.Raw(`DELETE FROM multi_format_version WHERE package_id=? AND version=?`, pkgID, version).Exec()
	if err != nil {
		return 0, errors.Wrap(err, "delete multi_format_version")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, errors.NotFoundError(nil).WithMessagef("version %s not found", version)
	}
	var remaining int64
	if err := o.Raw(`SELECT count(*) FROM multi_format_version WHERE package_id=?`, pkgID).QueryRow(&remaining); err != nil {
		return 0, err
	}
	if remaining == 0 {
		_, err := o.Raw(`DELETE FROM multi_format_package WHERE id=?`, pkgID).Exec()
		return 0, err
	}
	mutable, err := json.Marshal(map[string]any{"dist-tags": distTags})
	if err != nil {
		return 0, err
	}
	var pv int64
	err = o.Raw(`UPDATE multi_format_package SET proj_version=proj_version+1, mutable_state=?, last_index_digest=?, update_time=now() WHERE id=? RETURNING proj_version`,
		string(mutable), lastIndexDigest, pkgID).QueryRow(&pv)
	return pv, err
}

func (d *dao) LoadState(ctx context.Context, projectID int64, format, name string) (model.PackageState, bool, error) {
	st := model.PackageState{Format: format, Name: name, DistTags: map[string]string{}}
	o, err := orm.FromContext(ctx)
	if err != nil {
		return st, false, err
	}
	var pkgID int64
	var mutable string
	err = o.Raw(`
         SELECT id, proj_version, mutable_state FROM multi_format_package
         WHERE project_id=? AND format=? AND native_name=?`,
		projectID, format, name).QueryRow(&pkgID, &st.ProjVersion, &mutable)
	if errors.Is(err, orm.ErrNoRows) {
		return st, false, nil
	}
	if err != nil {
		return st, false, err
	}
	var ms struct {
		DistTags map[string]string `json:"dist-tags"`
	}
	if err := json.Unmarshal([]byte(mutable), &ms); err == nil && ms.DistTags != nil {
		st.DistTags = ms.DistTags
	}

	type verRow struct {
		Version       string    `orm:"column(version)"`
		PayloadDigest string    `orm:"column(payload_digest)"`
		PayloadSize   int64     `orm:"column(payload_size)"`
		Yanked        bool      `orm:"column(yanked)"`
		CreatedAt     time.Time `orm:"column(created_at)"`
		Meta          string    `orm:"column(meta)"`
	}
	var rows []verRow
	if _, err := o.Raw(`
         SELECT version, payload_digest, payload_size, yanked, created_at, meta
         FROM multi_format_version WHERE package_id=?`, pkgID).QueryRows(&rows); err != nil {
		return st, false, err
	}
	for _, r := range rows {
		v := model.Version{
			Version:       r.Version,
			PayloadDigest: r.PayloadDigest,
			PayloadSize:   r.PayloadSize,
			Yanked:        r.Yanked,
			Created:       r.CreatedAt.UTC(),
			Meta:          []byte(r.Meta),
		}
		// Multi-file (Maven) versions store their file set as the meta config (a
		// JSON array of FileRef). Decode it back so Files is first-class on the
		// typed PackageState after the PG round-trip (single-payload formats store
		// a JSON object meta, which fails this decode and leaves Files nil).
		if len(v.Meta) > 0 && v.Meta[0] == '[' {
			var files []model.FileRef
			if err := json.Unmarshal(v.Meta, &files); err == nil {
				v.Files = files
			}
		}
		st.Versions = append(st.Versions, v)
	}
	return st, true, nil
}

func (d *dao) ProjVersion(ctx context.Context, projectID int64, format, name string) (int64, bool, error) {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return 0, false, err
	}
	var pv int64
	err = o.Raw(`SELECT proj_version FROM multi_format_package WHERE project_id=? AND format=? AND native_name=?`,
		projectID, format, name).QueryRow(&pv)
	if errors.Is(err, orm.ErrNoRows) {
		return 0, false, nil
	}
	return pv, err == nil, err
}

func (d *dao) WipeAll(ctx context.Context) error {
	o, err := orm.FromContext(ctx)
	if err != nil {
		return err
	}
	_, err = o.Raw(`TRUNCATE multi_format_version, multi_format_package RESTART IDENTITY CASCADE`).Exec()
	return err
}

// AdvisoryLock acquires a session-level advisory lock on a dedicated *sql.Conn
// keyed by hashtext(key). The lock is held outside any beego-pooled transaction
// connection so the publish critical section may perform registry HTTP
// round-trips without pinning a transaction. The returned unlock func releases
// the lock and returns the connection to the pool; it is safe to call once.
func (d *dao) AdvisoryLock(ctx context.Context, key string) (func(), error) {
	db, err := beegoorm.GetDB()
	if err != nil {
		return nil, err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtext($1))`, key); err != nil {
		_ = conn.Close()
		return nil, errors.Wrap(err, "acquire advisory lock")
	}
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		// Release on a background context so unlock still runs if the request
		// context was cancelled mid critical-section.
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext($1))`, key)
		_ = conn.Close()
	}, nil
}
