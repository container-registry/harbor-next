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

package requestinfo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewContext_FromContext_RoundTrip(t *testing.T) {
	ctx := NewContext(context.Background(), "203.0.113.10", "docker/24.0.0")
	ip, ua := FromContext(ctx)
	assert.Equal(t, "203.0.113.10", ip)
	assert.Equal(t, "docker/24.0.0", ua)
}

func TestFromContext_NoValue(t *testing.T) {
	ip, ua := FromContext(context.Background())
	assert.Empty(t, ip)
	assert.Empty(t, ua)
}
