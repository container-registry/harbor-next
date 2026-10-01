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
	"testing"
)

var testcases = []struct {
	url         string
	expectedUrl string
	valid       bool
}{
	{"http://harbor.foo.com", "http://harbor.foo.com", true},
	{"http://harbor.foo.com/", "http://harbor.foo.com", true},
	{"http://harbor.foo.com/path", "http://harbor.foo.com/path", true},
	{"/", "", false},
	{"foo.html", "http://foo.html", true},
	{"*", "http://*", true},
	{"http://127.0.0.1/", "http://127.0.0.1", true},
	{"http://127.0.0.1:8080/", "http://127.0.0.1:8080", true},
	{"http://[fe80::1]/", "http://[fe80::1]", true},
	{"http://[fe80::1]:8080/", "http://[fe80::1]:8080", true},

	{"http://[fe80::1%25en0]/", "http://[fe80::1%en0]", true},
	{"http://[fe80::1%25en0]:8080/", "http://[fe80::1%en0]:8080", true},
	{"http://[fe80::1%25%65%6e%301-._~]/", "http://[fe80::1%en01-._~]", true},
	{"http://[fe80::1%25%65%6e%301-._~]:8080/", "http://[fe80::1%en01-._~]:8080", true},

	{"http://127.0.0.%31/", "", false},
	{"http://127.0.0.%31:8080/", "", false},
	{"http://10.0.0.1/test.txt#/api/version", "http://10.0.0.1/test.txt", true},
}

func TestValidateURL(t *testing.T) {
	for _, test := range testcases {
		url, err := ValidateURL(test.url)
		if test.valid {
			if err != nil {
				t.Errorf("ValidateURL:%q gave err %v; want no error", test.url, err)
			}
			if url != test.expectedUrl {
				t.Errorf("ValidateURL:%q gave %s; want %s", test.url, url, test.expectedUrl)
			}
		} else if !test.valid && err == nil {
			t.Errorf("ValidateURL:%q gave <nil> error; want some error", test.url)
		}
	}
}

func TestValidateURLSchemes(t *testing.T) {
	schemeCases := []struct {
		url     string
		schemes []string
		valid   bool
	}{
		{"sftp://harbor.foo.com", nil, true},
		{"s3://bucket/prefix", nil, true},
		{"ftp://harbor.foo.com", nil, true},
		{"gopher://harbor.foo.com", nil, false},
		{"file:///etc/passwd", nil, false},
		{"sftp://harbor.foo.com", []string{"http", "https"}, false},
		{"s3://bucket/prefix", []string{"http", "https"}, false},
		{"http://harbor.foo.com", []string{"http", "https"}, true},
		{"harbor.foo.com", []string{"http", "https"}, true},
	}
	for _, test := range schemeCases {
		_, err := ValidateURL(test.url, test.schemes...)
		if test.valid && err != nil {
			t.Errorf("ValidateURL:%q schemes %v gave err %v; want no error", test.url, test.schemes, err)
		}
		if !test.valid && err == nil {
			t.Errorf("ValidateURL:%q schemes %v gave <nil> error; want some error", test.url, test.schemes)
		}
	}
}
