package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildStreamLayerCandidatesAccountsForEmptyHistory(t *testing.T) {
	layers := []descriptor{{Digest: "sha256:python"}, {Digest: "sha256:util"}, {Digest: "sha256:ffmpeg"}}
	history := []imageHistory{
		{CreatedBy: "chunkah", Comment: "xattr/components/python3.bst"},
		{CreatedBy: "metadata", EmptyLayer: true},
		{CreatedBy: "chunkah", Comment: "xattr/components/util-linux.bst"},
		{CreatedBy: "chunkah", Comment: "xattr/components/ffmpeg.bst"},
	}
	got := buildStreamLayerCandidates(layers, history)
	if len(got) != 3 || got["python3.bst"].Digest != "sha256:python" || got["util-linux.bst"].Digest != "sha256:util" || got["ffmpeg.bst"].Digest != "sha256:ffmpeg" {
		t.Fatalf("buildStreamLayerCandidates = %#v", got)
	}
}

func TestArchiveDestinationRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := archiveDestination(root, "../../escape"); err == nil {
		t.Fatal("archiveDestination accepted traversal")
	}
}

func TestExtractTarDoesNotFollowArchiveSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: "usr", Typeflag: tar.TypeSymlink, Linkname: outside}); err != nil {
		t.Fatal(err)
	}
	data := []byte("tool")
	if err := tw.WriteHeader(&tar.Header{Name: "usr/bin/tool", Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractTar(tar.NewReader(&archive), root); err != nil {
		t.Fatalf("extractTar: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "usr", "bin", "tool")); err != nil {
		t.Fatalf("expected tool inside root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "bin", "tool")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped through symlink: %v", err)
	}
}
