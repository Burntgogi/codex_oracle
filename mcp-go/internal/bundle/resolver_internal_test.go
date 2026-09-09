package bundle

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFileIfAllowedFailsClosedWhenRelativePathCannotBeComputed(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "plan.md")
	if err := os.WriteFile(filePath, []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	files, ignored, err := fileIfAllowedWithRel(
		root,
		filePath,
		nil,
		Options{MaxFileSizeBytes: 1024},
		func(string, string) (string, error) { return "", errors.New("synthetic Rel failure") },
	)
	if err != nil {
		t.Fatalf("fileIfAllowedWithRel returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("files = %#v, want none", files)
	}
	if len(ignored) != 1 || !strings.Contains(ignored[0].Reason, "containment") {
		t.Fatalf("ignored = %#v, want containment failure", ignored)
	}
}

func TestRejectSymlinkUsesLstatMode(t *testing.T) {
	ignored, err := rejectSymlink("escape.md", func(string) (os.FileInfo, error) {
		return staticFileInfo{name: "escape.md", mode: os.ModeSymlink}, nil
	})
	if err != nil {
		t.Fatalf("rejectSymlink returned error: %v", err)
	}
	if ignored == nil || !strings.Contains(ignored.Reason, "symbolic link") {
		t.Fatalf("ignored = %#v, want symbolic-link rejection", ignored)
	}
}

func TestCanonicalContainmentRejectsResolvedPathOutsideRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	filePath := filepath.Join(root, "linked", "outside.md")
	outsidePath := filepath.Join(t.TempDir(), "outside.md")

	ignored, err := canonicalContainment(
		root,
		filePath,
		func(path string) (string, error) {
			switch path {
			case root:
				return root, nil
			case filePath:
				return outsidePath, nil
			default:
				return path, nil
			}
		},
		filepath.Rel,
	)
	if err != nil {
		t.Fatalf("canonicalContainment returned error: %v", err)
	}
	if ignored == nil || ignored.Reason != "outside root" {
		t.Fatalf("ignored = %#v, want outside-root rejection", ignored)
	}
}

func TestPreWalkContainmentFailsClosedWhenRelativePathCannotBeComputed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	target := filepath.Join(root, "docs")

	ignored := preWalkContainment(
		root,
		target,
		func(path string) (string, error) { return path, nil },
		func(string, string) (string, error) { return "", errors.New("synthetic Rel failure") },
	)
	if ignored == nil || !strings.Contains(ignored.Reason, "containment check failed") {
		t.Fatalf("ignored = %#v, want fail-closed relative-path rejection", ignored)
	}
}

func TestPreWalkContainmentFailsClosedWhenCanonicalizationFails(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	target := filepath.Join(root, "docs")

	ignored := preWalkContainment(
		root,
		target,
		func(string) (string, error) { return "", errors.New("synthetic EvalSymlinks failure") },
		filepath.Rel,
	)
	if ignored == nil || !strings.Contains(ignored.Reason, "containment check failed") {
		t.Fatalf("ignored = %#v, want fail-closed canonicalization rejection", ignored)
	}
}

func TestResolveAnchorsTraversalWhenDirectoryIsSwappedAfterPreWalk(t *testing.T) {
	root := t.TempDir()
	selectedDir := filepath.Join(root, "selected")
	if err := os.MkdirAll(selectedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll selected directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(selectedDir, "safe.md"), []byte("safe"), 0o644); err != nil {
		t.Fatalf("WriteFile safe file: %v", err)
	}

	outsideRoot := t.TempDir()
	outsideDescendant := "swapped-outside-descendant.md"
	if err := os.MkdirAll(filepath.Join(outsideRoot, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll outside directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(outsideRoot, "nested", outsideDescendant), []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile outside file: %v", err)
	}

	swapped := false
	result, err := resolveWithHooks(root, []string{"selected"}, Options{MaxFileSizeBytes: 1024}, resolverHooks{
		afterPreWalk: func() {
			swapped = true
			if err := replaceDirectoryWithLink(selectedDir, outsideRoot); err != nil {
				t.Skipf("platform cannot create a directory link for swap test: %v", err)
			}
		},
		beforeWalk: func(string) {
			t.Fatal("resolver attempted to traverse the swapped directory")
		},
	})
	if err != nil {
		t.Fatalf("resolveWithHooks returned error: %v", err)
	}
	if !swapped {
		t.Fatal("swap hook was not invoked")
	}
	if len(result.Files) != 0 {
		t.Fatalf("files = %#v, want none after directory swap", result.Files)
	}
	if len(result.Ignored) != 1 {
		t.Fatalf("ignored = %#v, want one rejection after directory swap", result.Ignored)
	}
	for _, file := range result.Files {
		if strings.Contains(file.Path, outsideDescendant) || strings.Contains(file.RelativePath, outsideDescendant) {
			t.Fatalf("files leaked outside descendant %q: %#v", outsideDescendant, result.Files)
		}
	}
	for _, ignored := range result.Ignored {
		if strings.Contains(ignored.Pattern, outsideDescendant) || strings.Contains(ignored.Path, outsideDescendant) || strings.Contains(ignored.Reason, outsideDescendant) {
			t.Fatalf("ignored leaked outside descendant %q: %#v", outsideDescendant, result.Ignored)
		}
	}
}

func TestResolverPreservesInRootAbsoluteIntermediateDirectoryLink(t *testing.T) {
	testResolverPreservesInRootAbsoluteIntermediateDirectoryLink(t, "alias")
}

func TestResolverPreservesUnicodeInRootAbsoluteIntermediateDirectoryLink(t *testing.T) {
	testResolverPreservesInRootAbsoluteIntermediateDirectoryLink(t, "alias-\u012f")
}

func testResolverPreservesInRootAbsoluteIntermediateDirectoryLink(t *testing.T, aliasName string) {
	t.Helper()
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(realDir, "nested"), 0o755); err != nil {
		t.Fatalf("MkdirAll real directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "nested", "guide.md"), []byte("guide"), 0o644); err != nil {
		t.Fatalf("WriteFile guide: %v", err)
	}
	aliasDir := filepath.Join(root, aliasName)
	if err := replaceDirectoryWithLink(aliasDir, realDir); err != nil {
		t.Skipf("platform cannot create an absolute in-root directory link: %v", err)
	}
	testCases := []struct {
		name     string
		patterns []string
		want     []string
	}{
		{name: "exact file", patterns: []string{filepath.Join(aliasName, "plan.md")}, want: []string{slash(filepath.Join(aliasName, "plan.md"))}},
		{name: "directory", patterns: []string{aliasName}, want: []string{slash(filepath.Join(aliasName, "nested", "guide.md")), slash(filepath.Join(aliasName, "plan.md"))}},
		{name: "glob", patterns: []string{filepath.Join(aliasName, "**", "*.md")}, want: []string{slash(filepath.Join(aliasName, "nested", "guide.md")), slash(filepath.Join(aliasName, "plan.md"))}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := Resolve(root, testCase.patterns, Options{MaxFileSizeBytes: 1024})
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if len(result.Ignored) != 0 {
				t.Fatalf("ignored = %#v, want none", result.Ignored)
			}
			if !sameStrings(result.RelativePaths(), testCase.want) {
				t.Fatalf("resolved paths = %#v, want %#v", result.RelativePaths(), testCase.want)
			}
			for _, file := range result.Files {
				wantPath := filepath.Join(root, filepath.FromSlash(file.RelativePath))
				if file.Path != wantPath {
					t.Fatalf("file path = %q, want requested alias path %q", file.Path, wantPath)
				}
			}
		})
	}
}

func TestRenderPromptBundleUsesRootedSnapshotAfterDirectorySwap(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll docs directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "plan.md"), []byte("safe content"), 0o644); err != nil {
		t.Fatalf("WriteFile safe content: %v", err)
	}

	resolved, err := Resolve(root, []string{filepath.Join("docs", "plan.md")}, Options{MaxFileSizeBytes: 1024})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	outsideRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideRoot, "plan.md"), []byte("outside content"), 0o644); err != nil {
		t.Fatalf("WriteFile outside content: %v", err)
	}
	if err := replaceDirectoryWithLink(docsDir, outsideRoot); err != nil {
		t.Skipf("platform cannot create a directory link for renderer swap test: %v", err)
	}

	rendered, err := RenderPromptBundle("review", resolved.Files)
	if err != nil {
		t.Fatalf("RenderPromptBundle returned error: %v", err)
	}
	if !strings.Contains(rendered.Text, "safe content") {
		t.Fatalf("rendered bundle omitted rooted snapshot: %q", rendered.Text)
	}
	if strings.Contains(rendered.Text, "outside content") {
		t.Fatalf("rendered bundle read swapped outside content: %q", rendered.Text)
	}
	zipPath := filepath.Join(t.TempDir(), "context.zip")
	if _, err := CreateZipBundle(zipPath, resolved.Files); err != nil {
		t.Fatalf("CreateZipBundle returned error: %v", err)
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer reader.Close()
	if len(reader.File) != 1 {
		t.Fatalf("zip files = %#v, want one", reader.File)
	}
	entry, err := reader.File[0].Open()
	if err != nil {
		t.Fatalf("Open zip entry: %v", err)
	}
	content, err := io.ReadAll(entry)
	closeErr := entry.Close()
	if err != nil {
		t.Fatalf("Read zip entry: %v", err)
	}
	if closeErr != nil {
		t.Fatalf("Close zip entry: %v", closeErr)
	}
	if string(content) != "safe content" {
		t.Fatalf("zip content = %q, want rooted snapshot", content)
	}
}

func TestResolveRejectsFinalComponentSecretLinkSwapBeforeSnapshot(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.md")
	if err := os.WriteFile(planPath, []byte("safe plan"), 0o644); err != nil {
		t.Fatalf("WriteFile plan: %v", err)
	}
	secretPath := filepath.Join(root, ".env")
	if err := os.WriteFile(secretPath, []byte("TOP_SECRET"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	result, err := resolveWithHooks(root, []string{"plan.md"}, Options{MaxFileSizeBytes: 1024}, resolverHooks{
		beforeFileOpen: func(string) {
			if err := replaceFileWithLink(planPath, secretPath); err != nil {
				t.Skipf("platform cannot create an in-root file link: %v", err)
			}
		},
	})
	if err != nil {
		t.Fatalf("resolveWithHooks returned error: %v", err)
	}
	if len(result.Files) != 0 || len(result.Ignored) != 1 {
		t.Fatalf("result = %#v, want one rejection and no files", result)
	}
	for _, ignored := range result.Ignored {
		if strings.Contains(ignored.Pattern, ".env") || strings.Contains(ignored.Path, ".env") || strings.Contains(ignored.Reason, "TOP_SECRET") {
			t.Fatalf("ignored leaked secret link details: %#v", result.Ignored)
		}
	}
}

func TestResolveRejectsFinalComponentSecretLinkSwapAfterLstat(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.md")
	secretPath := filepath.Join(root, ".env")
	if err := os.WriteFile(planPath, []byte("safe content"), 0o644); err != nil {
		t.Fatalf("WriteFile plan: %v", err)
	}
	if err := os.WriteFile(secretPath, []byte("TOP_SECRET"), 0o600); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	result, err := resolveWithHooks(root, []string{"plan.md"}, Options{MaxFileSizeBytes: 1024}, resolverHooks{
		afterFileLstat: func(string) {
			if err := replaceFileWithLink(planPath, secretPath); err != nil {
				t.Fatalf("replace plan with secret link: %v", err)
			}
		},
	})
	if err != nil {
		t.Fatalf("resolveWithHooks returned error: %v", err)
	}
	if len(result.Files) != 0 || len(result.Ignored) != 1 {
		t.Fatalf("result = %#v, want one rejection and no files", result)
	}
	for _, ignored := range result.Ignored {
		if strings.Contains(ignored.Pattern, ".env") || strings.Contains(ignored.Path, ".env") || strings.Contains(ignored.Reason, "TOP_SECRET") {
			t.Fatalf("ignored leaked secret link details: %#v", result.Ignored)
		}
	}
}

func TestResolveBoundsSnapshotAfterFileGrowth(t *testing.T) {
	root := t.TempDir()
	planPath := filepath.Join(root, "plan.md")
	if err := os.WriteFile(planPath, []byte("small"), 0o644); err != nil {
		t.Fatalf("WriteFile plan: %v", err)
	}

	result, err := resolveWithHooks(root, []string{"plan.md"}, Options{MaxFileSizeBytes: 8}, resolverHooks{
		afterFileOpen: func() {
			file, err := os.OpenFile(planPath, os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatalf("OpenFile growing plan: %v", err)
			}
			if _, err := file.WriteString(strings.Repeat("x", 64)); err != nil {
				_ = file.Close()
				t.Fatalf("WriteString growing plan: %v", err)
			}
			if err := file.Close(); err != nil {
				t.Fatalf("Close growing plan: %v", err)
			}
		},
	})
	if err != nil {
		t.Fatalf("resolveWithHooks returned error: %v", err)
	}
	if len(result.Files) != 0 || len(result.Ignored) != 1 || result.Ignored[0].Reason != "file exceeds max size" {
		t.Fatalf("result = %#v, want bounded oversize rejection", result)
	}
}

func replaceDirectoryWithLink(linkPath string, target string) error {
	if err := os.RemoveAll(linkPath); err != nil {
		return err
	}
	if err := os.Symlink(target, linkPath); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	output, err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", linkPath, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func replaceFileWithLink(linkPath string, target string) error {
	if err := os.Remove(linkPath); err != nil {
		return err
	}
	if err := os.Symlink(target, linkPath); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	if err := os.Link(target, linkPath); err != nil {
		return fmt.Errorf("create hard link: %w", err)
	}
	return nil
}

func sameStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

type staticFileInfo struct {
	name string
	mode os.FileMode
}

func (info staticFileInfo) Name() string      { return info.name }
func (staticFileInfo) Size() int64            { return 0 }
func (info staticFileInfo) Mode() os.FileMode { return info.mode }
func (staticFileInfo) ModTime() time.Time     { return time.Time{} }
func (info staticFileInfo) IsDir() bool       { return info.mode.IsDir() }
func (staticFileInfo) Sys() any               { return nil }
