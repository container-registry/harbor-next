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

//go:build tools

// Package tools keeps the go-openapi packages that only the generated swagger
// server imports in the main require block.
//
// src/server/v2.0/{models,restapi} are produced by `task build:gen-apis` and
// gitignored, so any `go mod tidy` run on a plain checkout — Renovate's, or a
// contributor's before they generate — sees no importer for these three and
// demotes them to `// indirect`. The tidy gate then fails against CI, which
// always generates first. The build tag keeps this file out of every binary;
// `go mod tidy` reads it anyway, because it resolves imports as if every build
// tag were set.
package tools

import (
	// Blank imports keep these direct requirements in go.mod; only the
	// generated swagger server imports them for real. See the package comment.
	_ "github.com/go-openapi/loads"
	_ "github.com/go-openapi/spec"
	_ "github.com/go-openapi/validate"
)
