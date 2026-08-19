package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const bootcLabel = "containers.bootc"

var (
	fedoraVersionPattern       = regexp.MustCompile(`(?:^|[^[:alnum:]])fc([0-9]+)(?:[^[:alnum:]]|$)`)
	centOSStreamVersionPattern = regexp.MustCompile(`(?i)(?:^|[^[:alnum:]])stream[-_ ]*([0-9]+)(?:[^[:alnum:]]|$)`)
)

type sbomDocument struct {
	Artifacts []struct {
		Type string            `json:"type"`
		CPEs []json.RawMessage `json:"cpes"`
	} `json:"artifacts"`
	Packages []struct {
		ExternalRefs []struct {
			ReferenceType    string `json:"referenceType"`
			ReferenceLocator string `json:"referenceLocator"`
		} `json:"externalRefs"`
	} `json:"packages"`
	Components []struct {
		PURL string `json:"purl"`
	} `json:"components"`
}

type syftDistro struct {
	PrettyName string   `json:"prettyName"`
	Name       string   `json:"name"`
	ID         string   `json:"id"`
	VersionID  string   `json:"versionID"`
	IDLike     []string `json:"idLike"`
	CPEName    string   `json:"cpeName"`
	BuildID    string   `json:"buildID,omitempty"`
}

var nativePackageTypes = map[string]struct{}{
	"apk": {}, "alpm": {}, "binary": {}, "deb": {}, "nix": {}, packageTypeRPM: {},
}

var nativePURLPrefixes = []string{"pkg:apk/", "pkg:alpm/", "pkg:deb/", "pkg:nix/", "pkg:rpm/"}

func hasUsableInventory(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var document sbomDocument
	if err := json.NewDecoder(f).Decode(&document); err != nil {
		return false, err
	}
	for _, artifact := range document.Artifacts {
		if _, ok := nativePackageTypes[artifact.Type]; ok && (artifact.Type != "binary" || len(artifact.CPEs) > 0) {
			return true, nil
		}
	}
	for _, pkg := range document.Packages {
		for _, ref := range pkg.ExternalRefs {
			if hasNativePURL(ref.ReferenceLocator) {
				return true, nil
			}
		}
	}
	for _, component := range document.Components {
		if hasNativePURL(component.PURL) {
			return true, nil
		}
	}
	return false, nil
}

func hasNativePURL(purl string) bool {
	purl = strings.ToLower(purl)
	for _, prefix := range nativePURLPrefixes {
		if strings.HasPrefix(purl, prefix) {
			return true
		}
	}
	return false
}

func bootcInventorySBOM(ctx context.Context, workDir string, reg *registry, art *artifact) (string, bool, error) {
	m, err := fetchManifest(ctx, reg, art.Repository, art.Digest)
	if err != nil {
		return "", false, fmt.Errorf("fetch image manifest: %w", err)
	}
	configBlob, err := fetchBlob(ctx, reg, art.Repository, m.Config.Digest)
	if err != nil {
		return "", false, fmt.Errorf("fetch image config: %w", err)
	}
	var config imageConfig
	if err := json.Unmarshal(configBlob, &config); err != nil {
		return "", false, fmt.Errorf("decode image config: %w", err)
	}
	if config.Config.Labels[bootcLabel] != "1" {
		return "", false, nil
	}
	if isBuildStreamImage(config.History) {
		sbom, err := buildStreamSBOM(ctx, workDir, reg, art.Repository, m.Layers, config.History)
		return sbom, true, err
	}

	candidates := rpmDBLayerCandidates(m.Layers, config.History)
	var candidateErrors []error
	for _, layer := range candidates {
		rpmDB, err := cachedRPMDB(ctx, workDir, reg, art.Repository, layer)
		if err != nil {
			candidateErrors = append(candidateErrors, fmt.Errorf("layer %s: %w", layer.Digest, err))
			continue
		}
		sbomPath, err := runSyftRPMDB(ctx, workDir, rpmDB, rpmDistro(config))
		if err != nil {
			return "", true, err
		}
		return sbomPath, true, nil
	}

	sbomPath, err := packageDBSBOM(ctx, workDir, reg, art.Repository, m.Layers, rpmDistro(config))
	if err == nil {
		return sbomPath, true, nil
	}
	candidateErrors = append(candidateErrors, err)
	return "", true, fmt.Errorf("discover bootc package inventory from image content: %w", errors.Join(candidateErrors...))
}

func fedoraVersion(config imageConfig) (string, error) {
	values := []string{
		config.Config.Labels["ostree.linux"],
		config.Config.Labels["org.opencontainers.image.version"],
	}
	for _, history := range config.History {
		values = append(values, history.CreatedBy)
	}
	for _, value := range values {
		if match := fedoraVersionPattern.FindStringSubmatch(value); len(match) == 2 {
			return match[1], nil
		}
	}
	return "", errors.New("bootc RPM inventory has no Fedora version metadata")
}

func rpmDistro(config imageConfig) syftDistro {
	labels := config.Config.Labels
	if id, version := labels["redhat.id"], labels["redhat.version-id"]; id != "" && version != "" {
		distro := syftDistro{
			PrettyName: id + " " + version,
			Name:       id,
			ID:         id,
			VersionID:  version,
		}
		if id == "almalinux" {
			distro.PrettyName = "AlmaLinux " + version
			distro.Name = "AlmaLinux"
			distro.IDLike = []string{"rhel", "centos", "fedora"}
			distro.CPEName = "cpe:/o:almalinux:almalinux:" + version
		}
		return distro
	}

	if match := centOSStreamVersionPattern.FindStringSubmatch(labels["org.opencontainers.image.version"]); len(match) == 2 {
		version := match[1]
		return syftDistro{
			PrettyName: "CentOS Stream " + version,
			Name:       "CentOS Stream",
			ID:         "centos",
			VersionID:  version,
			IDLike:     []string{"rhel", "fedora"},
			CPEName:    "cpe:/o:centos:centos:" + version,
		}
	}

	version, err := fedoraVersion(config)
	if err != nil {
		return syftDistro{}
	}
	return syftDistro{
		PrettyName: "Fedora Linux " + version,
		Name:       "Fedora Linux",
		ID:         "fedora",
		VersionID:  version,
		IDLike:     []string{"fedora"},
		CPEName:    "cpe:/o:fedoraproject:fedora:" + version,
	}
}

func rpmDBLayerCandidates(layers []descriptor, history []imageHistory) []descriptor {
	var candidates []descriptor
	layerIndex := 0
	for _, entry := range history {
		if entry.EmptyLayer {
			continue
		}
		if layerIndex >= len(layers) {
			break
		}
		layer := layers[layerIndex]
		layerIndex++
		createdBy := strings.ToLower(strings.TrimSpace(entry.CreatedBy))
		if strings.HasPrefix(createdBy, "rpm-") {
			candidates = append(candidates, layer)
		}
	}
	return candidates
}

func cachedRPMDB(ctx context.Context, workDir string, reg *registry, repo string, layer descriptor) (string, error) {
	cacheRoot := envOrDefault("SCANNER_GRYPE_CACHE_DIR", filepath.Join(workDir, "grype-cache"))
	cacheDir := filepath.Join(cacheRoot, "rpmdb")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("create RPM database cache: %w", err)
	}
	cachePath := filepath.Join(cacheDir, strings.TrimPrefix(layer.Digest, "sha256:")+".sqlite")
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		return cachePath, nil
	}

	layerPath := filepath.Join(workDir, "rpmdb-layer")
	if err := downloadBlob(ctx, reg, repo, layer.Digest, layerPath); err != nil {
		return "", err
	}
	temporary := cachePath + ".tmp"
	_ = os.Remove(temporary)
	if err := extractRPMDB(layerPath, temporary); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, cachePath); err != nil {
		return "", fmt.Errorf("cache RPM database: %w", err)
	}
	return cachePath, nil
}

func downloadBlob(ctx context.Context, reg *registry, repo, digest, path string) error {
	req, err := newRegistryRequest(ctx, http.MethodGet, reg, repo, "blobs", digest, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch layer blob: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError("fetch layer blob", resp)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create layer file: %w", err)
	}
	_, copyErr := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("download layer blob: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close layer file: %w", closeErr)
	}
	return nil
}

func extractRPMDB(layerPath, outPath string) error {
	target, direct, err := locateRPMDB(layerPath)
	if err != nil {
		return err
	}

	tr, closeArchive, err := openLayerTar(layerPath)
	if err != nil {
		return err
	}
	defer closeArchive()
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read layer tar: %w", err)
		}
		name := cleanTarPath(header.Name)
		if name != target || header.Typeflag != tar.TypeReg {
			continue
		}
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("create extracted RPM database: %w", err)
		}
		_, copyErr := io.Copy(f, tr)
		closeErr := f.Close()
		if copyErr != nil {
			return fmt.Errorf("extract RPM database: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close extracted RPM database: %w", closeErr)
		}
		return nil
	}
	if direct {
		return errors.New("RPM database tar entry has no regular file payload")
	}
	return fmt.Errorf("RPM database hardlink target %q not found", target)
}

func locateRPMDB(layerPath string) (target string, direct bool, err error) {
	tr, closeArchive, err := openLayerTar(layerPath)
	if err != nil {
		return "", false, err
	}
	defer closeArchive()
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", false, fmt.Errorf("read layer tar: %w", err)
		}
		if !isRPMDBPath(cleanTarPath(header.Name)) {
			continue
		}
		switch header.Typeflag {
		case tar.TypeReg:
			return cleanTarPath(header.Name), true, nil
		case tar.TypeLink:
			return cleanTarPath(header.Linkname), false, nil
		}
	}
	return "", false, errors.New("layer has no RPM database")
}

func openLayerTar(path string) (*tar.Reader, func() error, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open layer: %w", err)
	}
	magic := make([]byte, 4)
	if _, err := io.ReadFull(f, magic); err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("read layer header: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("rewind layer: %w", err)
	}
	if magic[0] == 0x28 && magic[1] == 0xb5 && magic[2] == 0x2f && magic[3] == 0xfd {
		if err := f.Close(); err != nil {
			return nil, nil, fmt.Errorf("close zstd layer: %w", err)
		}
		cmd := exec.Command("zstd", "-q", "-d", "-c", path)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return nil, nil, fmt.Errorf("open zstd layer output: %w", err)
		}
		if err := cmd.Start(); err != nil {
			return nil, nil, fmt.Errorf("start zstd layer decompression: %w", err)
		}
		return tar.NewReader(stdout), func() error {
			closeErr := stdout.Close()
			waitErr := cmd.Wait()
			return errors.Join(closeErr, waitErr)
		}, nil
	}
	if magic[0] != 0x1f || magic[1] != 0x8b {
		return tar.NewReader(f), f.Close, nil
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("open gzip layer: %w", err)
	}
	return tar.NewReader(gz), func() error {
		gzErr := gz.Close()
		fileErr := f.Close()
		return errors.Join(gzErr, fileErr)
	}, nil
}

func cleanTarPath(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+path)), "/")
}

func isRPMDBPath(path string) bool {
	switch path {
	case "var/lib/rpm/rpmdb.sqlite", "usr/share/rpm/rpmdb.sqlite", "usr/lib/sysimage/rpm/rpmdb.sqlite":
		return true
	default:
		return false
	}
}

func runSyftRPMDB(ctx context.Context, workDir, rpmDBPath string, distro syftDistro) (string, error) {
	out := filepath.Join(workDir, "sbom.rpm.syft.json")
	rpmRoot := filepath.Join(workDir, "rpm-root")
	canonicalDB := filepath.Join(rpmRoot, "usr", "share", "rpm", "rpmdb.sqlite")
	if err := os.MkdirAll(filepath.Dir(canonicalDB), 0o700); err != nil {
		return "", fmt.Errorf("create RPM database root: %w", err)
	}
	if err := os.Link(rpmDBPath, canonicalDB); err != nil {
		if err := copyFile(rpmDBPath, canonicalDB); err != nil {
			return "", fmt.Errorf("materialize RPM database: %w", err)
		}
	}
	args := []string{
		"scan", "dir:" + rpmRoot, "-q", "-o", "syft-json=" + out,
		"--override-default-catalogers", "rpm-db-cataloger",
	}
	cmd := exec.CommandContext(ctx, "syft", args...)
	cmd.Env = scannerEnv(workDir, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("catalog RPM database with Syft: %w: %s", err, strings.TrimSpace(string(output)))
	}

	data, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("read RPM SBOM: %w", err)
	}
	var inventory sbomDocument
	if err := json.Unmarshal(data, &inventory); err != nil {
		return "", fmt.Errorf("decode RPM SBOM: %w", err)
	}
	rpmCount := 0
	for _, artifact := range inventory.Artifacts {
		if artifact.Type == packageTypeRPM {
			rpmCount++
		}
	}
	if rpmCount == 0 {
		return "", errors.New("Syft found no RPM packages in bootc RPM database")
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return "", fmt.Errorf("decode RPM SBOM document: %w", err)
	}
	if distro.ID != "" {
		document["distro"] = distro
	}
	data, err = json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("encode RPM SBOM: %w", err)
	}
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return "", fmt.Errorf("write RPM SBOM: %w", err)
	}
	return out, nil
}

func copyFile(source, destination string) error {
	src, err := os.Open(source)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	return errors.Join(copyErr, closeErr)
}

func isNativePackageType(packageType string) bool {
	_, ok := nativePackageTypes[packageType]
	return ok
}
