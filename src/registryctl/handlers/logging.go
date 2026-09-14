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

package handlers

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/felixge/httpsnoop"
)

// clfTimeLayout is the timestamp format of the Apache Common Log Format.
const clfTimeLayout = "02/Jan/2006:15:04:05 -0700"

// loggingHandler writes one Apache Common Log Format line per request to out.
func loggingHandler(out io.Writer, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Snapshot the URL, the handlers below are free to rewrite r.URL.
		reqURL := *r.URL

		m := httpsnoop.CaptureMetrics(next, w, r)

		_, _ = out.Write(commonLogLine(r, reqURL, start, m.Code, m.Written))
	})
}

// commonLogLine renders one Common Log Format entry, terminated by a newline.
// The ident field is always "-".
func commonLogLine(r *http.Request, reqURL url.URL, ts time.Time, status int, size int64) []byte {
	username := "-"
	if reqURL.User != nil {
		if name := reqURL.User.Username(); name != "" {
			username = name
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	uri := r.RequestURI
	// HTTP/2 CONNECT names its target in the authority field, not in the URI.
	if r.ProtoMajor == 2 && r.Method == http.MethodConnect {
		uri = r.Host
	}
	if uri == "" {
		uri = reqURL.RequestURI()
	}

	var buf []byte
	buf = append(buf, host...)
	buf = append(buf, " - "...)
	buf = append(buf, username...)
	buf = append(buf, " ["...)
	buf = ts.AppendFormat(buf, clfTimeLayout)
	buf = append(buf, `] "`...)
	buf = append(buf, r.Method...)
	buf = append(buf, ' ')
	buf = append(buf, escapeQuoted(uri)...)
	buf = append(buf, ' ')
	buf = append(buf, r.Proto...)
	buf = append(buf, `" `...)
	buf = strconv.AppendInt(buf, int64(status), 10)
	buf = append(buf, ' ')
	buf = strconv.AppendInt(buf, size, 10)
	return append(buf, '\n')
}

// escapeQuoted escapes s as a Go quoted string would, minus the surrounding
// quotes, so that a request URI cannot break out of the quoted request field.
//
// This renders DEL (U+007F) as \x7f where the Apache convention writes \u007f.
// net/http answers 400 to a request line carrying a raw control byte, so a
// handler never observes one and the difference cannot show up in a real log.
func escapeQuoted(s string) string {
	quoted := strconv.Quote(s)
	return quoted[1 : len(quoted)-1]
}
