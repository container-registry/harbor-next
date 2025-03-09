package main

import (
	"context"
	"dagger/harbor/internal/dagger"
	"fmt"
	"log"
	"strings"
)

// to-do: update registry to v3
// to-do: stop usage of shell things. No shell spawning

type (
	Package  string
	Platform string
)

var (
	// targetPlatforms = []Platform{"linux/amd64", "linux/arm64"}
	targetPlatforms = []Platform{"linux/amd64"}
	packages        = []Package{"core", "jobservice", "registryctl", "portal", "registry", "nginx", "cmd/exporter", "trivy-adapter"}
	// packages = []string{"core", "jobservice"}
)

type BuildMetadata struct {
	Package    Package
	BinaryPath string
	Container  *dagger.Container
	Platform   Platform
}

func New(
	// +optional
	// +defaultPath="./"
	// +ignore=["bin", "node_modules"]
	source *dagger.Directory,
	// +optional
	// +defaultPath="./"
	// +ignore=[".dagger", "node_modules", ".github", "contrib", "docs", "icons", "tests", "make", "bin", "*.md"]
	filteredSrc *dagger.Directory,
	// +optional
	// +defaultPath="./"
	// +ignore=["*", "!.dagger"]
	onlyDagger *dagger.Directory,
) *Harbor {
	return &Harbor{Source: source, FilteredSrc: filteredSrc, OnlyDagger: onlyDagger}
}

type Harbor struct {
	Source      *dagger.Directory
	FilteredSrc *dagger.Directory
	OnlyDagger  *dagger.Directory
}

// build, publish and sign all images
func (m *Harbor) PublishAndSignAllImages(
	ctx context.Context,
	registry string,
	registryUsername string,
	registryPassword *dagger.Secret,
	imageTags []string,
	// +optional
	githubToken *dagger.Secret,
	// +optional
	actionsIdTokenRequestToken *dagger.Secret,
	// +optional
	actionsIdTokenRequestUrl string,
) (string, error) {
	imageAddrs := m.PublishAllImages(ctx, registry, registryUsername, imageTags, registryPassword)
	_, err := m.Sign(
		ctx,
		githubToken,
		actionsIdTokenRequestUrl,
		actionsIdTokenRequestToken,
		registryUsername,
		registryPassword,
		imageAddrs[0],
	)
	if err != nil {
		return "", fmt.Errorf("failed to sign image: %w", err)
	}

	fmt.Printf("Signed image: %s\n", imageAddrs)
	return imageAddrs[0], nil
}

// Sign signs a container image using Cosign, works also with GitHub Actions
func (m *Harbor) Sign(ctx context.Context,
	// +optional
	githubToken *dagger.Secret,
	// +optional
	actionsIdTokenRequestUrl string,
	// +optional
	actionsIdTokenRequestToken *dagger.Secret,
	registryUsername string,
	registryPassword *dagger.Secret,
	imageAddr string,
) (string, error) {
	registryPasswordPlain, _ := registryPassword.Plaintext(ctx)

	cosing_ctr := dag.Container().From("cgr.dev/chainguard/cosign")

	// If githubToken is provided, use it to sign the image. (GitHub Actions) use case
	if githubToken != nil {
		if actionsIdTokenRequestUrl == "" || actionsIdTokenRequestToken == nil {
			return "", fmt.Errorf("actionsIdTokenRequestUrl (exist=%s) and actionsIdTokenRequestToken (exist=%t) must be provided when githubToken is provided", actionsIdTokenRequestUrl, actionsIdTokenRequestToken != nil)
		}
		fmt.Printf("Setting the ENV Vars GITHUB_TOKEN, ACTIONS_ID_TOKEN_REQUEST_URL, ACTIONS_ID_TOKEN_REQUEST_TOKEN to sign with GitHub Token")
		cosing_ctr = cosing_ctr.WithSecretVariable("GITHUB_TOKEN", githubToken).
			WithEnvVariable("ACTIONS_ID_TOKEN_REQUEST_URL", actionsIdTokenRequestUrl).
			WithSecretVariable("ACTIONS_ID_TOKEN_REQUEST_TOKEN", actionsIdTokenRequestToken)
	}

	return cosing_ctr.WithSecretVariable("REGISTRY_PASSWORD", registryPassword).
		WithExec([]string{"cosign", "env"}).
		WithExec([]string{
			"cosign", "sign", "--yes", "--recursive",
			"--registry-username", registryUsername,
			"--registry-password", registryPasswordPlain,
			imageAddr,
			"--timeout", "1m",
		}).Stdout(ctx)
}

// Publishes All Images and variants
func (m *Harbor) PublishAllImages(
	ctx context.Context,
	registry, registryUsername string,
	imageTags []string,
	registryPassword *dagger.Secret,
) []string {
	fmt.Printf("provided tags: %s\n", imageTags)

	allImages := m.buildAllImages(ctx)
	platformVariantsContainer := make(map[Package][]*dagger.Container)
	for _, meta := range allImages {
		platformVariantsContainer[meta.Package] = append(platformVariantsContainer[meta.Package], meta.Container)
	}

	var imageAddresses []string
	for pkg, imgs := range platformVariantsContainer {
		for _, imageTag := range imageTags {
			container := dag.Container().WithRegistryAuth(registry, registryUsername, registryPassword)
			imgAddress, err := container.Publish(ctx,
				fmt.Sprintf("%s/%s/%s:%s", registry, "harbor-next", pkg, imageTag),
				dagger.ContainerPublishOpts{PlatformVariants: imgs},
			)
			if err != nil {
				fmt.Printf("Failed to publish image: %s/%s/%s:%s\n", registry, "harbor-next", pkg, imageTag)
				fmt.Printf("Error: %s\n", err)
				continue
			}
			imageAddresses = append(imageAddresses, imgAddress)
			fmt.Printf("Published image: %s\n", imgAddress)
		}
	}
	return imageAddresses
}

// publishes the specific image with the given tag
func (m *Harbor) PublishImage(
	ctx context.Context,
	registry, registryUsername string,
	imageTags []string,
	pkg Package,
	registryPassword *dagger.Secret,
) []string {
	var (
		imageAddresses []string
		images         []*dagger.Container
	)

	fmt.Printf("provided tags: %s\n", imageTags)

	for _, platform := range targetPlatforms {
		BuildImage := m.BuildImage(ctx, platform, pkg)
		images = append(images, BuildImage)
	}

	platformVariantsContainer := make(map[Package][]*dagger.Container)
	for _, image := range images {
		platformVariantsContainer[pkg] = append(platformVariantsContainer[pkg], image)
	}

	for pkg, imgs := range platformVariantsContainer {
		for _, imageTag := range imageTags {
			container := dag.Container().WithRegistryAuth(registry, registryUsername, registryPassword)
			imgAddress, err := container.Publish(ctx,
				fmt.Sprintf("%s/%s/%s:%s", registry, "harbor-next", pkg, imageTag),
				dagger.ContainerPublishOpts{PlatformVariants: imgs},
			)
			if err != nil {
				fmt.Printf("Failed to publish image: %s/%s/%s:%s\n", registry, "harbor-next", pkg, imageTag)
				fmt.Printf("Error: %s\n", err)
				continue
			}
			imageAddresses = append(imageAddresses, imgAddress)
			fmt.Printf("Published image: %s\n", imgAddress)
		}
	}

	return imageAddresses
}

// export all images as Tarball
func (m *Harbor) ExportAllImages(ctx context.Context) *dagger.Directory {
	metdata := m.buildAllImages(ctx)
	artifacts := dag.Directory()
	for _, meta := range metdata {
		artifacts = artifacts.WithFile(fmt.Sprintf("containers/%s/%s.tgz", meta.Platform, meta.Package), meta.Container.AsTarball())
	}
	return artifacts
}

// build all images
func (m *Harbor) BuildAllImages(ctx context.Context) []*dagger.Container {
	metdata := m.buildAllImages(ctx)
	images := make([]*dagger.Container, len(metdata))
	for i, meta := range metdata {
		images[i] = meta.Container
	}
	return images
}

func (m *Harbor) buildAllImages(ctx context.Context) []*BuildMetadata {
	var buildMetadata []*BuildMetadata
	for _, platform := range targetPlatforms {
		for _, pkg := range packages {
			img := m.BuildImage(ctx, platform, pkg)
			buildMetadata = append(buildMetadata, &BuildMetadata{
				Package:    pkg,
				BinaryPath: fmt.Sprintf("bin/%s/%s", platform, pkg),
				Container:  img,
				Platform:   platform,
			})
		}
	}
	return buildMetadata
}

// build single specified images
func (m *Harbor) BuildImage(ctx context.Context, platform Platform, pkg Package) *dagger.Container {
	buildMtd := m.buildImage(ctx, platform, pkg)
	if pkg == "core" {
		// the only thing missing here is the healthcheck
		// we can add those by updating the docker compose since dagger currently doesn't support healthchecks
		// issue: https://github.com/dagger/dagger/issues/9515
		buildMtd.Container = buildMtd.Container.WithDirectory("/migrations", m.Source.Directory("make/migrations")).
			WithDirectory("/icons", m.Source.Directory("icons")).
			WithDirectory("/views", m.Source.Directory("src/core/views"))
	}
	if pkg == "registryctl" {
		regBinary := m.registryBuilder(ctx)
		buildMtd.Container = buildMtd.Container.WithFile("/usr/bin/registry_DO_NOT_USE_GC", regBinary).
			WithExposedPort(8080)
	}

	return buildMtd.Container
}

// internal function to build registry
func (m *Harbor) registryBuilder(ctx context.Context) *dagger.File {
	registry := dag.Container().From("golang:1.22.3").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod-"+GO_VERSION)).
		WithMountedCache("/go/build-cache", dag.CacheVolume("go-build-"+GO_VERSION)).
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithEnvVariable("GOCACHE", "/go/build-cache").
		WithEnvVariable("DISTRIBUTION_DIR", "/go/src/github.com/docker/distribution").
		WithEnvVariable("BUILDTAGS", "include_oss include_gcs").
		WithEnvVariable("GO111MODULE", "auto").
		WithEnvVariable("CGO_ENABLED", "0").
		WithWorkdir("/go/src/github.com/docker").
		WithExec([]string{"git", "clone", "-b", REGISTRY_SRC_TAG, DISTRIBUTION_SRC}).
		WithWorkdir("distribution").
		WithFile("/redis.patch", m.OnlyDagger.File("./.dagger/registry/redis.patch")).
		WithExec([]string{"git", "apply", "/redis.patch"}).
		WithExec([]string{"echo", "build the registry binary"})

	registryBinary := registry.
		// to-do: check possible ways to remove make clean bin/registry
		WithExec([]string{"make", "clean", "bin/registry"}).
		File("bin/registry")

	return registryBinary
}

func (m *Harbor) buildImage(ctx context.Context, platform Platform, pkg Package) *BuildMetadata {
	var (
		buildMtd *BuildMetadata
		img      *dagger.Container
	)

	if pkg == "trivy-adapter" {
		img = m.buildTrivyAdapter(ctx, platform)
		buildMtd = &BuildMetadata{
			Package:    pkg,
			BinaryPath: "nil",
			Container:  img,
			Platform:   platform,
		}
	} else if pkg == "portal" {
		img = m.buildPortal(ctx, platform)
		buildMtd = &BuildMetadata{
			Package:    pkg,
			BinaryPath: "nil",
			Container:  img,
			Platform:   platform,
		}
	} else if pkg == "registry" {
		img = m.buildRegistry(ctx, platform)
		buildMtd = &BuildMetadata{
			Package:    pkg,
			BinaryPath: "nil",
			Container:  img,
			Platform:   platform,
		}
	} else if pkg == "nginx" {
		img = m.buildNginx(ctx, platform)
		buildMtd = &BuildMetadata{
			Package:    pkg,
			BinaryPath: "nil",
			Container:  img,
			Platform:   platform,
		}
	} else {
		buildMtd = m.buildBinary(ctx, platform, pkg)
		img = dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).From("alpine:latest").
			WithFile("/"+string(pkg), buildMtd.Container.File(buildMtd.BinaryPath))

		// // Set entrypoint
		// if pkg == "jobservice" {
		// 	img = img.WithEntrypoint([]string{"/" + string(pkg), "-c", "/etc/jobservice/config.yml"})
		// } else if pkg == "registryctl" {
		// 	img = img.WithEntrypoint([]string{"/" + string(pkg), "-c", "/etc/registryctl/config.yml"})
		// }

		// Set entrypoint based on package
		entrypoint := []string{"/" + string(pkg)}
		if pkg == "jobservice" {
			entrypoint = append(entrypoint, "-c", "/etc/jobservice/config.yml")
		} else if pkg == "registryctl" {
			entrypoint = append(entrypoint, "-c", "/etc/registryctl/config.yml")
		}

		if DEBUG {
			img = img.
        WithExec([]string{"apk", "add", "delve=1.23.1-r2"}).
				WithExposedPort(8080).
				WithExposedPort(4001, dagger.ContainerWithExposedPortOpts{ExperimentalSkipHealthcheck: true}).
				// WithEntrypoint([]string{"/" + string(pkg)}).
				// /root/go/bin/dlv --headless=true --listen=localhost:4001 --accept-multiclient --log-output=debugger,debuglineerr,gdbwire,lldbout,rpc --log=true --continue --api-version=2 exec $pkg
				WithEntrypoint(append([]string{
					"/root/go/bin/dlv",
					"--headless=true",
					"--listen=0.0.0.0:" + DEBUG_PORT,
					"--accept-multiclient",
					"--log-output=debugger,debuglineerr,gdbwire,lldbout,rpc",
					"--log=true",
					"--continue",
					"--api-version=2",
					"exec",
				}, entrypoint...))
		} else {
			img = img.WithEntrypoint(entrypoint)
		}
	}

	buildMtd.Container = img
	return buildMtd
}

// build all binaries and return directory containing all binaries
func (m *Harbor) BuildAllBinaries(ctx context.Context) *dagger.Directory {
	output := dag.Directory()
	builds := m.buildAllBinaries(ctx)
	for _, build := range builds {
		output = output.WithFile(build.BinaryPath, build.Container.File(build.BinaryPath))
	}
	return output
}

func (m *Harbor) buildAllBinaries(ctx context.Context) []*BuildMetadata {
	var buildContainers []*BuildMetadata
	for _, platform := range targetPlatforms {
		for _, pkg := range packages {
			buildContainer := m.buildBinary(ctx, platform, pkg)
			buildContainers = append(buildContainers, buildContainer)
		}
	}
	return buildContainers
}

// builds binary for the specified package
func (m *Harbor) BuildBinary(ctx context.Context, platform Platform, pkg Package) *dagger.File {
	build := m.buildBinary(ctx, platform, pkg)
	return build.Container.File(build.BinaryPath)
}

func (m *Harbor) buildBinary(ctx context.Context, platform Platform, pkg Package) *BuildMetadata {
	var (
		srcWithSwagger *dagger.Directory
		ldflags        string
	)

	if !DEBUG {
		ldflags = "-extldflags=-static -s -w"
	}
	goflags := "-buildvcs=false"
	gcflags := "all=-N -l"

	os, arch, err := parsePlatform(string(platform))
	if err != nil {
		log.Fatalf("Error parsing platform: %v", err)
	}

	srcWithSwagger = m.genAPIs(ctx)
	m.FilteredSrc = m.FilteredSrc.WithDirectory("/src/server/v2.0", srcWithSwagger)

	if pkg == "core" {
		gitCommit := m.fetchGitCommit(ctx)
		version := m.getVersion(ctx)

		m.lintAPIs(ctx)
		// srcWithSwagger = m.genAPIs(ctx)
		// m.Source = srcWithSwagger
		ldflags = fmt.Sprintf(`-X github.com/goharbor/harbor/src/pkg/version.GitCommit=%s
                    -X github.com/goharbor/harbor/src/pkg/version.ReleaseVersion=%s
      `, gitCommit, version)
	}

	outputPath := fmt.Sprintf("bin/%s/%s", platform, pkg)
	src := fmt.Sprintf("%s/main.go", pkg)
	builder := dag.Container().
		From("golang:1.23.2").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod-"+GO_VERSION)).
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithMountedCache("/go/build-cache", dag.CacheVolume("go-build-"+GO_VERSION)).
		WithEnvVariable("GOCACHE", "/go/build-cache").
		// update for better caching
		WithMountedDirectory("/src", m.FilteredSrc.Directory("./src")).
		WithWorkdir("/src").
		WithEnvVariable("GOOS", os).
		WithEnvVariable("GOARCH", arch).
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", goflags, "-gcflags=" + gcflags, "-o", outputPath, "-ldflags", ldflags, src})

	return &BuildMetadata{
		Package:    pkg,
		BinaryPath: outputPath,
		Container:  builder,
		Platform:   platform,
	}
}

// internal function to build Nginx
func (m *Harbor) buildNginx(ctx context.Context, platform Platform) *dagger.Container {
	fmt.Println("🛠️  Building Harbor Nginx...")

	return dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).
		From("nginx:alpine").
		WithExposedPort(8080).
		WithEntrypoint([]string{"nginx", "-g", "daemon off;"})
}

func (m *Harbor) buildRegistry(ctx context.Context, platform Platform) *dagger.Container {
	fmt.Println("🛠️  Building Harbor Registry...")

	regBinary := m.registryBuilder(ctx)
	return dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).
		WithFile("/usr/bin/registry_DO_NOT_USE_GC", regBinary).
		WithExposedPort(5000).
		WithExposedPort(5443).
		WithEntrypoint([]string{"/usr/bin/registry_DO_NOT_USE_GC", "serve", "/etc/registry/config.yml"})
}

// internal function to build Trivy Adapter
func (m *Harbor) buildTrivyAdapter(ctx context.Context, platform Platform) *dagger.Container {
	fmt.Println("🛠️  Building Trivy Adapter...")

	trivyBinDir := dag.Container().From("golang:1.23.2").
		WithWorkdir("/go/src/github.com/goharbor/").
		WithExec([]string{"git", "clone", "-b", TRIVYADAPTERVERSION, "https://github.com/goharbor/harbor-scanner-trivy.git"}).
		WithWorkdir("harbor-scanner-trivy").
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithMountedCache("/go/build-cache", dag.CacheVolume("go-build-"+GO_VERSION)).
		WithEnvVariable("GOCACHE", "/go/build-cache").
		WithEnvVariable("DISTRIBUTION_DIR", "/go/src/github.com/docker/distribution").
		WithEnvVariable("BUILDTAGS", "include_oss include_gcs").
		WithEnvVariable("GO111MODULE", "auto").
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-o", "./binary/scanner-trivy", "cmd/scanner-trivy/main.go"}).
		WithExec([]string{"wget", "-O", "trivyDownload", TRIVY_DOWNLOAD_URL}).
		WithExec([]string{"tar", "-zxv", "-f", "trivyDownload"}).
		WithExec([]string{"cp", "trivy", "./binary/trivy"}).
		Directory("binary")

	trivyAdapter := trivyBinDir.File("./trivy")
	trivyScanner := trivyBinDir.File("./scanner-trivy")

	return dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).
		From("aquasec/trivy:"+TRIVY_VERSION_NO_PREFIX).
		WithFile("/home/scanner/bin/scanner-trivy", trivyScanner).
		WithFile("/usr/local/bin/trivy", trivyAdapter).
		// ENV TRIVY_VERSION=${trivy_version}
		WithEnvVariable("TRIVY_VERSION", TRIVYVERSION).
		WithExposedPort(8080).
		WithExposedPort(8443).
		WithEntrypoint([]string{"/home/scanner/bin/scanner-trivy"})
}

// internal function to build harbor-portal
func (m *Harbor) buildPortal(ctx context.Context, platform Platform) *dagger.Container {
	fmt.Println("🛠️  Building Harbor Portal...")

	swaggerYaml := dag.Container().From("alpine:latest").
		// for better caching
		WithMountedDirectory("/api", m.FilteredSrc.Directory("./api")).
		WithWorkdir("/api").
		File("v2.0/swagger.yaml")

	LICENSE := dag.Container().From("alpine:latest").
		WithMountedDirectory("/harbor", m.FilteredSrc).
		WithWorkdir("/harbor").
		WithExec([]string{"ls"}).
		File("LICENSE")

	// before := dag.Container().
	//    From("node:16.18.0").
	// 	WithMountedCache(USER_HOME_DIR+"/.bun/install/cache", dag.CacheVolume("bun")).
	// 	WithMountedCache(USER_HOME_DIR+"/.npm", dag.CacheVolume("node")).
	//    WithMountedCache("/root/.npm", dag.CacheVolume("node-16")).
	// 	// for better caching
	// 	WithMountedDirectory("/harbor", m.Source).
	// 	WithWorkdir("/harbor/src/portal").
	// 	WithEnvVariable("NPM_CONFIG_REGISTRY", NPM_REGISTRY).
	// 	WithEnvVariable("BUN_INSTALL_CACHE_DIR", "/root/.bun/install/cache").
	//    // $BUN_INSTALL_CACHE_DIR
	// 	// WithExec([]string{"bun", "pm", "trust", "--all"}).
	// 	WithFile("swagger.yaml", swaggerYaml).
	// 	WithExec([]string{"npm", "install", "--unsafe-perm"}).
	// 	WithExec([]string{"npm", "run", "generate-build-timestamp"}).
	// 	WithExec([]string{"npm", "run", "release"})

	before := dag.Container().
		From("oven/bun:1.2.4").
		WithMountedCache(USER_HOME_DIR+"/.bun/install/cache", dag.CacheVolume("bun")).
		WithMountedCache("/root/.npm", dag.CacheVolume("node-16")).
		WithMountedCache("/root/.angular", dag.CacheVolume("angular")).
		// for better caching
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor/src/portal").
		WithEnvVariable("NPM_CONFIG_REGISTRY", NPM_REGISTRY).
		WithEnvVariable("BUN_INSTALL_CACHE_DIR", "/root/.bun/install/cache").
		// $BUN_INSTALL_CACHE_DIR
		// WithExec([]string{"bun", "pm", "trust", "--all"}).
		WithFile("swagger.yaml", swaggerYaml).
		WithExec([]string{"apt", "update"}).
		WithExec([]string{"apt", "install", "unzip"}).
		WithExec([]string{"bun", "install", "--no-verify", "--unsafe-perm"}).
		WithExec([]string{"bun", "pm", "trust", "--all"}).
		WithExec([]string{"bun", "install", "--no-verify"}).
		WithExec([]string{"ls", "-al"}).
		WithExec([]string{"bun", "run", "generate-build-timestamp"}).
		WithExec([]string{"bun", "run", "node", "--max_old_space_size=2048", "node_modules/@angular/cli/bin/ng", "build", "--configuration", "production"})

	builder := before.
		WithExec([]string{"bun", "install", "js-yaml@4.1.0", "--no-verify"}).
		WithExec([]string{"sh", "-c", fmt.Sprintf("bun -e \"const yaml = require('js-yaml'); const fs = require('fs'); const swagger = yaml.load(fs.readFileSync('swagger.yaml', 'utf8')); fs.writeFileSync('swagger.json', JSON.stringify(swagger));\" ")}).
		WithFile("/harbor/src/portal/dist/LICENSE", LICENSE)

	builderDir := builder.Directory("/harbor")

	// swagger UI only supports npm some edge case error
	swagger := dag.Container().From("node:16.18.0").
		WithMountedCache("/root/.npm", dag.CacheVolume("node-16")).
		WithMountedCache("/root/.angular", dag.CacheVolume("angular")).
		WithMountedDirectory("/harbor", builderDir).
		WithWorkdir("/harbor/src/portal/app-swagger-ui").
		WithExec([]string{"npm", "install", "--unsafe-perm"}).
		WithExec([]string{"npm", "run", "build"}).
		WithWorkdir("/harbor/src/portal")

	deployer := dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).From("nginx:alpine").
		WithFile("/usr/share/nginx/html/swagger.json", builder.File("/harbor/src/portal/swagger.json")).
		WithDirectory("/usr/share/nginx/html", builder.Directory("/harbor/src/portal/dist")).
		WithDirectory("/usr/share/nginx/html", swagger.Directory("/harbor/src/portal/app-swagger-ui/dist")).
		WithWorkdir("/usr/share/nginx/html").
		WithExec([]string{"ls"}).
		WithWorkdir("/").
		WithExposedPort(8080).
		WithExposedPort(8443).
		WithEntrypoint([]string{"nginx", "-g", "daemon off;"})

	return deployer
}

// use to parse given platform as string
func parsePlatform(platform string) (string, string, error) {
	parts := strings.Split(platform, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid platform format: %s. Should be os/arch. E.g. darwin/amd64", platform)
	}
	return parts[0], parts[1], nil
}

// fetches git commit
func (m *Harbor) fetchGitCommit(ctx context.Context) string {
	dirOpts := dagger.ContainerWithDirectoryOpts{
		Include: []string{".git"},
	}

	// temp container with git installed
	temp := dag.Container().
		From("golang:1.23.2").
		WithDirectory("/src", m.FilteredSrc, dirOpts).
		WithWorkdir("/src")

	gitCommit, _ := temp.WithExec([]string{"git", "rev-parse", "--short=8", "HEAD"}).Stdout(ctx)

	return gitCommit
}

// generate APIs
func (m *Harbor) genAPIs(_ context.Context) *dagger.Directory {
	SWAGGER_VERSION := "v0.25.0"
	SWAGGER_SPEC := "api/v2.0/swagger.yaml"
	TARGET_DIR := "src/server/v2.0"
	APP_NAME := "harbor"

	temp := dag.Container().
		From("quay.io/goswagger/swagger:"+SWAGGER_VERSION).
		WithMountedDirectory("/src", m.FilteredSrc).
		WithWorkdir("/src").
		WithExec([]string{"swagger", "version"}).
		// Clean up old generated code and create necessary directories
		WithExec([]string{"rm", "-rf", TARGET_DIR + "/{models,restapi}"}).
		WithExec([]string{"mkdir", "-p", TARGET_DIR}).
		// Generate the server files using the Swagger tool
		WithExec([]string{"swagger", "generate", "server", "--template-dir=./tools/swagger/templates", "--exclude-main", "--additional-initialism=CVE", "--additional-initialism=GC", "--additional-initialism=OIDC", "-f", SWAGGER_SPEC, "-A", APP_NAME, "--target", TARGET_DIR}).
		WithExec([]string{"ls", "-la"}).
		Directory("/src/src/server/v2.0")

	return temp
}

// get version from VERSION file
func (m *Harbor) getVersion(ctx context.Context) string {
	dirOpts := dagger.ContainerWithDirectoryOpts{
		Include: []string{"VERSION"},
	}

	temp := dag.Container().
		From("golang:1.23.2").
		WithDirectory("/src", m.FilteredSrc, dirOpts).
		WithWorkdir("/src").
		WithExec([]string{"ls", "-la"})

	version, _ := temp.WithExec([]string{"cat", "VERSION"}).Stdout(ctx)
	return version
}
