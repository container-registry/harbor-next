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

package registry

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/lib/q"
	"github.com/goharbor/harbor/src/pkg/feature"
	"github.com/goharbor/harbor/src/pkg/reg/model"
	"github.com/goharbor/harbor/src/testing/mock"
	testingreg "github.com/goharbor/harbor/src/testing/pkg/reg"
)

func TestModelSyncGatesHuggingFace(t *testing.T) {
	gate, gated := feature.RegistryTypeFeature(model.RegistryTypeHuggingFace)
	require.True(t, gated)
	assert.Equal(t, feature.ModelSync, gate)

	t.Setenv("HARBOR_ENABLE_MODEL_SYNC", "false")
	assert.False(t, feature.RegistryTypeEnabled(context.Background(), model.RegistryTypeHuggingFace))
	assert.True(t, feature.RegistryTypeEnabled(context.Background(), model.RegistryTypeHarbor))

	t.Setenv("HARBOR_ENABLE_MODEL_SYNC", "true")
	assert.True(t, feature.RegistryTypeEnabled(context.Background(), model.RegistryTypeHuggingFace))
}

func TestModelSyncDisableGuard(t *testing.T) {
	isHuggingFace := mock.MatchedBy(func(query *q.Query) bool {
		return query.Keywords["Type"] == model.RegistryTypeHuggingFace
	})

	regMgr := &testingreg.Manager{}
	regMgr.On("Count", mock.Anything, isHuggingFace).Return(int64(2), nil).Once()
	err := modelSyncDisableGuard{regMgr: regMgr}.Validate(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 Hugging Face registry endpoint(s) exist")

	regMgr = &testingreg.Manager{}
	regMgr.On("Count", mock.Anything, isHuggingFace).Return(int64(0), nil).Once()
	assert.NoError(t, modelSyncDisableGuard{regMgr: regMgr}.Validate(context.Background()))

	regMgr = &testingreg.Manager{}
	regMgr.On("Count", mock.Anything, mock.Anything).Return(int64(0), errors.New("db down")).Once()
	assert.ErrorContains(t, modelSyncDisableGuard{regMgr: regMgr}.Validate(context.Background()), "db down")
}
