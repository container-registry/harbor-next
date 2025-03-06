package main

import (
	"context"
	"dagger/harbor/internal/dagger"
	"fmt"
	"log"
	"strings"
)

// to-do: update registry to v3
// to-do: add documentation
// to-do: stop usage of shell things. No shell spawning

type (
	Package  string
	Platform string
)

var (
	targetPlatforms = []Platform{"linux/amd64", "linux/arm64"}
	packages        = []Package{"core", "jobservice", "registryctl", "portal", "registry", "nginx", "cmd/exporter", "cmd/standalone-db-migrator", "trivy-adapter"}
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
	// +ignore=["bin"]
	source *dagger.Directory,
) *Harbor {
	return &Harbor{Source: source}
}

type Harbor struct {
	Source *dagger.Directory
}

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

// publishes the specific package with the given tag and version
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

func (m *Harbor) ExportAllImages(ctx context.Context) *dagger.Directory {
	metdata := m.buildAllImages(ctx)
	artifacts := dag.Directory()
	for _, meta := range metdata {
		artifacts = artifacts.WithFile(fmt.Sprintf("containers/%s/%s.tgz", meta.Platform, meta.Package), meta.Container.AsTarball())
	}
	return artifacts
}

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
		WithMountedFile("/redis.patch", m.Source.File(".dagger/registry/redis.patch")).
		WithWorkdir("/go/src/github.com/docker").
		WithExec([]string{"git", "clone", "-b", REGISTRY_SRC_TAG, DISTRIBUTION_SRC}).
		WithWorkdir("distribution").
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
		img = dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).From("busybox:latest").
			WithFile("/"+string(pkg), buildMtd.Container.File(buildMtd.BinaryPath))

		// Set entrypoint
		if pkg == "jobservice" {
			img = img.WithEntrypoint([]string{"/" + string(pkg), "-c", "/etc/jobservice/config.yml"})
		} else if pkg == "registryctl" {
			img = img.WithEntrypoint([]string{"/" + string(pkg), "-c", "/etc/registryctl/config.yml"})
		}
	}

	buildMtd.Container = img
	return buildMtd
}

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
	var srcWithSwagger *dagger.Directory
	ldflags := "-extldflags=-static -s -w"
	goflags := "-buildvcs=false"

	os, arch, err := parsePlatform(string(platform))
	if err != nil {
		log.Fatalf("Error parsing platform: %v", err)
	}

	if pkg == "core" {
		gitCommit := m.fetchGitCommit(ctx)
		version := m.getVersion(ctx)

		m.lintAPIs(ctx)
		srcWithSwagger = m.genAPIs(ctx)
		m.Source = srcWithSwagger

		ldflags = fmt.Sprintf(`-X github.com/goharbor/harbor/src/pkg/version.GitCommit=%s
                    -X github.com/goharbor/harbor/src/pkg/version.ReleaseVersion=%s
      `, gitCommit, version)
	}

	outputPath := fmt.Sprintf("bin/%s/%s", platform, pkg)
	src := fmt.Sprintf("%s/main.go", pkg)
	builder := dag.Container().
		From("golang:latest").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod-"+GO_VERSION)).
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithMountedCache("/go/build-cache", dag.CacheVolume("go-build-"+GO_VERSION)).
		WithEnvVariable("GOCACHE", "/go/build-cache").
		// update for better caching
		WithMountedDirectory("/src", m.Source.Directory("./src")).
		WithWorkdir("/src").
		WithEnvVariable("GOOS", os).
		WithEnvVariable("GOARCH", arch).
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", goflags, "-o", outputPath, "-ldflags", ldflags, src})

	return &BuildMetadata{
		Package:    pkg,
		BinaryPath: outputPath,
		Container:  builder,
		Platform:   platform,
	}
}

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

func (m *Harbor) buildPortal(ctx context.Context, platform Platform) *dagger.Container {
	fmt.Println("🛠️  Building Harbor Portal...")

	m.Source = m.genAPIs(ctx)

	swaggerYaml := dag.Container().From("alpine:latest").
		// for better caching
		WithMountedDirectory("/api", m.Source.Directory("./api")).
		WithWorkdir("/api").
		File("v2.0/swagger.yaml")

	LICENSE := dag.Container().From("alpine:latest").
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor").
		WithExec([]string{"ls"}).
		File("LICENSE")

	builder := dag.Container().
		From("node:16.18.0").
		WithMountedCache("/root/.npm", dag.CacheVolume("node")).
		// for better caching
		WithMountedDirectory("/portal", m.Source.Directory("./src/portal")).
		WithWorkdir("/portal").
		WithFile("swagger.yaml", swaggerYaml).
		WithEnvVariable("NPM_CONFIG_REGISTRY", NPM_REGISTRY).
		WithExec([]string{"npm", "install", "--unsafe-perm"}).
		WithExec([]string{"npm", "run", "generate-build-timestamp"}).
		WithExec([]string{"node", "--max_old_space_size=2048", "node_modules/@angular/cli/bin/ng", "build", "--configuration", "production"}).
		WithExec([]string{"npm", "install", "js-yaml@4.1.0"}).
		WithExec([]string{"sh", "-c", fmt.Sprintf("node -e \"const yaml = require('js-yaml'); const fs = require('fs'); const swagger = yaml.load(fs.readFileSync('swagger.yaml', 'utf8')); fs.writeFileSync('swagger.json', JSON.stringify(swagger));\" ")}).
		WithFile("dist/LICENSE", LICENSE).
		WithWorkdir("app-swagger-ui").
		WithExec([]string{"npm", "install", "--unsafe-perm"}).
		WithExec([]string{"npm", "run", "build"}).
		WithWorkdir("/portal")

	deployer := dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).From("nginx:alpine").
		WithFile("/usr/share/nginx/html/swagger.json", builder.File("/portal/swagger.json")).
		WithDirectory("/usr/share/nginx/html", builder.Directory("/portal/dist")).
		WithDirectory("/usr/share/nginx/html", builder.Directory("/portal/app-swagger-ui/dist")).
		WithWorkdir("/usr/share/nginx/html").
		WithExec([]string{"ls"}).
		WithWorkdir("/").
		WithExposedPort(8080).
		WithExposedPort(8443).
		WithEntrypoint([]string{"nginx", "-g", "daemon off;"})

	return deployer
}

func parsePlatform(platform string) (string, string, error) {
	parts := strings.Split(platform, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid platform format: %s. Should be os/arch. E.g. darwin/amd64", platform)
	}
	return parts[0], parts[1], nil
}

func (m *Harbor) fetchGitCommit(ctx context.Context) string {
	// temp container with git installed
	temp := dag.Container().
		From("golang:latest").
		WithMountedDirectory("/src", m.Source).
		WithWorkdir("/src")

	gitCommit, _ := temp.WithExec([]string{"git", "rev-parse", "--short=8", "HEAD"}).Stdout(ctx)

	return gitCommit
}

func (m *Harbor) genAPIs(_ context.Context) *dagger.Directory {
	SWAGGER_VERSION := "v0.25.0"
	SWAGGER_SPEC := "api/v2.0/swagger.yaml"
	TARGET_DIR := "src/server/v2.0"
	APP_NAME := "harbor"

	temp := dag.Container().
		From("quay.io/goswagger/swagger:"+SWAGGER_VERSION).
		WithMountedDirectory("/src", m.Source).
		WithWorkdir("/src").
		WithExec([]string{"swagger", "version"}).
		// Clean up old generated code and create necessary directories
		WithExec([]string{"rm", "-rf", TARGET_DIR + "/{models,restapi}"}).
		WithExec([]string{"mkdir", "-p", TARGET_DIR}).
		// Generate the server files using the Swagger tool
		WithExec([]string{"swagger", "generate", "server", "--template-dir=./tools/swagger/templates", "--exclude-main", "--additional-initialism=CVE", "--additional-initialism=GC", "--additional-initialism=OIDC", "-f", SWAGGER_SPEC, "-A", APP_NAME, "--target", TARGET_DIR}).
		Directory("/src")

	return temp
}

func (m *Harbor) getVersion(ctx context.Context) string {
	// temp container with git installed
	dirOpts := dagger.ContainerWithDirectoryOpts{
		Include: []string{"VERSION"},
	}

	temp := dag.Container().
		From("golang:latest").
		WithDirectory("/src", m.Source, dirOpts).
		WithWorkdir("/src").
		WithExec([]string{"ls", "-la"})

	version, _ := temp.WithExec([]string{"cat", "VERSION"}).Stdout(ctx)
	return version
}
