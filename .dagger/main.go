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

var (
	SupportedPlatforms = []string{"linux/arm64", "linux/amd64"}
	packages           = []string{"core", "jobservice", "registryctl", "cmd/exporter", "cmd/standalone-db-migrator"}
	//packages = []string{"core", "jobservice"}
)

type BuildMetadata struct {
	Package    string
	BinaryPath string
	Container  *dagger.Container
	Platform   string
}

func New(
// Local or remote directory with source code, defaults to "./"
// +optional
// +defaultPath="./"
	source *dagger.Directory,
) *Harbor {
	return &Harbor{Source: source}
}

type Harbor struct {
	Source *dagger.Directory
}

func (m *Harbor) ExportAllImages(ctx context.Context) (string, error) {
	metdata := m.buildAllImages(ctx)
	for _, meta := range metdata {
		export, err := meta.Container.Export(ctx, fmt.Sprintf("bin/container/%s/%s.tgz", meta.Platform, meta.Package))
		export, err := meta.Container.AsTarball(ctx, fmt.Sprintf("bin/container/%s/%s.tgz", meta.Platform, meta.Package))
		println(export)
		if err != nil {
			return "", err
		}
	}
	return "bin/container", nil
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
	for _, platform := range SupportedPlatforms {
		for _, pkg := range packages {
			img := m.BuildImage(ctx, platform, pkg)
			buildMetadata = append(buildMetadata, &BuildMetadata{
				Package:    pkg,
				BinaryPath: fmt.Sprintf("bin/%s/%s", platform, pkg),
				Container:  img,
				Platform:   platform,
			})
		}
		// build portal
	}
	return buildMetadata
}

func (m *Harbor) BuildImage(ctx context.Context, platform string, pkg string) *dagger.Container {
	return m.buildImage(ctx, platform, pkg).Container
}

func (m *Harbor) buildImage(ctx context.Context, platform string, pkg string) *BuildMetadata {
	build := m.buildBinary(ctx, platform, pkg)
	img := dag.Container(dagger.ContainerOpts{Platform: dagger.Platform(platform)}).
		WithFile("/"+pkg, build.Container.File(build.BinaryPath)).
		WithEntrypoint([]string{"/" + pkg})
	build.Container = img
	return build
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
	for _, platform := range SupportedPlatforms {
		for _, pkg := range packages {
			buildContainer := m.buildBinary(ctx, platform, pkg)
			buildContainers = append(buildContainers, buildContainer)
		}
		// build portal
	}
	return buildContainers
}

func (m *Harbor) BuildBinary(ctx context.Context, platform string, pkg string) *dagger.File {
	build := m.buildBinary(ctx, platform, pkg)
	return build.Container.File(build.BinaryPath)
}

func (m *Harbor) buildBinary(ctx context.Context, platform string, pkg string) *BuildMetadata {

	os, arch, err := parsePlatform(platform)
	if err != nil {
		log.Fatalf("Error parsing platform: %v", err)
	}

	outputPath := fmt.Sprintf("bin/%s/%s", platform, pkg)
	src := fmt.Sprintf("%s/main.go", pkg)
	builder := dag.Container().
		From("golang:latest").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("go-mod-"+GO_VERSION)).
		WithEnvVariable("GOMODCACHE", "/go/pkg/mod").
		WithMountedCache("/go/build-cache", dag.CacheVolume("go-build-"+GO_VERSION)).
		WithEnvVariable("GOCACHE", "/go/build-cache").
		WithMountedDirectory("/harbor", m.Source). // Ensure the source directory with go.mod is mounted
		WithWorkdir("/harbor/src/").
		WithEnvVariable("GOOS", os).
		WithEnvVariable("GOARCH", arch).
		WithEnvVariable("CGO_ENABLED", "0").
		WithExec([]string{"go", "build", "-o", outputPath, "-ldflags", "-extldflags=-static -s -w", src})

	return &BuildMetadata{
		Package:    pkg,
		BinaryPath: outputPath,
		Container:  builder,
		Platform:   platform,
	}
}

func (m *Harbor) buildPortal(ctx context.Context, platform string, pkg string) *dagger.Directory {
	fmt.Println("🛠️  Building Harbor Core...")
	// Define the path for the binary output
	os, arch, err := parsePlatform(platform)

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
		WithMountedDirectory("/harbor", m.Source). // Ensure the source directory with go.mod is mounted
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
