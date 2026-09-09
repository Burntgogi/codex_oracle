package bundle_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex_oracle/internal/bundle"
)

func TestCreateZipBundlePreservesRelativePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main\n")
	binaryPath := filepath.Join(root, "assets", "image.bin")
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeBytes(t, binaryPath, []byte{0, 1, 2, 3})
	resolved, err := bundle.Resolve(root, []string{"src", "assets"}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "context.zip")

	artifact, err := bundle.CreateZipBundle(outputPath, resolved.Files)
	if err != nil {
		t.Fatalf("CreateZipBundle returned error: %v", err)
	}

	if artifact.Path != outputPath {
		t.Fatalf("artifact.Path = %q, want %q", artifact.Path, outputPath)
	}
	if artifact.FileCount != 2 {
		t.Fatalf("FileCount = %d, want 2", artifact.FileCount)
	}
	zipReader, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer zipReader.Close()
	names := map[string]bool{}
	for _, file := range zipReader.File {
		names[file.Name] = true
	}
	if !names["src/main.go"] {
		t.Fatalf("zip names = %#v, want src/main.go", names)
	}
	if !names["assets/image.bin"] {
		t.Fatalf("zip names = %#v, want assets/image.bin", names)
	}
}

func TestCreateTextBundleWritesRenderedBundle(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# Readme\n")
	resolved, err := bundle.Resolve(root, []string{"README.md"}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	outputPath := filepath.Join(t.TempDir(), "context.md")

	artifact, err := bundle.CreateTextBundle(outputPath, "Review", resolved.Files)
	if err != nil {
		t.Fatalf("CreateTextBundle returned error: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if artifact.MimeType != "text/markdown" {
		t.Fatalf("MimeType = %q, want text/markdown", artifact.MimeType)
	}
	if !strings.Contains(string(data), "# Oracle Consult Bundle") {
		t.Fatalf("bundle file did not contain rendered header:\n%s", data)
	}
}
