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

package multiformat

import "context"

// projectIDKey carries the resolved Harbor project id from the multiformatauth
// middleware to the adapters. The project NAME is the leading URL path segment
// (parsed by each adapter); the id is resolved + authorized once upstream.
type projectIDKey struct{}

// WithProjectID stashes the resolved project id on the context. multiformatauth calls
// this after authorizing the request.
func WithProjectID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, projectIDKey{}, id)
}

// ProjectIDFromContext returns the project id stashed by multiformatauth, or 0 if none.
func ProjectIDFromContext(ctx context.Context) int64 {
	if v, ok := ctx.Value(projectIDKey{}).(int64); ok {
		return v
	}
	return 0
}
