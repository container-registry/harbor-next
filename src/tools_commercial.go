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

// Package tools also keeps the libraries that only the commercial patch
// branches import in the main require block.
//
// The patch branches carry business logic; the libraries it needs belong here,
// so no patch branch has to edit src/go.mod and every octopus of them merges
// clean. Without an importer on plain main, `go mod tidy` demotes or drops
// these, the branch that needs one has to add it back, and that edit is the
// only thing the release octopus ever conflicts on. The build tag keeps the
// file out of every binary; `go mod tidy` reads it anyway, because it resolves
// imports as if every build tag were set. Same mechanism as tools.go.
//
// Each import names the branch that needs it. Drop an entry when its last
// importer goes away.
package tools

import (
	// 0003-sftp-replication
	_ "github.com/aws/aws-sdk-go/service/s3"
	_ "github.com/pkg/sftp"
	_ "golang.org/x/crypto/ssh"

	// 0004-federated-robot-accounts
	_ "github.com/davecgh/go-spew/spew"
	_ "github.com/lestrrat-go/jwx/v3/jwa"
	_ "github.com/lestrrat-go/jwx/v3/jwk"
	_ "github.com/lestrrat-go/jwx/v3/jws"
	_ "github.com/lestrrat-go/jwx/v3/jwt"

	// 0006-aws-rds-iam-auth
	_ "github.com/aws/aws-sdk-go-v2/feature/rds/auth"

	// 0005-pgx-monitoring, 0006-aws-rds-iam-auth
	_ "go.opentelemetry.io/otel/exporters/prometheus"
	_ "go.opentelemetry.io/otel/metric"
	_ "go.opentelemetry.io/otel/sdk/metric"

	// 0007-multi-format-artifacts
	_ "github.com/cucumber/godog"
	_ "golang.org/x/mod/module"
	_ "golang.org/x/mod/sumdb"

	// 0008-audit-log-otel
	_ "github.com/prometheus/client_golang/prometheus/testutil"
	_ "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	_ "go.opentelemetry.io/otel/log"
	_ "go.opentelemetry.io/otel/sdk/log"
	_ "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	_ "google.golang.org/protobuf/proto"
)
