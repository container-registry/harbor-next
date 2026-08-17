package main

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxPackageDBEntrySize = 512 << 20
	packageDBCacheVersion = 4
)

type packageDBProvider struct {
	packageType string
	cataloger   string
}

type packageDBCacheMarker struct {
	SchemaVersion int    `json:"schema_version"`
	PackageType   string `json:"package_type"`
	Packages      int    `json:"packages"`
}

var packageDBProviders = []packageDBProvider{
	{packageType: "rpm", cataloger: "rpm-db-cataloger"},
	{packageType: "alpm", cataloger: "alpm-db-cataloger"},
	{packageType: "deb", cataloger: "dpkg-db-cataloger"},
	{packageType: "apk", cataloger: "apk-db-cataloger"},
	{packageType: "nix", cataloger: "nix-cataloger"},
}

func packageDBSBOM(ctx context.Context, workDir string, reg *registry, repo string, layers []descriptor, distro syftDistro) (string, error) {
	var candidateErrors []error
	for i := len(layers) - 1; i >= 0; i-- {
		root, provider, packages, err := cachedPackageDBInventory(ctx, workDir, reg, repo, layers[i])
		if err != nil {
			candidateErrors = append(candidateErrors, fmt.Errorf("layer %s: %w", layers[i].Digest, err))
			continue
		}
		if provider == nil {
			continue
		}
		if packages == 0 {
			candidateErrors = append(candidateErrors, fmt.Errorf("layer %s: empty %s database", layers[i].Digest, provider.packageType))
			continue
		}
		return runSyftPackageDB(ctx, workDir, root, provider, distro)
	}
	if len(candidateErrors) > 0 {
		return "", fmt.Errorf("inspect bootc package databases: %w", errors.Join(candidateErrors...))
	}
	return "", errors.New("bootc image has no supported package database in its layers")
}

func cachedPackageDBInventory(ctx context.Context, workDir string, reg *registry, repo string, layer descriptor) (string, *packageDBProvider, int, error) {
	cacheRoot := envOrDefault("SCANNER_GRYPE_CACHE_DIR", filepath.Join(workDir, "grype-cache"))
	cacheDir := filepath.Join(cacheRoot, "package-db", strings.TrimPrefix(layer.Digest, "sha256:"))
	markerPath := filepath.Join(cacheDir, ".inventory.json")
	if data, err := os.ReadFile(markerPath); err == nil {
		var marker packageDBCacheMarker
		if json.Unmarshal(data, &marker) == nil && marker.SchemaVersion == packageDBCacheVersion {
			if marker.PackageType == "" {
				return "", nil, 0, nil
			}
			if provider := packageDBProviderByType(marker.PackageType); provider != nil {
				return cacheDir, provider, marker.Packages, nil
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(cacheDir), 0o700); err != nil {
		return "", nil, 0, fmt.Errorf("create package database cache: %w", err)
	}
	layerPath := filepath.Join(workDir, "package-db-layer")
	if err := downloadBlob(ctx, reg, repo, layer.Digest, layerPath); err != nil {
		return "", nil, 0, err
	}
	temporary := cacheDir + fmt.Sprintf(".tmp-%d", os.Getpid())
	_ = os.RemoveAll(temporary)
	provider, packages, err := extractPackageDBInventory(layerPath, temporary)
	if err != nil {
		_ = os.RemoveAll(temporary)
		return "", nil, 0, err
	}
	marker := packageDBCacheMarker{SchemaVersion: packageDBCacheVersion, Packages: packages}
	if provider != nil {
		marker.PackageType = provider.packageType
	}
	data, err := json.Marshal(marker)
	if err != nil {
		_ = os.RemoveAll(temporary)
		return "", nil, 0, err
	}
	if err := os.MkdirAll(temporary, 0o700); err != nil {
		return "", nil, 0, err
	}
	if err := os.WriteFile(filepath.Join(temporary, ".inventory.json"), data, 0o600); err != nil {
		_ = os.RemoveAll(temporary)
		return "", nil, 0, err
	}
	_ = os.RemoveAll(cacheDir)
	if err := os.Rename(temporary, cacheDir); err != nil {
		return "", nil, 0, fmt.Errorf("cache package database inventory: %w", err)
	}
	if provider == nil {
		return "", nil, 0, nil
	}
	return cacheDir, provider, packages, nil
}

func extractPackageDBInventory(layerPath, root string) (*packageDBProvider, int, error) {
	tr, closeArchive, err := openLayerTar(layerPath)
	if err != nil {
		return nil, 0, err
	}
	defer closeArchive()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, 0, fmt.Errorf("create package database root: %w", err)
	}

	var selected *packageDBProvider
	packages := 0
	sawRPMHardlink := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("read layer tar: %w", err)
		}
		name := cleanTarPath(header.Name)
		if header.Typeflag == tar.TypeLink && isRPMDBPath(name) {
			sawRPMHardlink = true
			continue
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		canonical := ""
		isPackage := false
		if name == "etc/os-release" || name == "usr/lib/os-release" {
			canonical = name
		} else {
			provider, destination, ok := packageDBProviderForPath(name)
			if !ok || (selected != nil && selected.packageType != provider.packageType) {
				continue
			}
			selected = provider
			canonical = destination
			isPackage = true
		}
		if header.Size < 0 || header.Size > maxPackageDBEntrySize {
			return nil, 0, fmt.Errorf("package database entry %q has invalid size %d", name, header.Size)
		}
		destination, err := archiveDestination(root, canonical)
		if err != nil {
			return nil, 0, err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return nil, 0, fmt.Errorf("create package database directory: %w", err)
		}
		if err := writeTarEntry(destination, 0o600, io.LimitReader(tr, header.Size)); err != nil {
			return nil, 0, fmt.Errorf("extract package database entry %q: %w", name, err)
		}
		if isPackage {
			packages++
		}
	}
	if sawRPMHardlink {
		if selected != nil && selected.packageType != packageTypeRPM {
			return nil, 0, fmt.Errorf("layer contains both %s and RPM package databases", selected.packageType)
		}
		if selected != nil {
			return selected, packages, nil
		}
		destination, err := archiveDestination(root, "usr/share/rpm/rpmdb.sqlite")
		if err != nil {
			return nil, 0, err
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return nil, 0, fmt.Errorf("create RPM database directory: %w", err)
		}
		if err := extractRPMDB(layerPath, destination); err != nil {
			return nil, 0, fmt.Errorf("extract hardlinked RPM database: %w", err)
		}
		selected = packageDBProviderByType(packageTypeRPM)
		packages = 1
	}
	return selected, packages, nil
}

func packageDBProviderForPath(path string) (*packageDBProvider, string, bool) {
	path = cleanTarPath(path)
	if isRPMDBPath(path) {
		return packageDBProviderByType("rpm"), path, true
	}
	if index := strings.Index(path, "var/lib/pacman/local/"); index >= 0 {
		suffix := path[index+len("var/lib/pacman/local/"):]
		if validALPMDescription(suffix) {
			return packageDBProviderByType("alpm"), "var/lib/pacman/local/" + suffix, true
		}
	}
	if suffix, ok := strings.CutPrefix(path, "usr/lib/sysimage/lib/pacman/local/"); ok && validALPMDescription(suffix) {
		return packageDBProviderByType("alpm"), "var/lib/pacman/local/" + suffix, true
	}
	switch path {
	case "var/lib/dpkg/status", "usr/lib/sysimage/var/lib/dpkg/status":
		return packageDBProviderByType("deb"), "var/lib/dpkg/status", true
	case "lib/apk/db/installed", "usr/lib/sysimage/lib/apk/db/installed":
		return packageDBProviderByType("apk"), "lib/apk/db/installed", true
	case "nix/var/nix/db/db.sqlite":
		return packageDBProviderByType("nix"), path, true
	default:
		return nil, "", false
	}
}

func validALPMDescription(suffix string) bool {
	parts := strings.Split(suffix, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] == "desc"
}

func packageDBProviderByType(packageType string) *packageDBProvider {
	for i := range packageDBProviders {
		if packageDBProviders[i].packageType == packageType {
			return &packageDBProviders[i]
		}
	}
	return nil
}

func runSyftPackageDB(ctx context.Context, workDir, root string, provider *packageDBProvider, distro syftDistro) (string, error) {
	out := filepath.Join(workDir, "sbom."+provider.packageType+".syft.json")
	args := []string{
		"scan", "dir:" + root, "-q", "-o", "syft-json=" + out,
		"--override-default-catalogers", provider.cataloger,
	}
	cmd := exec.CommandContext(ctx, "syft", args...)
	cmd.Env = scannerEnv(workDir, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("catalog %s database with Syft: %w: %s", provider.packageType, err, strings.TrimSpace(string(output)))
	}
	data, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("read %s SBOM: %w", provider.packageType, err)
	}
	var inventory sbomDocument
	if err := json.Unmarshal(data, &inventory); err != nil {
		return "", fmt.Errorf("decode %s SBOM: %w", provider.packageType, err)
	}
	for _, artifact := range inventory.Artifacts {
		if artifact.Type == provider.packageType {
			switch provider.packageType {
			case "rpm":
				if distro.ID != "" {
					if err := setSyftDistro(out, distro); err != nil {
						return "", err
					}
				}
			case "alpm":
				if err := setSyftDistro(out, syftDistro{
					PrettyName: "Arch Linux",
					Name:       "Arch Linux",
					ID:         "arch",
					VersionID:  "rolling",
					IDLike:     []string{"archlinux"},
				}); err != nil {
					return "", err
				}
			}
			return out, nil
		}
	}
	return "", fmt.Errorf("Syft found no %s packages in bootc package database", provider.packageType)
}

func setSyftDistro(path string, distro syftDistro) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Syft SBOM for distro metadata: %w", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode Syft SBOM for distro metadata: %w", err)
	}
	document["distro"] = distro
	data, err = json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode Syft SBOM distro metadata: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write Syft SBOM distro metadata: %w", err)
	}
	return nil
}
