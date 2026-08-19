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

package dao

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/rds/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/lib/dbpool"
	"github.com/goharbor/harbor/src/lib/log"
)

// iamAuthMaxConnLifetime caps connection lifetime below the 15-minute
// IAM token expiry so connections are recycled with fresh tokens.
const iamAuthMaxConnLifetime = 14 * time.Minute

// iamAuthOption returns a dbpool.Option that configures a BeforeConnect hook
// to generate a fresh AWS RDS IAM auth token for each new connection. It also
// caps MaxConnLifetime to 14 minutes (tokens expire at 15).
//
// AWS config is loaded eagerly so misconfiguration is caught at startup.
func iamAuthOption(ctx context.Context, cfg *models.PostGreSQL) (dbpool.Option, error) {
	region := resolveAWSRegion(cfg.AWSRegion)
	if region == "" {
		return nil, fmt.Errorf("IAM auth: AWS region is required (set POSTGRESQL_AWS_REGION or AWS_REGION)")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("IAM auth: load AWS config: %w", err)
	}

	endpoint := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	username := cfg.Username

	return func(poolCfg *pgxpool.Config) {
		if poolCfg.MaxConnLifetime == 0 || poolCfg.MaxConnLifetime > iamAuthMaxConnLifetime {
			poolCfg.MaxConnLifetime = iamAuthMaxConnLifetime
		}

		poolCfg.BeforeConnect = func(ctx context.Context, connCfg *pgx.ConnConfig) error {
			token, err := auth.BuildAuthToken(ctx, endpoint, region, username, awsCfg.Credentials)
			if err != nil {
				return fmt.Errorf("IAM auth: build token: %w", err)
			}
			connCfg.Password = token
			log.Debugf("IAM Auth: Token refreshed for %s@%s", username, endpoint)
			return nil
		}

		log.Infof("IAM Auth: Enabled for region=%s endpoint=%s user=%s", region, endpoint, username)
	}, nil
}

// iamMigrationToken generates a single IAM token for golang-migrate,
// which opens its own connection outside of pgxpool.
func iamMigrationToken(ctx context.Context, cfg *models.PostGreSQL) (string, error) {
	region := resolveAWSRegion(cfg.AWSRegion)
	if region == "" {
		return "", fmt.Errorf("IAM auth: AWS region is required for migration")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return "", fmt.Errorf("IAM auth: load AWS config for migration: %w", err)
	}

	endpoint := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	token, err := auth.BuildAuthToken(ctx, endpoint, region, cfg.Username, awsCfg.Credentials)
	if err != nil {
		return "", fmt.Errorf("IAM auth: build migration token: %w", err)
	}

	log.Infof("IAM Auth: Token generated for database migration")
	return token, nil
}

// resolveAWSRegion returns the configured region, falling back to standard
// AWS environment variables if not explicitly set.
func resolveAWSRegion(configured string) string {
	if configured != "" {
		return configured
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	return os.Getenv("AWS_DEFAULT_REGION")
}
