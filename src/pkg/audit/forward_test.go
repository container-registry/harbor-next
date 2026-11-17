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

package audit

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/lib/config"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
)

func TestDefaultLoggerReusesLoggerForSameEndpoint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		for {
			if _, err := ln.Accept(); err != nil {
				return
			}
		}
	}()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	config.InitWithSettings(map[string]any{common.AuditLogForwardEndpoint: "LOCALHOST:" + port})
	ctx := context.Background()

	mgr := &LoggerManager{}
	mgr.Init(ctx, "localhost:"+port)
	assert.True(t, mgr.initialized)
	first := mgr.remoteLogger

	// an endpoint that differs only in case reuses the existing logger
	assert.Same(t, first, mgr.DefaultLogger(ctx))
	assert.Same(t, first, mgr.DefaultLogger(ctx))
}

func TestInitClearsEndpointOnFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	mgr := &LoggerManager{}
	mgr.Init(context.Background(), addr)
	assert.False(t, mgr.initialized)
	assert.Empty(t, mgr.endpoint)
	assert.NotNil(t, mgr.remoteLogger)
}
