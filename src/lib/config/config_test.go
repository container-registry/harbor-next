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

package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	comModels "github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/config/metadata"
)

// useUnregisteredManager points DefaultCfgManager at a name nothing has
// registered, which is what DefaultMgr sees when CORE_URL is unreachable or
// JOBSERVICE_SECRET is unset, and restores the previous value afterwards.
func useUnregisteredManager(t *testing.T) {
	t.Helper()
	original := DefaultCfgManager
	t.Cleanup(func() { DefaultCfgManager = original })
	DefaultCfgManager = "no-such-config-manager"
}

// TestDefaultMgrReturnsNilWhenUnregistered pins that a misconfigured
// deployment surfaces as a nil manager rather than a half-built one, since
// that nil is what every caller now has to guard against.
func TestDefaultMgrReturnsNilWhenUnregistered(t *testing.T) {
	useUnregisteredManager(t)

	assert.Nil(t, DefaultMgr())
}

// TestLoadWithNilManager guards the nil dereference that segfaulted
// jobservice startup: Load must report the missing manager as an error
// instead of panicking on mgr.Load.
func TestLoadWithNilManager(t *testing.T) {
	useUnregisteredManager(t)

	err := Load(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config manager is not initialized")
}

// TestUploadWithNilManager is the same guard for Upload, which runs against
// the ORM context and would otherwise dereference a nil manager there.
func TestUploadWithNilManager(t *testing.T) {
	useUnregisteredManager(t)

	err := Upload(map[string]any{"some": "value"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "config manager is not initialized")
}

// TestLoadDelegatesToManager checks the non-nil path still reaches the
// registered manager, so the guard above can't be satisfied by Load simply
// refusing to do anything.
func TestLoadDelegatesToManager(t *testing.T) {
	original := DefaultCfgManager
	t.Cleanup(func() { DefaultCfgManager = original })

	mgr := &recordingManager{}
	Register("recording-test-manager", mgr)
	t.Cleanup(func() {
		managersMU.Lock()
		defer managersMU.Unlock()
		delete(managers, "recording-test-manager")
	})
	DefaultCfgManager = "recording-test-manager"

	require.NoError(t, Load(context.Background()))
	assert.Equal(t, 1, mgr.loadCalls, "Load should delegate to the registered manager")
}

// recordingManager is a Manager that only records whether Load was called;
// the remaining methods satisfy the interface and are never reached here.
type recordingManager struct {
	loadCalls int
}

func (m *recordingManager) Load(_ context.Context) error {
	m.loadCalls++
	return nil
}

func (m *recordingManager) Set(_ context.Context, _ string, _ any) {}

func (m *recordingManager) Save(_ context.Context) error { return nil }

func (m *recordingManager) Get(_ context.Context, _ string) *metadata.ConfigureValue {
	return nil
}

func (m *recordingManager) GetItemFromDriver(_ context.Context, _ string) (map[string]any, error) {
	return nil, nil
}

func (m *recordingManager) UpdateConfig(_ context.Context, _ map[string]any) error { return nil }

func (m *recordingManager) GetUserCfgs(_ context.Context) map[string]any { return nil }

func (m *recordingManager) ValidateCfg(_ context.Context, _ map[string]any) error { return nil }

func (m *recordingManager) GetAll(_ context.Context) map[string]any { return nil }

func (m *recordingManager) GetDatabaseCfg() *comModels.Database { return nil }

var _ Manager = &recordingManager{}