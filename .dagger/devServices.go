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

func (m *Harbor) RegistryCtlService(ctx context.Context) *dagger.Service {
	regConfigDir := m.Source.Directory(".dagger/config/registry")
	regCtlConfig := m.Source.File(".dagger/config/registryctl/config.yml")

	regCtl := m.BuildImage(ctx, DEV_PLATFORM, "registryctl", "v3.0").
		WithMountedDirectory("/etc/registry", regConfigDir).
		WithMountedFile("/etc/registryctl/config.yml", regCtlConfig).
		// - ./common/config/registryctl/env envs are similar to this
		WithEnvVariable("JOBSERVICE_SECRET", "Harbor12345").
		WithEnvVariable("CORE_SECRET", "Harbor12345").
		WithServiceBinding("redis", m.RedisService()).
		WithServiceBinding("registry", m.RedisService()).
		AsService()
	return regCtl
}

func (m *Harbor) DbService() *dagger.Service {
	postgres := dag.Container().From("goharbor/harbor-db:dev").
		WithExposedPort(5432).
		WithEnvVariable("POSTGRES_PASSWORD", "root123").
		AsService()
	return postgres
}

func (m *Harbor) RedisService() *dagger.Service {
	return dag.Container().
		From("goharbor/redis-photon:dev").
		WithExposedPort(6379).
		AsService()
}

func (m *Harbor) RegistryService(ctx context.Context) *dagger.Service {
	regConfigDir := m.Source.Directory(".dagger/config/registry")

	// 5001 is can be used to debug according to config
	reg := m.buildRegistry(ctx, DEV_PLATFORM).
		WithMountedDirectory("/etc/registry", regConfigDir).
		WithServiceBinding("redis", m.RedisService()).
		WithExposedPort(5000).
		// WithExposedPort(5001).
		AsService()
	return reg
}
