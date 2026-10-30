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

//go:build !db

package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
)

// A secret that cannot be encrypted used to be stored in plaintext and then
// dropped on load, while the API answered 200.
func TestSaveRefusesASecretItCannotEncrypt(t *testing.T) {
	t.Setenv("KEY_PATH", filepath.Join(t.TempDir(), "missing-key"))

	d := &Database{}
	err := d.Save(context.Background(), map[string]any{common.OIDCClientSecret: "s3cret"})
	require.ErrorContains(t, err, common.OIDCClientSecret)
}
