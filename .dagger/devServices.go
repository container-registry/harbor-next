package main

import (
	"context"
	"dagger/harbor/internal/dagger"
)

func (m *Harbor) NginxService(ctx context.Context) *dagger.Service {
	nginxConfig := m.Source.File(".dagger/config/proxy/nginx.conf")

	nginxSrv := m.BuildImage(ctx, DEV_PLATFORM, "nginx", DEV_VERSION).
		// nginxSrv := dag.Container().From("goharbor/nginx-photon:dev").
		WithMountedFile("/etc/nginx/nginx.conf", nginxConfig).
		WithExposedPort(8080).
		WithoutExposedPort(8443).
		AsService()
	return nginxSrv
}

// not working as expected
func (m *Harbor) PortalService(ctx context.Context) *dagger.Service {
	nginxConfig := m.Source.File(".dagger/config/proxy/nginx.conf")

	portal := m.BuildImage(ctx, DEV_PLATFORM, "portal", DEV_VERSION).
		// portal := dag.Container().From("goharbor/harbor-portal:dev").
		WithMountedFile("/etc/nginx/nginx.conf", nginxConfig).
		WithExposedPort(8080).
		WithoutExposedPort(8443).
		AsService()
	return portal
}

func (m *Harbor) JobService(ctx context.Context) *dagger.Service {
	jobSrvConfig := m.Source.File(".dagger/config/jobservice/config.yml")
	envFile := m.Source.File(".dagger/config/jobservice/env")
	run_script := m.Source.File(".dagger/config/run_env.sh")

	jobSrv := m.BuildImage(ctx, DEV_PLATFORM, "jobservice", DEV_VERSION).
		WithMountedFile("/etc/jobservice/config.yml", jobSrvConfig).
		WithMountedDirectory("/var/log/jobs", m.Source.Directory(".dagger/config/jobservice")).
		WithMountedFile("/envFile", envFile).
		WithMountedFile("/run_script", run_script).
		WithExposedPort(8080).
		WithEntrypoint([]string{"/run_script", "/jobservice -c /etc/jobservice/config.yml"}).
		AsService()
	return jobSrv
}

func (m *Harbor) coreService(ctx context.Context) *dagger.Service {
	coreConfig := m.Source.File(".dagger/config/core/app.conf")
	envFile := m.Source.File(".dagger/config/core/env")
	run_script := m.Source.File(".dagger/config/run_env.sh")

	core := m.BuildImage(ctx, DEV_PLATFORM, "core", DEV_VERSION).
		WithMountedFile("/etc/core/app.conf", coreConfig).
		WithMountedFile("/envFile", envFile).
		WithMountedFile("/run_script", run_script).
		WithExposedPort(8080).
		// WithExposedPort(80).
		WithEntrypoint([]string{"/run_script", "/core"}).
		AsService()

	return core
}

func (m *Harbor) RegistryCtlService(ctx context.Context) *dagger.Service {
	regConfigDir := m.Source.Directory(".dagger/config/registry")
	regCtlConfig := m.Source.File(".dagger/config/registryctl/config.yml")
	envFile := m.Source.File(".dagger/config/jobservice/env")
	run_script := m.Source.File(".dagger/config/run_env.sh")

	regCtl := m.BuildImage(ctx, DEV_PLATFORM, "registryctl", DEV_VERSION).
		WithMountedDirectory("/etc/registry", regConfigDir).
		WithMountedFile("/etc/registryctl/config.yml", regCtlConfig).
		WithMountedFile("/envFile", envFile).
		WithMountedFile("/run_script", run_script).
    WithEntrypoint([]string{"/run_script", "/registryctl -c /etc/registryctl/config.yml"}).
		AsService()

	return regCtl
}

func (m *Harbor) PostgresService(ctx context.Context) *dagger.Service {
	postgres := dag.Container().From("goharbor/harbor-db:dev").
		WithExposedPort(5432).
		WithEnvVariable("POSTGRES_PASSWORD", "root123").
		AsService()
	return postgres
}

func (m *Harbor) RedisService(ctx context.Context) *dagger.Service {
	return dag.Container().
		From("goharbor/redis-photon:dev").
		WithExposedPort(6379).
		AsService()
}

func (m *Harbor) RegistryService(ctx context.Context) *dagger.Service {
	regConfigDir := m.Source.Directory(".dagger/config/registry")

	// 5001 is can be used to debug according to config
	reg := m.BuildImage(ctx, DEV_PLATFORM, "registry", DEV_VERSION).
		WithMountedDirectory("/etc/registry", regConfigDir).
		WithExposedPort(5000).
		WithoutExposedPort(5001).
		WithoutExposedPort(5443).
		AsService()
	return reg
}
