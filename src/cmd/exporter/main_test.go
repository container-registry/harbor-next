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

package main

import (
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getMinConns must distinguish an unset knob from an explicit 0, which is a
// valid pgxpool setting (no warm floor). See harbor-next#564.
func TestGetMinConns(t *testing.T) {
	tests := []struct {
		name string
		env  string // "" means the variable is not set at all
		set  bool
		want *int32
	}{
		{name: "unset falls back to the dbpool default", set: false, want: nil},
		{name: "empty reads as unset", set: true, env: "", want: nil},
		{name: "explicit zero is honoured", set: true, env: "0", want: ptr(0)},
		{name: "explicit value is honoured", set: true, env: "5", want: ptr(5)},
		// A bare int32() would wrap 1<<32 to an explicit 0 — the opposite of
		// the configured intent. Out-of-range reads as unset instead.
		{name: "value above int32 range reads as unset", set: true, env: "4294967296", want: nil},
		{name: "value below int32 range reads as unset", set: true, env: "-4294967296", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			viper.SetEnvPrefix("harbor")
			viper.AutomaticEnv()
			viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

			// t.Setenv registers restoration of the pre-test value; the unset
			// case then removes the variable so ambient CI env cannot leak in.
			t.Setenv("HARBOR_DATABASE_MIN_CONNS", tt.env)
			if !tt.set {
				require.NoError(t, os.Unsetenv("HARBOR_DATABASE_MIN_CONNS"))
			}

			got := getMinConns()
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *tt.want, *got)
		})
	}
}

func ptr(v int32) *int32 { return &v }
