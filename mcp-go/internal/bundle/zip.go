package bundle

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"time"
)

type BundleArtifact struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	FileCount int    `json:"fileCount"`
}

func CreateZipBundle(outputPath string, files []File) (BundleArtifact, error) {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return BundleArtifact{}, err
	}
	output, err := os.Create(outputPath)
	if err != nil {
		return BundleArtifact{}, err
	}
	zipWriter := zip.NewWriter(output)
	for _, file := range files {
		if err := addZipFile(zipWriter, file); err != nil {
			_ = zipWriter.Close()
			_ = output.Close()
			return BundleArtifact{}, err
		}
	}
	if err := zipWriter.Close(); err != nil {
		_ = output.Close()
		return BundleArtifact{}, err
	}
	if err := output.Close(); err != nil {
		return BundleArtifact{}, err
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return BundleArtifact{}, err
	}
	return BundleArtifact{
		Path:      outputPath,
		Kind:      "zip",
		MimeType:  "application/zip",
		SizeBytes: info.Size(),
		FileCount: len(files),
	}, nil
}

func CreateTextBundle(outputPath string, prompt string, files []File) (BundleArtifact, error) {
	rendered, err := RenderPromptBundle(prompt, files)
	if err != nil {
		return BundleArtifact{}, err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return BundleArtifact{}, err
	}
	if err := os.WriteFile(outputPath, []byte(rendered.Text), 0o644); err != nil {
		return BundleArtifact{}, err
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return BundleArtifact{}, err
	}
	return BundleArtifact{
		Path:      outputPath,
		Kind:      "text",
		MimeType:  "text/markdown",
		SizeBytes: info.Size(),
		FileCount: len(files),
	}, nil
}

func addZipFile(zipWriter *zip.Writer, file File) error {
	header := &zip.FileHeader{
		Name:     filepath.ToSlash(file.RelativePath),
		Method:   zip.Deflate,
		Modified: time.Now().UTC(),
	}
	if file.hasContent {
		if !file.modTime.IsZero() {
			header.Modified = file.modTime
		}
		header.SetMode(file.mode)
		entry, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, bytes.NewReader(file.content))
		return err
	}
	info, err := os.Stat(file.Path)
	if err != nil {
		return err
	}
	if !info.ModTime().IsZero() {
		header.Modified = info.ModTime()
	}
	header.SetMode(info.Mode())
	entry, err := zipWriter.CreateHeader(header)
	if err != nil {
		return err
	}
	input, err := os.Open(file.Path)
	if err != nil {
		return err
	}
	defer input.Close()
	_, err = io.Copy(entry, input)
	return err
}
