package main

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// These Dakota layers contain native executables with stable NVD CPE identities.
var buildStreamNativeComponents = []string{"python3.bst", "util-linux.bst", "ffmpeg.bst"}

func isBuildStreamImage(history []imageHistory) bool {
	for _, entry := range history {
		if strings.EqualFold(strings.TrimSpace(entry.CreatedBy), "chunkah") && strings.Contains(entry.Comment, "xattr/components/") {
			return true
		}
	}
	return false
}

func buildStreamSBOM(ctx context.Context, workDir string, reg *registry, repo string, layers []descriptor, history []imageHistory) (string, error) {
	candidates := buildStreamLayerCandidates(layers, history)
	if len(candidates) != len(buildStreamNativeComponents) {
		return "", fmt.Errorf("BuildStream inventory has %d of %d required component layers", len(candidates), len(buildStreamNativeComponents))
	}
	root := filepath.Join(workDir, "buildstream-root")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create BuildStream root: %w", err)
	}
	for _, component := range buildStreamNativeComponents {
		layer := candidates[component]
		layerPath := filepath.Join(workDir, component+".layer")
		if err := downloadBlob(ctx, reg, repo, layer.Digest, layerPath); err != nil {
			return "", fmt.Errorf("download BuildStream component %s: %w", component, err)
		}
		if err := extractZstdLayer(ctx, layerPath, root); err != nil {
			return "", fmt.Errorf("extract BuildStream component %s: %w", component, err)
		}
	}
	return runSyftBuildStream(ctx, workDir, root)
}

func buildStreamLayerCandidates(layers []descriptor, history []imageHistory) map[string]descriptor {
	candidates := make(map[string]descriptor)
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
		for _, component := range buildStreamNativeComponents {
			if strings.Contains(entry.Comment, "components/"+component) {
				candidates[component] = layer
			}
		}
	}
	return candidates
}

func extractZstdLayer(ctx context.Context, layerPath, root string) error {
	cmd := exec.CommandContext(ctx, "zstd", "-q", "-d", "-c", layerPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open zstd output: %w", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start zstd: %w", err)
	}
	extractErr := extractTar(tar.NewReader(stdout), root)
	if extractErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return extractErr
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		return fmt.Errorf("decompress zstd layer: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func extractTar(tr *tar.Reader, root string) error {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		path, err := archiveDestination(root, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, os.FileMode(header.Mode)&0o777); err != nil {
				return fmt.Errorf("create tar directory: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create tar parent: %w", err)
			}
			if err := writeTarEntry(path, os.FileMode(header.Mode)&0o777, tr); err != nil {
				return err
			}
		}
	}
}

func archiveDestination(root, name string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(name))
	if filepath.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("tar path %q escapes extraction root", name)
	}
	destination := filepath.Join(root, filepath.FromSlash(clean))
	relative, err := filepath.Rel(root, destination)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("tar path %q escapes extraction root", name)
	}
	return destination, nil
}

func writeTarEntry(path string, mode os.FileMode, reader io.Reader) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("create tar file: %w", err)
	}
	_, copyErr := io.Copy(f, reader)
	closeErr := f.Close()
	return errors.Join(copyErr, closeErr)
}

func runSyftBuildStream(ctx context.Context, workDir, root string) (string, error) {
	out := filepath.Join(workDir, "sbom.buildstream.syft.json")
	args := []string{"scan", "dir:" + root, "-q", "-o", "syft-json=" + out}
	cmd := exec.CommandContext(ctx, "syft", args...)
	cmd.Env = scannerEnv(workDir, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("catalog BuildStream components with Syft: %w: %s", err, strings.TrimSpace(string(output)))
	}
	usable, err := hasUsableInventory(out)
	if err != nil {
		return "", fmt.Errorf("validate BuildStream SBOM: %w", err)
	}
	if !usable {
		return "", errors.New("Syft found no native binaries in BuildStream components")
	}
	return out, nil
}
