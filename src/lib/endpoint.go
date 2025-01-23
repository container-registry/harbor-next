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

package lib

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/goharbor/harbor/src/lib/errors"
)

// ValidateHTTPURL checks whether the provided string is a valid HTTP URL.
// If it is, return the URL in format "scheme://host:port" to avoid the SSRF
func ValidateURL(s string, requiredSchemas ...string) (string, error) {
	s = strings.Trim(s, " ")
	s = strings.TrimRight(s, "/")
	if len(s) == 0 {
		return "", errors.New(nil).WithCode(errors.BadRequestCode).WithMessage("empty string")
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	url, err := url.Parse(s)
	if err != nil {
		return "", errors.New(nil).WithCode(errors.BadRequestCode).WithMessagef("invalid URL: %s", err.Error())
	}
	if len(requiredSchemas) == 0 {
		requiredSchemas = []string{"http", "https", "s3", "sftp", "ftp"}
	}
	var found bool
	for _, schema := range requiredSchemas {
		if url.Scheme != schema {
			continue
		}
		found = true
	}
	if !found {
		return "", errors.New(nil).WithCode(errors.BadRequestCode).WithMessage("invalid scheme: %s", url.Scheme)
	}
	// To avoid SSRF security issue, refer to #3755 for more detail
	return fmt.Sprintf("%s://%s%s", url.Scheme, url.Host, url.Path), nil
}
