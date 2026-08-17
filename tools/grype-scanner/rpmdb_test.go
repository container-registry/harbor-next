package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestHasUsableInventory(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want bool
	}{
		{name: "syft", doc: `{"artifacts":[{"type":"rpm"}]}`, want: true},
		{name: "arch syft", doc: `{"artifacts":[{"type":"alpm"}]}`, want: true},
		{name: "debian syft", doc: `{"artifacts":[{"type":"deb"}]}`, want: true},
		{name: "alpine syft", doc: `{"artifacts":[{"type":"apk"}]}`, want: true},
		{name: "nix syft", doc: `{"artifacts":[{"type":"nix"}]}`, want: true},
		{name: "native binary", doc: `{"artifacts":[{"type":"binary","cpes":[{"cpe":"cpe:2.3:a:ffmpeg:ffmpeg:7.1.3:*:*:*:*:*:*:*"}]}]}`, want: true},
		{name: "unidentified binary", doc: `{"artifacts":[{"type":"binary"}]}`},
		{name: "spdx", doc: `{"packages":[{"externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:rpm/fedora/bash@5.2"}]}]}`, want: true},
		{name: "arch spdx", doc: `{"packages":[{"externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:alpm/arch/bash@5.2"}]}]}`, want: true},
		{name: "cyclonedx", doc: `{"components":[{"purl":"pkg:rpm/fedora/bash@5.2"}]}`, want: true},
		{name: "oci only", doc: `{"packages":[{"externalRefs":[{"referenceType":"purl","referenceLocator":"pkg:oci/image@sha256:abc"}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sbom.json")
			if err := os.WriteFile(path, []byte(tt.doc), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := hasUsableInventory(path)
			if err != nil {
				t.Fatalf("hasUsableInventory: %v", err)
			}
			if got != tt.want {
				t.Fatalf("hasUsableInventory = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNativePackageTypes(t *testing.T) {
	for _, packageType := range []string{"rpm", "alpm", "deb", "apk", "nix", "binary"} {
		t.Run(packageType, func(t *testing.T) {
			if !isNativePackageType(packageType) {
				t.Fatalf("isNativePackageType(%q) = false", packageType)
			}
		})
	}
	if isNativePackageType("go-module") {
		t.Fatal("Go modules must not be reported as native OS packages")
	}
}

func TestFedoraVersion(t *testing.T) {
	var config imageConfig
	config.Config.Labels = map[string]string{"ostree.linux": "7.0.9-ogc3.2.fc44.x86_64"}
	got, err := fedoraVersion(config)
	if err != nil {
		t.Fatalf("fedoraVersion: %v", err)
	}
	if got != "44" {
		t.Fatalf("fedoraVersion = %q, want 44", got)
	}
}

func TestRPMDistroFromRedHatLabels(t *testing.T) {
	var config imageConfig
	config.Config.Labels = map[string]string{
		"redhat.id":         "almalinux",
		"redhat.version-id": "10",
	}

	got := rpmDistro(config)
	if got.ID != "almalinux" || got.VersionID != "10" {
		t.Fatalf("rpmDistro = %#v, want AlmaLinux 10", got)
	}
}

func TestRPMDistroFromCentOSStreamVersion(t *testing.T) {
	var config imageConfig
	config.Config.Labels = map[string]string{
		"org.opencontainers.image.version": "stream10.1",
		"ostree.linux":                     "6.12.0-248.el10.x86_64",
	}

	got := rpmDistro(config)
	if got.ID != "centos" || got.VersionID != "10" || got.Name != "CentOS Stream" {
		t.Fatalf("rpmDistro = %#v, want CentOS Stream 10", got)
	}
}

func TestRPMDBLayerCandidatesAccountsForEmptyHistory(t *testing.T) {
	layers := []descriptor{{Digest: "sha256:base"}, {Digest: "sha256:rpm"}}
	history := []imageHistory{
		{CreatedBy: "base"},
		{CreatedBy: "metadata", EmptyLayer: true},
		{CreatedBy: "rpm-6.0.1-2.fc44.x86_64"},
	}
	got := rpmDBLayerCandidates(layers, history)
	if len(got) != 1 || got[0].Digest != "sha256:rpm" {
		t.Fatalf("rpmDBLayerCandidates = %#v", got)
	}
}

func TestRPMDBLayerCandidatesSupportsEnterpriseLinux(t *testing.T) {
	layers := []descriptor{{Digest: "sha256:base"}, {Digest: "sha256:rpm"}}
	history := []imageHistory{
		{CreatedBy: "base"},
		{CreatedBy: "rpm-4.19.1.1-25.el10.alma.1.aarch64"},
	}

	got := rpmDBLayerCandidates(layers, history)
	if len(got) != 1 || got[0].Digest != "sha256:rpm" {
		t.Fatalf("rpmDBLayerCandidates = %#v, want AlmaLinux RPM layer", got)
	}
}

func TestExtractRPMDBHardlink(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "layer.tar.gz")
	createRPMDBLayer(t, layer, true)
	out := filepath.Join(dir, "rpmdb.sqlite")
	if err := extractRPMDB(layer, out); err != nil {
		t.Fatalf("extractRPMDB: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sqlite-data" {
		t.Fatalf("extracted data = %q", data)
	}
}

func TestExtractRPMDBRegularFile(t *testing.T) {
	dir := t.TempDir()
	layer := filepath.Join(dir, "layer.tar.gz")
	createRPMDBLayer(t, layer, false)
	out := filepath.Join(dir, "rpmdb.sqlite")
	if err := extractRPMDB(layer, out); err != nil {
		t.Fatalf("extractRPMDB: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sqlite-data" {
		t.Fatalf("extracted data = %q", data)
	}
}

func createRPMDBLayer(t *testing.T, path string, hardlink bool) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	if hardlink {
		target := "sysroot/ostree/repo/objects/aa/database.file"
		writeTarFile(t, tw, target, []byte("sqlite-data"))
		if err := tw.WriteHeader(&tar.Header{
			Name:     "usr/share/rpm/rpmdb.sqlite",
			Typeflag: tar.TypeLink,
			Linkname: target,
			Mode:     0o644,
		}); err != nil {
			t.Fatal(err)
		}
	} else {
		writeTarFile(t, tw, "usr/lib/sysimage/rpm/rpmdb.sqlite", []byte("sqlite-data"))
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
}

func writeTarFile(t *testing.T, tw *tar.Writer, name string, data []byte) {
	t.Helper()
	if err := tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
}
