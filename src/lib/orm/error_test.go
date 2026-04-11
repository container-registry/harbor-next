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
	"testing"

	"github.com/beego/beego/v2/client/orm"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/errors"
)

func TestIsNotFoundError(t *testing.T) {
	// nil error
	err := AsNotFoundError(nil, "")
	assert.Nil(t, err)

	// common error
	err = AsNotFoundError(errors.New("common error"), "")
	assert.Nil(t, err)

	// pass
	message := "message"
	err = AsNotFoundError(orm.ErrNoRows, "%s", "message")
	require.NotNil(t, err)
	assert.Equal(t, errors.NotFoundCode, err.Code)
	assert.Equal(t, message, err.Message)
}

func TestIsConflictError(t *testing.T) {
	// nil error
	err := AsConflictError(nil, "")
	assert.Nil(t, err)

	// common error
	err = AsConflictError(errors.New("common error"), "")
	assert.Nil(t, err)

	// pass
	message := "message"
	err = AsConflictError(&pgconn.PgError{
		Code: "23505",
	}, "%s", message)
	require.NotNil(t, err)
	assert.Equal(t, errors.ConflictCode, err.Code)
	assert.Equal(t, message, err.Message)
}

// TestIsDuplicateKeyError_Classification pins the error code classification.
// If someone changes the pgconn import from v5 to v4, or changes the error code,
// this test breaks.
func TestIsDuplicateKeyError_Classification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"unique violation 23505", &pgconn.PgError{Code: "23505"}, true},
		{"FK violation 23503 is not duplicate", &pgconn.PgError{Code: "23503"}, false},
		{"syntax error is not duplicate", &pgconn.PgError{Code: "42601"}, false},
		{"generic error", errors.New("generic"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsDuplicateKeyError(tt.err))
		})
	}
}

// TestIsViolatingForeignKeyConstraintError_Classification pins FK error detection.
func TestIsViolatingForeignKeyConstraintError_Classification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"FK violation 23503", &pgconn.PgError{Code: "23503"}, true},
		{"unique violation 23505 is not FK", &pgconn.PgError{Code: "23505"}, false},
		{"generic error", errors.New("generic"), false},
		{"nil error", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isViolatingForeignKeyConstraintError(tt.err))
		})
	}
}

func TestIsForeignKeyError(t *testing.T) {
	// nil error
	err := AsForeignKeyError(nil, "")
	assert.Nil(t, err)

	// common error
	err = AsForeignKeyError(errors.New("common error"), "")
	assert.Nil(t, err)

	// pass
	message := "message"
	err = AsForeignKeyError(&pgconn.PgError{
		Code: "23503",
	}, "%s", message)
	require.NotNil(t, err)
	assert.Equal(t, errors.ViolateForeignKeyConstraintCode, err.Code)
	assert.Equal(t, message, err.Message)
}
