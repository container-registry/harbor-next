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

package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/goharbor/harbor/src/pkg/p2p/preheat/models/provider"
	"github.com/goharbor/harbor/src/pkg/p2p/preheat/provider/auth"
)

// TestCheckProgressTaskTable pins the task table CheckProgress renders into
// PreheatingStatus.Message. The shared mock returns a success job with no
// tasks, so neither the rows nor the frame are covered there.
func TestCheckProgressTaskTable(t *testing.T) {
	body := `{
		"id": 7,
		"state": "SUCCESS",
		"created_at": "2026-09-24T00:00:00Z",
		"updated_at": "2026-09-24T00:01:00Z",
		"result": {
			"job_states": [
				{
					"error": "",
					"results": [
						{
							"scheduler_cluster_id": 3,
							"success_tasks": [
								{"url": "http://harbor/v2/library/busybox/blobs/sha256:aaa", "hostname": "peer-1", "ip": "10.0.0.1"}
							],
							"failure_tasks": [
								{"url": "http://harbor/v2/library/busybox/blobs/sha256:bbb", "hostname": "peer-2", "ip": "10.0.0.2", "description": "connection refused"}
							]
						}
					]
				}
			]
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := &dragonflyJobResponse{}
		require.NoError(t, json.Unmarshal([]byte(body), resp))
		resp.CreatedAt = time.Now()
		resp.UpdatedAt = time.Now()

		payload, err := json.Marshal(resp)
		require.NoError(t, err)
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	driver := &DragonflyDriver{
		instance: &provider.Instance{
			ID:       1,
			Name:     "test-instance",
			Vendor:   DriverDragonfly,
			Endpoint: srv.URL,
			AuthMode: auth.AuthModeNone,
			Enabled:  true,
			Insecure: true,
			Status:   DriverStatusHealthy,
		},
	}

	st, err := driver.CheckProgress("7")
	require.NoError(t, err)
	require.Equal(t, provider.PreheatingStatusSuccess, st.Status)

	// The +-| frame is tablewriter's ASCII style, not the Unicode one v1
	// renders by default. Cluster ID is left-aligned: v0.0.5 right-aligned
	// numeric cells and v1 has no equivalent.
	expected := "" +
		"+---------------------------------------------------+----------+----------+------------+---------+--------------------+\n" +
		"|                     BLOB URL                      | HOSTNAME |    IP    | CLUSTER ID |  STATE  |   ERROR MESSAGE    |\n" +
		"+---------------------------------------------------+----------+----------+------------+---------+--------------------+\n" +
		"| http://harbor/v2/library/busybox/blobs/sha256:aaa | peer-1   | 10.0.0.1 | 3          | SUCCESS |                    |\n" +
		"| http://harbor/v2/library/busybox/blobs/sha256:bbb | peer-2   | 10.0.0.2 | 3          | FAILURE | connection refused |\n" +
		"+---------------------------------------------------+----------+----------+------------+---------+--------------------+\n"

	assert.Equal(t, expected, st.Message)
}
