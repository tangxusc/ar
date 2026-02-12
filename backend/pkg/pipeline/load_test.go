package pipeline

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeArchiveToTar_PlainTarWithTarGzSuffix(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "pipeline-alpine.tar.gz")
	writeTarArchive(t, archivePath)

	tarPath, cleanup, err := normalizeArchiveToTar(archivePath)
	if err != nil {
		t.Fatalf("normalizeArchiveToTar returned error: %v", err)
	}
	t.Cleanup(cleanup)

	if tarPath != archivePath {
		t.Fatalf("expected original path for plain tar, got %q", tarPath)
	}
}

func TestNormalizeArchiveToTar_RealTarGz(t *testing.T) {
	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "pipeline-alpine.tar.gz")
	writeTarGzArchive(t, archivePath)

	tarPath, cleanup, err := normalizeArchiveToTar(archivePath)
	if err != nil {
		t.Fatalf("normalizeArchiveToTar returned error: %v", err)
	}

	if tarPath == archivePath {
		t.Fatalf("expected temporary tar path for real gzip archive")
	}

	if _, err := os.Stat(tarPath); err != nil {
		t.Fatalf("expected temp tar to exist, stat error: %v", err)
	}

	cleanup()

	if _, err := os.Stat(tarPath); !os.IsNotExist(err) {
		t.Fatalf("expected temp tar removed after cleanup, stat error: %v", err)
	}
}

func writeTarArchive(t *testing.T, path string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar file failed: %v", err)
	}
	defer file.Close()

	tw := tar.NewWriter(file)
	defer tw.Close()

	content := []byte("hello")
	hdr := &tar.Header{
		Name: "hello.txt",
		Mode: 0600,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header failed: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body failed: %v", err)
	}
}

func writeTarGzArchive(t *testing.T, path string) {
	t.Helper()

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create tar.gz file failed: %v", err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	content := []byte("hello")
	hdr := &tar.Header{
		Name: "hello.txt",
		Mode: 0600,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header failed: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body failed: %v", err)
	}
}
