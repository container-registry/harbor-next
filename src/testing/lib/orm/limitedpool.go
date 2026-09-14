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

package orm

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"sync"
	"testing"
	"time"

	beegoorm "github.com/beego/beego/v2/client/orm"
)

const limitedDriverName = "harbor-limited-pool"

func init() {
	sql.Register(limitedDriverName, limitedDriver{})
}

var (
	limitedOnce sync.Once
	limitedDB   *sql.DB
	limitedErr  error
)

// RegisterLimitedPool registers a stub database capped at maxConns connections
// as Beego's "default" alias and returns it. Queries answer with an empty result
// set; the point is connection accounting, not query results.
//
// It stands in for Harbor's pgxpool: database/sql parks an acquire beyond the cap
// with no deadline, exactly as the pool does, so code needing a second connection
// while holding the first never returns.
//
// Beego's alias registry is process-global, so the alias is registered once per
// test binary and later calls only resize the cap. Do not use it in a package
// whose tests register a real database.
func RegisterLimitedPool(t testing.TB, maxConns int) *sql.DB {
	t.Helper()

	limitedOnce.Do(func() {
		db := sql.OpenDB(limitedConnector{})
		// A connection recycled between acquires would hide a missing release.
		db.SetConnMaxIdleTime(0)
		db.SetConnMaxLifetime(0)

		if err := beegoorm.RegisterDriver(limitedDriverName, beegoorm.DRPostgres); err != nil {
			limitedErr = err
			return
		}
		if err := beegoorm.AddAliasWthDB("default", limitedDriverName, db); err != nil {
			limitedErr = err
			return
		}
		limitedDB = db
	})
	if limitedErr != nil {
		t.Fatalf("register limited pool: %v", limitedErr)
	}

	limitedDB.SetMaxOpenConns(maxConns)
	limitedDB.SetMaxIdleConns(maxConns)
	return limitedDB
}

// RunsWithin reports whether fn returns within d. On timeout fn keeps running in
// its own goroutine, parked on the pool with nothing left to wake it.
func RunsWithin(d time.Duration, fn func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

type limitedConnector struct{}

func (limitedConnector) Connect(context.Context) (driver.Conn, error) { return &limitedConn{}, nil }
func (limitedConnector) Driver() driver.Driver                        { return limitedDriver{} }

type limitedDriver struct{}

func (limitedDriver) Open(string) (driver.Conn, error) { return &limitedConn{}, nil }

type limitedConn struct{}

func (*limitedConn) Prepare(string) (driver.Stmt, error) { return limitedStmt{}, nil }
func (*limitedConn) Close() error                        { return nil }
func (*limitedConn) Begin() (driver.Tx, error)           { return limitedTx{}, nil }

func (*limitedConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return limitedTx{}, nil
}

func (*limitedConn) Ping(context.Context) error { return nil }

func (*limitedConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return emptyRows{}, nil
}

func (*limitedConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

type limitedTx struct{}

func (limitedTx) Commit() error   { return nil }
func (limitedTx) Rollback() error { return nil }

type limitedStmt struct{}

func (limitedStmt) Close() error  { return nil }
func (limitedStmt) NumInput() int { return -1 }

func (limitedStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

func (limitedStmt) Query([]driver.Value) (driver.Rows, error) { return emptyRows{}, nil }

type emptyRows struct{}

func (emptyRows) Columns() []string         { return nil }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }
