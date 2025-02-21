package main

import (
	"context"
	"dagger/harbor/internal/dagger"
)

// registryctl: Registry controller for interacting with the registry.
// core: The business logic service that relies on the database, registry, and Redis.
// portal: The user-facing portal can be started after core.
// jobservice: Background jobs, which require the core service to be available.
// proxy: The last service to start as it routes traffic to all the other services.

func (m *Harbor) DbService() *dagger.Service {
	postgres := dag.Container().From("goharbor/harbor-db:dev").
		WithExposedPort(5432).
		WithEnvVariable("POSTGRES_PASSWORD", "root123").
		AsService()
	return postgres
}

