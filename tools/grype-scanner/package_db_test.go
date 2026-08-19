package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExtractPackageDBInventoryDetectsArchSysimage(t *testing.T) {
	layer := filepath.Join(t.TempDir(), "layer.tar.gz")
	f, err := os.Create(layer)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	writeTarFile(t, tw, "etc/os-release", []byte("NAME=\"Arch Linux\"\nID=arch\nBUILD_ID=rolling\n"))
	writeTarFile(t, tw, "usr/lib/sysimage/lib/pacman/local/bash-5.3.3-1/desc", []byte("%NAME%\nbash\n\n%VERSION%\n5.3.3-1\n"))
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "root")
	provider, packages, err := extractPackageDBInventory(layer, root)
	if err != nil {
		t.Fatalf("extractPackageDBInventory: %v", err)
	}
	if provider == nil || provider.packageType != "alpm" || packages != 1 {
		t.Fatalf("provider = %#v, packages = %d", provider, packages)
	}
	for _, path := range []string{
		filepath.Join(root, "etc", "os-release"),
		filepath.Join(root, "var", "lib", "pacman", "local", "bash-5.3.3-1", "desc"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected extracted path %s: %v", path, err)
		}
	}
}

func TestExtractPackageDBInventoryFromZstdLayer(t *testing.T) {
	if _, err := exec.LookPath("zstd"); err != nil {
		t.Skip("zstd not installed")
	}
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "layer.tar")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	writeTarFile(t, tw, "lib/apk/db/installed", []byte("C:Q1\nP:alpine-baselayout\nV:3.7.1-r0\n"))
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	layer := filepath.Join(dir, "layer.tar.zst")
	if output, err := exec.Command("zstd", "-q", "-o", layer, tarPath).CombinedOutput(); err != nil {
		t.Fatalf("compress fixture: %v: %s", err, output)
	}

	provider, packages, err := extractPackageDBInventory(layer, filepath.Join(dir, "root"))
	if err != nil {
		t.Fatalf("extractPackageDBInventory: %v", err)
	}
	if provider == nil || provider.packageType != "apk" || packages != 1 {
		t.Fatalf("provider = %#v, packages = %d", provider, packages)
	}
}

func TestExtractPackageDBInventoryDetectsRPM(t *testing.T) {
	layer := filepath.Join(t.TempDir(), "layer.tar.gz")
	f, err := os.Create(layer)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	target := "sysroot/ostree/repo/objects/26/26e7fc0ee438417d637e7654be0f19bb2117bb48aa4ba23333f608d691c357.file"
	writeTarFile(t, tw, target, []byte("sqlite-data"))
	if err := tw.WriteHeader(&tar.Header{
		Name:     "usr/share/rpm/rpmdb.sqlite",
		Linkname: target,
		Typeflag: tar.TypeLink,
	}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "root")
	provider, packages, err := extractPackageDBInventory(layer, root)
	if err != nil {
		t.Fatalf("extractPackageDBInventory: %v", err)
	}
	if provider == nil || provider.packageType != "rpm" || packages != 1 {
		t.Fatalf("provider = %#v, packages = %d", provider, packages)
	}
	if _, err := os.Stat(filepath.Join(root, "usr", "share", "rpm", "rpmdb.sqlite")); err != nil {
		t.Fatalf("expected extracted RPM database: %v", err)
	}
}

func TestPackageDBProviderUsesContentPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "legacy Arch sysimage", path: "usr/lib/sysimage/lib/pacman/local/bash/desc", want: "alpm"},
		{name: "canonical Arch sysimage", path: "usr/lib/sysimage/var/lib/pacman/local/bash/desc", want: "alpm"},
		{name: "Debian", path: "var/lib/dpkg/status", want: "deb"},
		{name: "Alpine", path: "lib/apk/db/installed", want: "apk"},
		{name: "Nix", path: "nix/var/nix/db/db.sqlite", want: "nix"},
		{name: "RPM", path: "usr/share/rpm/rpmdb.sqlite", want: "rpm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, _, ok := packageDBProviderForPath(tt.path)
			if !ok || provider.packageType != tt.want {
				t.Fatalf("packageDBProviderForPath(%q) = %#v, %v", tt.path, provider, ok)
			}
		})
	}
}
