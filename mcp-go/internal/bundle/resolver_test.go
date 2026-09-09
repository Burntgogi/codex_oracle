package bundle_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"codex_oracle/internal/bundle"
)

func TestResolverIncludesSafeFilesAndSkipsSecrets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "plan.md"), "plan")
	writeFile(t, filepath.Join(root, ".env"), "OPENAI_API_KEY=secret")
	writeFile(t, filepath.Join(root, "node_modules", "ignored.js"), "ignored")

	result, err := bundle.Resolve(root, []string{"."}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	paths := result.RelativePaths()
	if !contains(paths, "docs/plan.md") {
		t.Fatalf("resolved paths = %v, want docs/plan.md", paths)
	}
	if contains(paths, ".env") {
		t.Fatalf("resolved paths = %v, did not want .env", paths)
	}
	if contains(paths, "node_modules/ignored.js") {
		t.Fatalf("resolved paths = %v, did not want node_modules file", paths)
	}
}

func TestResolverHonorsExcludes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main")
	writeFile(t, filepath.Join(root, "src", "main_test.go"), "package main")

	result, err := bundle.Resolve(root, []string{"src", "!src/*_test.go"}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	paths := result.RelativePaths()
	if !contains(paths, "src/main.go") {
		t.Fatalf("resolved paths = %v, want src/main.go", paths)
	}
	if contains(paths, "src/main_test.go") {
		t.Fatalf("resolved paths = %v, did not want src/main_test.go", paths)
	}
}

func TestResolverRejectsSameVolumeOutsideDirectoryBeforeTraversal(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	outsideDescendant := "outside-descendant.md"
	writeFile(t, filepath.Join(outsideRoot, "nested", outsideDescendant), "outside")

	result, err := bundle.Resolve(root, []string{outsideRoot}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	assertRejectedWithoutDescendantLeak(t, result, outsideDescendant)
}

func TestResolverRejectsDotDotOutsideDirectoryBeforeTraversal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	outsideRoot := filepath.Join(parent, "outside")
	outsideDescendant := "dotdot-descendant.md"
	writeFile(t, filepath.Join(root, "inside.md"), "inside")
	writeFile(t, filepath.Join(outsideRoot, "nested", outsideDescendant), "outside")

	result, err := bundle.Resolve(root, []string{filepath.Join("..", "outside")}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	assertRejectedWithoutDescendantLeak(t, result, outsideDescendant)
}

func TestResolverRejectsOutsideGlobBaseBeforeTraversal(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	outsideDescendant := "glob-descendant.md"
	writeFile(t, filepath.Join(outsideRoot, "nested", outsideDescendant), "outside")

	result, err := bundle.Resolve(root, []string{filepath.Join(outsideRoot, "**", "*.md")}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	assertRejectedWithoutDescendantLeak(t, result, outsideDescendant)
}

func TestResolverRejectsSymlinkDirectoryTargetingOutsideRootBeforeTraversal(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	outsideDescendant := "symlink-descendant.md"
	writeFile(t, filepath.Join(outsideRoot, "nested", outsideDescendant), "outside")
	linkPath := filepath.Join(root, "linked-outside")
	if err := os.Symlink(outsideRoot, linkPath); err != nil {
		t.Skipf("platform cannot create directory symlinks: %v", err)
	}

	result, err := bundle.Resolve(root, []string{"linked-outside"}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	assertRejectedWithoutDescendantLeak(t, result, outsideDescendant)
}

func TestResolverPreservesInRootDirectoryAndGlobControls(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "guide.md"), "guide")
	writeFile(t, filepath.Join(root, "src", "main.go"), "package main")
	writeFile(t, filepath.Join(root, "src", "main_test.go"), "package main")

	result, err := bundle.Resolve(root, []string{"docs", "src/*.go", "!src/*_test.go"}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	paths := result.RelativePaths()
	if !contains(paths, "docs/guide.md") || !contains(paths, "src/main.go") {
		t.Fatalf("resolved paths = %#v, want in-root directory and glob files", paths)
	}
	if contains(paths, "src/main_test.go") {
		t.Fatalf("resolved paths = %#v, did not want excluded glob file", paths)
	}
}

func TestResolverFailsClosedAcrossWindowsVolumes(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cross-volume filepath.Rel behavior is Windows-specific")
	}

	root := t.TempDir()
	rootVolume := filepath.VolumeName(root)
	var otherRoot string
	for drive := 'A'; drive <= 'Z'; drive++ {
		volumeRoot := string(drive) + `:\`
		if strings.EqualFold(filepath.VolumeName(volumeRoot), rootVolume) {
			continue
		}
		if info, err := os.Stat(volumeRoot); err != nil || !info.IsDir() {
			continue
		}
		candidate, err := os.MkdirTemp(volumeRoot, "codex-oracle-rel-")
		if err == nil {
			otherRoot = candidate
			break
		}
	}
	if otherRoot == "" {
		t.Skip("no writable second Windows volume is available")
	}
	t.Cleanup(func() { _ = os.RemoveAll(otherRoot) })

	outsideDescendant := "cross-volume-descendant.md"
	writeFile(t, filepath.Join(otherRoot, "nested", outsideDescendant), "outside")
	result, err := bundle.Resolve(root, []string{otherRoot}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	assertRejectedWithoutDescendantLeak(t, result, outsideDescendant)
}

func TestResolverRejectsSymlinkFileTargetingOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outsideRoot := t.TempDir()
	writeFile(t, filepath.Join(root, "docs", "plan.md"), "plan")
	outsideFile := filepath.Join(outsideRoot, "outside.md")
	writeFile(t, outsideFile, "outside")
	symlinkPath := filepath.Join(root, "escape.md")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Skipf("platform cannot create file symlinks: %v", err)
	}

	result, err := bundle.Resolve(root, []string{"."}, bundle.Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if !contains(result.RelativePaths(), "docs/plan.md") {
		t.Fatalf("resolved paths = %#v, want regular in-root file", result.RelativePaths())
	}
	if contains(result.RelativePaths(), "escape.md") {
		t.Fatalf("resolved paths = %#v, did not want symlink escape", result.RelativePaths())
	}
	if !ignoredForReason(result.Ignored, "escape.md", "outside root") {
		t.Fatalf("ignored = %#v, want canonical outside-root rejection", result.Ignored)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func ignoredForReason(ignored []bundle.IgnoredFile, pathSuffix string, reason string) bool {
	for _, item := range ignored {
		if strings.HasSuffix(filepath.ToSlash(item.Path), pathSuffix) && strings.Contains(item.Reason, reason) {
			return true
		}
	}
	return false
}

func assertRejectedWithoutDescendantLeak(t *testing.T, result bundle.Result, descendant string) {
	t.Helper()
	if len(result.Files) != 0 {
		t.Fatalf("resolved files = %#v, want none", result.Files)
	}
	if len(result.Ignored) != 1 || !strings.Contains(result.Ignored[0].Reason, "outside root") {
		t.Fatalf("ignored = %#v, want one outside-root rejection", result.Ignored)
	}
	for _, file := range result.Files {
		if strings.Contains(file.Path, descendant) || strings.Contains(file.RelativePath, descendant) {
			t.Fatalf("files leaked outside descendant %q: %#v", descendant, result.Files)
		}
	}
	for _, ignored := range result.Ignored {
		if strings.Contains(ignored.Pattern, descendant) || strings.Contains(ignored.Path, descendant) || strings.Contains(ignored.Reason, descendant) {
			t.Fatalf("ignored leaked outside descendant %q: %#v", descendant, result.Ignored)
		}
	}
}
