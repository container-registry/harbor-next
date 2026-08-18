//go:build e2e

// Package steps holds every godog step definition implementing the contract
// listed in SPEC.md §8. Each file groups steps by theme:
//
//	common_steps.go        — system, projects, users, RBAC, audit
//	registry_steps.go      — push/pull, multi-arch, GC, quota, immutability
//	signing_steps.go       — cosign sign/verify/attest
//	replication_steps.go   — registry endpoints and replication policies
//	scan_steps.go          — trivy scan lifecycle
//	webhook_steps.go       — in-process listener and webhook policies
//	branding_steps.go      — patch 0001: /api/v2.0/systeminfo/branding
//	package_steps.go       — native npm and Maven package formats (multiformat)
//	package_access_steps.go — access control over those package formats
//	pgx_monitoring_steps.go — patch 0004: pgx DB metrics on core/jobservice
//
// All step files contribute to a single Register entrypoint called by hooks.go
// so duplicate regexes are surfaced at registration time (SPEC §8.4).
package steps

import (
	"github.com/cucumber/godog"
)

// Register wires every step definition into a single ScenarioContext.
func Register(sc *godog.ScenarioContext) {
	registerCommon(sc)
	registerRegistry(sc)
	registerSigning(sc)
	registerReplication(sc)
	registerScan(sc)
	registerWebhook(sc)
	registerIdp(sc)
	registerCommercial(sc)
	registerBranding(sc)
	registerPgxMonitoring(sc)
	registerNextUpdateRobotAccount(sc)
	registerPackages(sc)
	registerPackageAccess(sc)
}
