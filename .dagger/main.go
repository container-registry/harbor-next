package main

import (
	"context"
	"dagger/harbor/internal/dagger"
	"fmt"
	"log"
	"strings"
)

const (
	GOLANGCILINT_VERSION = "v1.61.0"
	GO_VERSION           = "latest"
	SYFT_VERSION         = "v1.9.0"
	GORELEASER_VERSION   = "v2.3.2"
)

type (
	Package  string
	Platform string
)

var (
	targetPlatforms = []Platform{"linux/arm64", "linux/amd64"}
	packages        = []Package{"core", "jobservice", "registryctl", "cmd/exporter", "cmd/standalone-db-migrator"}
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
	source *dagger.Directory,
) *Harbor {
	return &Harbor{Source: source}
}

type Harbor struct {
	Source *dagger.Directory
}

// LintReport Executes the Linter and writes the linting results to a file golangci-linter-report.sarif
func (m *Harbor) LintReport(ctx context.Context) (string, error) {
	report := "golangci-lint-report.sarif"
	// output, _ = m.Source.File(report).Name(ctx)
	return m.linter(ctx).WithExec([]string{
		"golangci-lint", "run",
		"--out-format", "sarif:" + report,
		"--issues-exit-code", "0",
	}).File(report).Export(ctx, report)
}

// Lint Run the linter golangci-linter
func (m *Harbor) Lint(ctx context.Context) (string, error) {
	return m.linter(ctx).WithExec([]string{"golangci-lint", "run"}).Stderr(ctx)
}

func (m *Harbor) linter(_ context.Context) *dagger.Container {
	fmt.Printf("👀 Running linter")
	linter := dag.Container().
		From("golangci/golangci-lint:"+GOLANGCILINT_VERSION+"-alpine").
		WithMountedCache("/lint-cache", dag.CacheVolume("/lint-cache")).
		WithEnvVariable("GOLANGCI_LINT_CACHE", "/lint-cache").
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor/src/")
	return linter
}

func (m *Harbor) PublishAndSignAllImages(
	ctx context.Context,
	registry string,
	registryUsername string,
	version string,
	registryPassword *dagger.Secret,
	imageTags []string,
	// +optional
	githubToken *dagger.Secret,
	// +optional
	actionsIdTokenRequestToken *dagger.Secret,
	// +optional
	actionsIdTokenRequestUrl string,
) (string, error) {
	imageAddrs := m.PublishAllImages(ctx, registry, registryUsername, imageTags, registryPassword, version)
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

// to-do: going to work on this
func (m *Harbor) PublishAllImages(
	ctx context.Context,
	registry, registryUsername string,
	imageTags []string,
	registryPassword *dagger.Secret,
	version string,
) []string {
	allImages := m.buildAllImages(ctx, version)

	// for i, tag := range imageTags {
	// 	imageTags[i] = strings.TrimSpace(tag)
	// 	if strings.HasPrefix(imageTags[i], "v") {
	// 		imageTags[i] = strings.TrimPrefix(imageTags[i], "v")
	// 	}
	// }
	fmt.Printf("provided tags: %s\n", imageTags)

	platformVariantsContainer := make(map[Package][]*dagger.Container)
	for _, meta := range allImages {
		platformVariantsContainer[meta.Package] = append(platformVariantsContainer[meta.Package], meta.Container)
	}

	var imageAddresses []string
	for pkg, imgs := range platformVariantsContainer {
		for _, imageTag := range imageTags {
			container := dag.Container().WithRegistryAuth(registry, registryUsername, registryPassword)
			imgAddress, err := container.Publish(ctx,
				fmt.Sprintf("%s/%s/%s:%s", registry, "harbor", pkg, imageTag),
				dagger.ContainerPublishOpts{PlatformVariants: imgs},
			)
			if err != nil {
				fmt.Printf("Failed to publish image: %s/%s/%s:%s\n", registry, "harbor", pkg, imageTag)
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
	version string,
) []string {
	BuildImage := m.BuildImage(ctx, targetPlatforms[1], pkg, version)

	// why do we need this @vad1mo any ideas??
	// for i, tag := range imageTags {
	// 	imageTags[i] = strings.TrimSpace(tag)
	// 	if strings.HasPrefix(imageTags[i], "v") {
	// 		imageTags[i] = strings.TrimPrefix(imageTags[i], "v")
	// 	}
	// }
	fmt.Printf("provided tags: %s\n", imageTags)

	var (
		imageAddresses []string
		images         []*dagger.Container
	)
	images = append(images, BuildImage)
	for _, imageTag := range imageTags {
		container := BuildImage.WithRegistryAuth(registry, registryUsername, registryPassword)
		imgAddress, err := container.Publish(ctx,
			fmt.Sprintf("%s/%s/%s:%s", registry, "harbor", pkg, imageTag),
		)
		if err != nil {
			fmt.Printf("Failed to publish image: %s/%s/%s:%s\n", registry, "harbor", pkg, imageTag)
			fmt.Printf("Error: %s\n", err)
			continue
		}
		imageAddresses = append(imageAddresses, imgAddress)
		fmt.Printf("Published image: %s\n", imgAddress)
	}

	return imageAddresses
}

func (m *Harbor) ExportAllImages(ctx context.Context, version string) *dagger.Directory {
	metdata := m.buildAllImages(ctx, version)
	artifacts := dag.Directory()
	for _, meta := range metdata {
		artifacts = artifacts.WithFile(fmt.Sprintf("containers/%s/%s.tgz", meta.Platform, meta.Package), meta.Container.AsTarball())
	}
	return artifacts
}

func (m *Harbor) BuildAllImages(ctx context.Context, version string) []*dagger.Container {
	metdata := m.buildAllImages(ctx, version)
	images := make([]*dagger.Container, len(metdata))
	for i, meta := range metdata {
		images[i] = meta.Container
	}
	return images
}

func (m *Harbor) buildAllImages(ctx context.Context, version string) []*BuildMetadata {
	var buildMetadata []*BuildMetadata
	for _, platform := range targetPlatforms {
		for _, pkg := range packages {
			img := m.BuildImage(ctx, platform, pkg, version)
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

func (m *Harbor) BuildImage(ctx context.Context, platform Platform, pkg Package, version string) *dagger.Container {
	buildMtd := m.buildImage(ctx, platform, pkg, version)
	if pkg == "core" {
		// the only thing missing here is the healthcheck
		// we can add those by updating the docker compose since dagger currently doesn't support healthchecks
		// issue: https://github.com/dagger/dagger/issues/9515
		buildMtd.Container = buildMtd.Container.WithDirectory("/migrations", m.Source.Directory("make/migrations")).
			WithDirectory("/icons", m.Source.Directory("icons")).
			WithDirectory("/views", m.Source.Directory("src/core/views"))
	}
	return buildMtd.Container
}

func (m *Harbor) buildImage(ctx context.Context, platform Platform, pkg Package, version string) *BuildMetadata {
	buildMtd := m.buildBinary(ctx, platform, pkg, version)
	img := dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(string(platform))}).
		WithFile("/"+string(pkg), buildMtd.Container.File(buildMtd.BinaryPath))
	if pkg == "jobservice" {
		img = img.WithEntrypoint([]string{"/" + string(pkg), "-c", "/etc/jobservice/config.yml"})
	} else {
		img = img.WithEntrypoint([]string{"/" + string(pkg)})
	}
	buildMtd.Container = img
	return buildMtd
}

func (m *Harbor) BuildAllBinaries(ctx context.Context, version string) *dagger.Directory {
	output := dag.Directory()
	builds := m.buildAllBinaries(ctx, version)
	for _, build := range builds {
		output = output.WithFile(build.BinaryPath, build.Container.File(build.BinaryPath))
	}
	return output
}

func (m *Harbor) buildAllBinaries(ctx context.Context, version string) []*BuildMetadata {
	var buildContainers []*BuildMetadata
	for _, platform := range targetPlatforms {
		for _, pkg := range packages {
			buildContainer := m.buildBinary(ctx, platform, pkg, version)
			buildContainers = append(buildContainers, buildContainer)
		}
	}
	return buildContainers
}

// builds binary for the specified package
func (m *Harbor) BuildBinary(ctx context.Context, platform Platform, pkg Package, version string) *dagger.File {
	build := m.buildBinary(ctx, platform, pkg, version)
	return build.Container.File(build.BinaryPath)
}

func (m *Harbor) buildBinary(ctx context.Context, platform Platform, pkg Package, version string) *BuildMetadata {
	ldflags := "-extldflags=-static -s -w"
	goflags := "-buildvcs=false"

	os, arch, err := parsePlatform(string(platform))
	if err != nil {
		log.Fatalf("Error parsing platform: %v", err)
	}

	gitCommit := m.fetchGitCommit(ctx)
	if pkg == "core" {
		m.lintAPIs(ctx)
		dirWithSwagger := m.genAPIs(ctx)
		m.Source = dirWithSwagger

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
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor/src/").
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

func (m *Harbor) buildPortal(ctx context.Context, platform Platform, pkg Package) *dagger.Directory {
	fmt.Println("🛠️  Building Harbor Core...")
	os, arch, err := parsePlatform(string(platform))
	if err != nil {
		log.Fatalf("Error parsing platform: %v", err)
	}

	outputPath := fmt.Sprintf("bin/%s/%s", platform, pkg)
	src := fmt.Sprintf("src/%s/main.go", pkg)
	builder := dag.Container().
		From("golang:latest").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod-"+GO_VERSION)).
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithMountedCache("/go/build-cache", dag.CacheVolume("go-build-"+GO_VERSION)).
		WithEnvVariable("GOCACHE", "/go/build-cache").
		WithMountedDirectory("/harbor", m.Source).
		WithWorkdir("/harbor").
		WithEnvVariable("GOOS", os).
		WithEnvVariable("GOARCH", arch).
		WithExec([]string{"go", "build", "-o", outputPath, src})
	return builder.Directory(outputPath)
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

	gitCommit, _ := temp.WithExec([]string{"git", "rev-parse", "--short=8", "HEAD", "--always"}).Stdout(ctx)

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

func (m *Harbor) lintAPIs(_ context.Context) *dagger.Directory {
	temp := dag.Container().
		From("stoplight/spectral:6.11.1").
		WithMountedDirectory("/src", m.Source).
		WithWorkdir("/src").
		WithExec([]string{"spectral", "--version"}).
		WithExec([]string{"spectral", "lint", "./api/v2.0/swagger.yaml"}).
		Directory("/src")

	return temp
}
