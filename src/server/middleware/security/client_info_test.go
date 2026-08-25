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

package security

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetClientIP(t *testing.T) {
	header := func(v string) http.Header {
		h := http.Header{}
		h.Set("X-Forwarded-For", v)
		return h
	}
	type args struct {
		r *http.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"nil request", args{nil}, ""},
		{"no header", args{&http.Request{RemoteAddr: "10.10.10.10"}}, "10.10.10.10"},
		{"set x forwarded for", args{&http.Request{Header: header("1.1.1.1"), RemoteAddr: "10.10.10.10"}}, "1.1.1.1"},
		{"proxy chain uses first hop", args{&http.Request{Header: header("1.1.1.1, 2.2.2.2, 3.3.3.3"), RemoteAddr: "10.10.10.10"}}, "1.1.1.1"},
		{"first hop has surrounding whitespace", args{&http.Request{Header: header(" 1.1.1.1 , 2.2.2.2"), RemoteAddr: "10.10.10.10"}}, "1.1.1.1"},
		{"first hop is port-qualified", args{&http.Request{Header: header("1.1.1.1:8080, 2.2.2.2"), RemoteAddr: "10.10.10.10"}}, "1.1.1.1"},
		{"ipv6", args{&http.Request{Header: header("::1"), RemoteAddr: "10.10.10.10"}}, "::1"},
		{"ipv6 port-qualified", args{&http.Request{Header: header("[::1]:8080"), RemoteAddr: "10.10.10.10"}}, "::1"},
		{"garbage header falls back to RemoteAddr", args{&http.Request{Header: header("<script>alert(1)</script>"), RemoteAddr: "10.10.10.10"}}, "10.10.10.10"},
		{"oversized RemoteAddr is truncated", args{&http.Request{RemoteAddr: strings.Repeat("9", maxClientIPLen+50)}}, strings.Repeat("9", maxClientIPLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetClientIP(tt.args.r); got != tt.want {
				t.Errorf("GetClientIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetUserAgent(t *testing.T) {
	header := func(v string) http.Header {
		h := http.Header{}
		h.Set("user-agent", v)
		return h
	}
	type args struct {
		r *http.Request
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"nil request", args{nil}, ""},
		{"no header", args{&http.Request{}}, ""},
		{"with user-agent", args{&http.Request{Header: header("docker")}}, "docker"},
		{"oversized user-agent is truncated", args{&http.Request{Header: header(strings.Repeat("a", maxUserAgentLen+50))}}, strings.Repeat("a", maxUserAgentLen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetUserAgent(tt.args.r); got != tt.want {
				t.Errorf("GetUserAgent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	assert.Equal(t, "abc", truncateRunes("abc", 5))
	assert.Equal(t, "abc", truncateRunes("abc", 3))
	assert.Equal(t, "ab", truncateRunes("abc", 2))
	// multi-byte runes must not be split mid-character
	assert.Equal(t, "日本", truncateRunes("日本語", 2))
}

func TestParseIP(t *testing.T) {
	assert.Equal(t, "1.1.1.1", parseIP("1.1.1.1").String())
	assert.Equal(t, "1.1.1.1", parseIP("1.1.1.1:8080").String())
	assert.Equal(t, "::1", parseIP("::1").String())
	assert.Equal(t, "::1", parseIP("[::1]:8080").String())
	assert.Nil(t, parseIP("not-an-ip"))
	assert.Nil(t, parseIP("not-an-ip:8080"))
}
