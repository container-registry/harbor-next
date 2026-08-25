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

// Package requestinfo threads the source IP address and User-Agent of the
// originating HTTP request through the notification pipeline, mirroring how
// the sibling "operator" package threads the acting username. It exists
// because artifact push/pull/delete events are resolved several layers below
// the HTTP handler (and sometimes asynchronously, e.g. proxy-cache
// background pulls), where *http.Request is no longer available but the
// request-scoped context still is.
package requestinfo

import "context"

// ContextKey is the key for storing request info in the context.
type ContextKey struct{}

// Info carries the source IP address and User-Agent of the request that
// triggered an artifact event.
type Info struct {
	ClientAddress string
	UserAgent     string
}

// NewContext returns a copy of ctx carrying the given client address and
// user agent.
func NewContext(ctx context.Context, clientAddress, userAgent string) context.Context {
	return context.WithValue(ctx, ContextKey{}, Info{
		ClientAddress: clientAddress,
		UserAgent:     userAgent,
	})
}

// FromContext returns the client address and user agent stored on ctx, if
// any. Both are empty when ctx carries no request info, e.g. for events
// fired from background jobs rather than an HTTP request.
func FromContext(ctx context.Context) (clientAddress, userAgent string) {
	info, ok := ctx.Value(ContextKey{}).(Info)
	if !ok {
		return "", ""
	}
	return info.ClientAddress, info.UserAgent
}
