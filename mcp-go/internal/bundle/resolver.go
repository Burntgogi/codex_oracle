package bundle

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Options struct {
	MaxFileSizeBytes int64
}

type File struct {
	Path         string `json:"path"`
	RelativePath string `json:"relativePath"`
	SizeBytes    int64  `json:"sizeBytes"`
	content      []byte
	hasContent   bool
	mode         fs.FileMode
	modTime      time.Time
}

type IgnoredFile struct {
	Pattern string `json:"pattern,omitempty"`
	Path    string `json:"path,omitempty"`
	Reason  string `json:"reason"`
}

type Result struct {
	Files   []File        `json:"files"`
	Ignored []IgnoredFile `json:"ignored,omitempty"`
}

type resolverHooks struct {
	afterPreWalk   func()
	beforeWalk     func(string)
	afterFileLstat func(string)
	beforeFileOpen func(string)
	afterFileOpen  func()
}

type rootedTarget struct {
	requested string
	canonical string
	finalLink bool
}

func (r Result) RelativePaths() []string {
	paths := make([]string, 0, len(r.Files))
	for _, file := range r.Files {
		paths = append(paths, file.RelativePath)
	}
	return paths
}

func Resolve(root string, patterns []string, options Options) (Result, error) {
	return resolveWithHooks(root, patterns, options, resolverHooks{})
}

func resolveWithHooks(root string, patterns []string, options Options, hooks resolverHooks) (Result, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return Result{}, err
	}
	if len(patterns) == 0 {
		return Result{}, nil
	}
	rootHandle, err := os.OpenRoot(rootAbs)
	if err != nil {
		return Result{}, err
	}
	defer rootHandle.Close()

	includes, excludes := splitPatterns(patterns)
	result := Result{}
	seen := map[string]bool{}
	for _, include := range includes {
		matches, ignored, err := resolveInclude(rootAbs, rootHandle, include, excludes, options, hooks)
		result.Ignored = append(result.Ignored, ignored...)
		if err != nil {
			result.Ignored = append(result.Ignored, IgnoredFile{Pattern: include, Reason: err.Error()})
			continue
		}
		for _, match := range matches {
			if seen[match.RelativePath] {
				continue
			}
			seen[match.RelativePath] = true
			result.Files = append(result.Files, match)
		}
	}
	sort.Slice(result.Files, func(i, j int) bool {
		return result.Files[i].RelativePath < result.Files[j].RelativePath
	})
	return result, nil
}

func splitPatterns(patterns []string) ([]string, []string) {
	includes := []string{}
	excludes := []string{}
	for _, pattern := range patterns {
		trimmed := strings.TrimSpace(pattern)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "!") {
			excludes = append(excludes, strings.TrimPrefix(trimmed, "!"))
			continue
		}
		includes = append(includes, trimmed)
	}
	return includes, excludes
}

func resolveInclude(rootAbs string, rootHandle *os.Root, include string, excludes []string, options Options, hooks resolverHooks) ([]File, []IgnoredFile, error) {
	if hasGlob(include) {
		base := globBase(rootAbs, include)
		baseTarget, ignored := validateRootedTarget(rootAbs, base, canonicalizePath, filepath.Rel)
		if ignored != nil {
			return nil, nil, fmt.Errorf("%s", ignored.Reason)
		}
		if hooks.afterPreWalk != nil {
			hooks.afterPreWalk()
		}
		return walkGlob(rootAbs, rootHandle, include, baseTarget, excludes, options, hooks)
	}

	target := includeTarget(rootAbs, include)
	rootedTarget, ignored := validateRootedTarget(rootAbs, target, canonicalizePath, filepath.Rel)
	if ignored != nil {
		return nil, nil, fmt.Errorf("%s", ignored.Reason)
	}
	if hooks.afterPreWalk != nil {
		hooks.afterPreWalk()
	}
	info, err := rootHandle.Stat(rootedTarget.canonical)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return walkDir(rootAbs, rootHandle, rootedTarget.canonical, rootedTarget.requested, excludes, options, hooks)
	}
	return fileIfAllowedInRoot(rootAbs, rootHandle, rootedTarget.canonical, rootedTarget.requested, rootedTarget.finalLink, excludes, options, hooks)
}

func walkDir(rootAbs string, rootHandle *os.Root, canonicalDir string, requestedDir string, excludes []string, options Options, hooks resolverHooks) ([]File, []IgnoredFile, error) {
	files := []File{}
	ignored := []IgnoredFile{}
	if hooks.beforeWalk != nil {
		hooks.beforeWalk(canonicalDir)
	}
	err := fs.WalkDir(rootHandle.FS(), canonicalDir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			ignored = append(ignored, IgnoredFile{Path: requestedWalkPath(canonicalDir, requestedDir, current), Reason: err.Error()})
			return nil
		}
		if entry.IsDir() && shouldSkipDir(entry.Name()) && current != canonicalDir {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		requested := requestedWalkPath(canonicalDir, requestedDir, current)
		added, skipped, err := fileIfAllowedInRoot(rootAbs, rootHandle, slash(current), requested, false, excludes, options, hooks)
		if err != nil {
			ignored = append(ignored, IgnoredFile{Path: requested, Reason: err.Error()})
			return nil
		}
		files = append(files, added...)
		ignored = append(ignored, skipped...)
		return nil
	})
	return files, ignored, err
}

func walkGlob(rootAbs string, rootHandle *os.Root, pattern string, base rootedTarget, excludes []string, options Options, hooks resolverHooks) ([]File, []IgnoredFile, error) {
	canonicalBase := base.canonical
	requestedBase := base.requested
	info, err := rootHandle.Stat(canonicalBase)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()) {
		canonicalBase = "."
		requestedBase = "."
	} else if err != nil {
		return nil, nil, err
	}
	files := []File{}
	ignored := []IgnoredFile{}
	if hooks.beforeWalk != nil {
		hooks.beforeWalk(canonicalBase)
	}
	err = fs.WalkDir(rootHandle.FS(), canonicalBase, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			ignored = append(ignored, IgnoredFile{Path: requestedWalkPath(canonicalBase, requestedBase, current), Reason: err.Error()})
			return nil
		}
		if entry.IsDir() && shouldSkipDir(entry.Name()) && current != canonicalBase {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		requested := requestedWalkPath(canonicalBase, requestedBase, current)
		if !matchPattern(requested, slash(pattern)) {
			return nil
		}
		added, skipped, err := fileIfAllowedInRoot(rootAbs, rootHandle, slash(current), requested, false, excludes, options, hooks)
		if err != nil {
			ignored = append(ignored, IgnoredFile{Path: requested, Reason: err.Error()})
			return nil
		}
		files = append(files, added...)
		ignored = append(ignored, skipped...)
		return nil
	})
	return files, ignored, err
}

func requestedWalkPath(canonicalBase string, requestedBase string, current string) string {
	rel, err := relativeSlash(canonicalBase, current, filepath.Rel)
	if err != nil || outsideRoot(rel) {
		return slash(current)
	}
	if rel == "." {
		return requestedBase
	}
	return slash(filepath.Join(filepath.FromSlash(requestedBase), filepath.FromSlash(rel)))
}

func fileIfAllowedInRoot(rootAbs string, rootHandle *os.Root, canonical string, requested string, finalLink bool, excludes []string, options Options, hooks resolverHooks) ([]File, []IgnoredFile, error) {
	lstatInfo, err := rootHandle.Lstat(canonical)
	if err != nil {
		return nil, nil, err
	}
	if lstatInfo.Mode()&os.ModeSymlink != 0 {
		if _, err := rootHandle.Stat(canonical); err != nil {
			return nil, []IgnoredFile{{Path: requested, Reason: "outside root"}}, nil
		}
		return nil, []IgnoredFile{{Path: requested, Reason: "symbolic link"}}, nil
	}
	if hooks.afterFileLstat != nil {
		hooks.afterFileLstat(canonical)
	}
	info, err := rootHandle.Stat(canonical)
	if err != nil {
		return nil, nil, err
	}
	if finalLink {
		return nil, []IgnoredFile{{Path: requested, Reason: "symbolic link"}}, nil
	}
	if !lstatInfo.Mode().IsRegular() || !info.Mode().IsRegular() {
		return nil, []IgnoredFile{{Path: requested, Reason: "not a regular file"}}, nil
	}
	if !os.SameFile(lstatInfo, info) {
		return nil, []IgnoredFile{{Path: requested, Reason: "file changed during containment check"}}, nil
	}
	if ignored := ignoredByFilePolicy(requested, excludes); ignored != nil {
		return nil, []IgnoredFile{*ignored}, nil
	}
	if hooks.beforeFileOpen != nil {
		hooks.beforeFileOpen(canonical)
	}
	opened, err := rootHandle.Open(canonical)
	if err != nil {
		return nil, nil, err
	}
	defer opened.Close()
	openedInfo, err := opened.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return nil, []IgnoredFile{{Path: requested, Reason: "file changed during containment check"}}, nil
	}
	if options.MaxFileSizeBytes > 0 && openedInfo.Size() > options.MaxFileSizeBytes {
		return nil, []IgnoredFile{{Path: requested, Reason: "file exceeds max size"}}, nil
	}
	if hooks.afterFileOpen != nil {
		hooks.afterFileOpen()
	}
	content, tooLarge, err := readBoundedFile(opened, options.MaxFileSizeBytes)
	if err != nil {
		return nil, nil, err
	}
	if tooLarge {
		return nil, []IgnoredFile{{Path: requested, Reason: "file exceeds max size"}}, nil
	}
	return []File{{
		Path:         filepath.Join(rootAbs, filepath.FromSlash(requested)),
		RelativePath: requested,
		SizeBytes:    openedInfo.Size(),
		content:      content,
		hasContent:   true,
		mode:         openedInfo.Mode(),
		modTime:      openedInfo.ModTime(),
	}}, nil, nil
}

func readBoundedFile(file *os.File, maxBytes int64) ([]byte, bool, error) {
	if maxBytes <= 0 {
		content, err := io.ReadAll(file)
		return content, false, err
	}
	maxInt64 := int64(^uint64(0) >> 1)
	limit := maxBytes
	if maxBytes < maxInt64 {
		limit++
	}
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, false, err
	}
	return content, int64(len(content)) > maxBytes, nil
}

func fileIfAllowedWithRel(rootAbs string, filePath string, excludes []string, options Options, relFunc func(string, string) (string, error)) ([]File, []IgnoredFile, error) {
	rel, err := relativeSlash(rootAbs, filePath, relFunc)
	if err != nil {
		return nil, []IgnoredFile{containmentFailure(filePath, err)}, nil
	}
	if outsideRoot(rel) {
		return nil, []IgnoredFile{{Path: filePath, Reason: "outside root"}}, nil
	}
	canonical, err := canonicalContainment(rootAbs, filePath, filepath.EvalSymlinks, filepath.Rel)
	if err != nil {
		return nil, nil, err
	}
	if canonical != nil {
		return nil, []IgnoredFile{*canonical}, nil
	}
	symlink, err := rejectSymlink(filePath, os.Lstat)
	if err != nil {
		return nil, nil, err
	}
	if symlink != nil {
		return nil, []IgnoredFile{*symlink}, nil
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, err
	}
	return fileIfAllowedInfo(rootAbs, rel, info, excludes, options)
}

func fileIfAllowedInfo(rootAbs string, rel string, info fs.FileInfo, excludes []string, options Options) ([]File, []IgnoredFile, error) {
	if ignored := ignoredByFilePolicy(rel, excludes); ignored != nil {
		return nil, []IgnoredFile{*ignored}, nil
	}
	if options.MaxFileSizeBytes > 0 && info.Size() > options.MaxFileSizeBytes {
		return nil, []IgnoredFile{{Path: rel, Reason: "file exceeds max size"}}, nil
	}
	return []File{{Path: filepath.Join(rootAbs, filepath.FromSlash(rel)), RelativePath: rel, SizeBytes: info.Size()}}, nil, nil
}

func ignoredByFilePolicy(rel string, excludes []string) *IgnoredFile {
	if excluded(rel, excludes) {
		return &IgnoredFile{Path: rel, Reason: "excluded by pattern"}
	}
	if isSecretName(path.Base(rel)) {
		return &IgnoredFile{Path: rel, Reason: "secret-like filename"}
	}
	return nil
}

func excluded(rel string, excludes []string) bool {
	for _, exclude := range excludes {
		if matchPattern(rel, slash(exclude)) {
			return true
		}
	}
	return false
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", "coverage", ".next", ".turbo":
		return true
	default:
		return false
	}
}

func isSecretName(name string) bool {
	lower := strings.ToLower(name)
	return lower == ".env" ||
		strings.HasPrefix(lower, ".env.") ||
		strings.HasSuffix(lower, ".pem") ||
		strings.HasSuffix(lower, ".key") ||
		strings.HasSuffix(lower, ".p12") ||
		strings.HasSuffix(lower, ".pfx")
}

func hasGlob(pattern string) bool {
	return strings.ContainsAny(pattern, "*?[")
}

func globBase(rootAbs string, pattern string) string {
	target := includeTarget(rootAbs, pattern)
	volume := filepath.VolumeName(target)
	parts := strings.Split(slash(strings.TrimPrefix(target, volume)), "/")
	baseParts := []string{}
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		baseParts = append(baseParts, part)
	}
	if len(baseParts) == 0 {
		return rootAbs
	}
	basePrefix := volume
	if filepath.IsAbs(target) {
		basePrefix += string(filepath.Separator)
	}
	return filepath.Join(append([]string{basePrefix}, baseParts...)...)
}

func matchPattern(rel string, pattern string) bool {
	if pattern == "." {
		return true
	}
	if !strings.Contains(pattern, "**") {
		matched, err := path.Match(pattern, rel)
		return err == nil && matched
	}
	return matchDoubleStar(strings.Split(pattern, "/"), strings.Split(rel, "/"))
}

func matchDoubleStar(patternParts []string, relParts []string) bool {
	if len(patternParts) == 0 {
		return len(relParts) == 0
	}
	if patternParts[0] == "**" {
		if matchDoubleStar(patternParts[1:], relParts) {
			return true
		}
		for i := range relParts {
			if matchDoubleStar(patternParts[1:], relParts[i+1:]) {
				return true
			}
		}
		return false
	}
	if len(relParts) == 0 {
		return false
	}
	matched, err := path.Match(patternParts[0], relParts[0])
	if err != nil || !matched {
		return false
	}
	return matchDoubleStar(patternParts[1:], relParts[1:])
}

func relSlash(rootAbs string, filePath string) (string, error) {
	return relativeSlash(rootAbs, filePath, filepath.Rel)
}

func relativeSlash(rootAbs string, filePath string, relFunc func(string, string) (string, error)) (string, error) {
	rel, err := relFunc(rootAbs, filePath)
	if err != nil {
		return "", err
	}
	return slash(rel), nil
}

func includeTarget(rootAbs string, include string) string {
	target := filepath.FromSlash(include)
	if filepath.IsAbs(target) {
		return target
	}
	return filepath.Join(rootAbs, target)
}

func canonicalizePath(path string) (string, error) {
	resolved := filepath.Clean(path)
	for linksWalked := 0; linksWalked <= 255; linksWalked++ {
		next, foundLink, err := resolveFirstLink(resolved)
		if err != nil {
			return "", err
		}
		if !foundLink {
			return filepath.Clean(next), nil
		}
		resolved = filepath.Clean(next)
	}
	return "", fmt.Errorf("canonicalization exceeded symbolic-link limit")
}

func resolveFirstLink(path string) (string, bool, error) {
	volume := filepath.VolumeName(path)
	current := volume
	if filepath.IsAbs(path) {
		current += string(filepath.Separator)
	}
	rest := strings.TrimPrefix(path, volume)
	parts := strings.FieldsFunc(rest, isPathSeparatorRune)
	for index, part := range parts {
		candidate := filepath.Join(current, part)
		link, err := os.Readlink(candidate)
		if err != nil {
			info, lstatErr := os.Lstat(candidate)
			if lstatErr != nil {
				return "", false, lstatErr
			}
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return "", false, err
			}
			current = candidate
			continue
		}
		if filepath.IsAbs(link) {
			current = link
		} else {
			current = filepath.Join(filepath.Dir(candidate), link)
		}
		if index+1 < len(parts) {
			current = filepath.Join(append([]string{current}, parts[index+1:]...)...)
		}
		return current, true, nil
	}
	return current, false, nil
}

func isPathSeparatorRune(value rune) bool {
	return value == '/' || (os.PathSeparator == '\\' && value == '\\')
}

func validateRootedTarget(rootAbs string, target string, evalSymlinks func(string) (string, error), relFunc func(string, string) (string, error)) (rootedTarget, *IgnoredFile) {
	if !sameVolume(rootAbs, target) {
		ignored := IgnoredFile{Path: target, Reason: "outside root"}
		return rootedTarget{}, &ignored
	}
	requested, err := relativeSlash(rootAbs, target, relFunc)
	if err != nil {
		ignored := containmentFailure(target, err)
		return rootedTarget{}, &ignored
	}
	if outsideRoot(requested) {
		ignored := IgnoredFile{Path: target, Reason: "outside root"}
		return rootedTarget{}, &ignored
	}
	canonicalRoot, err := evalSymlinks(rootAbs)
	if err != nil {
		ignored := containmentFailure(target, err)
		return rootedTarget{}, &ignored
	}
	canonicalTarget, err := evalSymlinks(target)
	if err != nil {
		ignored := containmentFailure(target, err)
		return rootedTarget{}, &ignored
	}
	if !sameVolume(canonicalRoot, canonicalTarget) {
		ignored := IgnoredFile{Path: target, Reason: "outside root"}
		return rootedTarget{}, &ignored
	}
	canonical, err := relativeSlash(canonicalRoot, canonicalTarget, relFunc)
	if err != nil {
		ignored := containmentFailure(target, err)
		return rootedTarget{}, &ignored
	}
	if outsideRoot(canonical) {
		ignored := IgnoredFile{Path: target, Reason: "outside root"}
		return rootedTarget{}, &ignored
	}
	finalLink, err := finalPathComponentIsLink(target, canonicalTarget, evalSymlinks)
	if err != nil {
		ignored := containmentFailure(target, err)
		return rootedTarget{}, &ignored
	}
	return rootedTarget{requested: requested, canonical: canonical, finalLink: finalLink}, nil
}

func finalPathComponentIsLink(target string, canonicalTarget string, evalSymlinks func(string) (string, error)) (bool, error) {
	canonicalParent, err := evalSymlinks(filepath.Dir(target))
	if err != nil {
		return false, err
	}
	expected := filepath.Join(canonicalParent, filepath.Base(target))
	if runtime.GOOS == "windows" {
		return !strings.EqualFold(filepath.Clean(expected), filepath.Clean(canonicalTarget)), nil
	}
	return filepath.Clean(expected) != filepath.Clean(canonicalTarget), nil
}

func preWalkContainment(rootAbs string, target string, evalSymlinks func(string) (string, error), relFunc func(string, string) (string, error)) *IgnoredFile {
	_, ignored := validateRootedTarget(rootAbs, target, evalSymlinks, relFunc)
	return ignored
}

func containmentFailure(filePath string, err error) IgnoredFile {
	return IgnoredFile{Path: filePath, Reason: fmt.Sprintf("containment check failed: %v", err)}
}

func canonicalContainment(rootAbs string, filePath string, evalSymlinks func(string) (string, error), relFunc func(string, string) (string, error)) (*IgnoredFile, error) {
	_, ignored := validateRootedTarget(rootAbs, filePath, evalSymlinks, relFunc)
	return ignored, nil
}

func outsideRoot(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, "../")
}

func sameVolume(rootAbs string, target string) bool {
	return strings.EqualFold(filepath.VolumeName(rootAbs), filepath.VolumeName(target))
}

func rejectSymlink(filePath string, lstat func(string) (os.FileInfo, error)) (*IgnoredFile, error) {
	info, err := lstat(filePath)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return nil, nil
	}
	ignored := IgnoredFile{Path: filePath, Reason: "symbolic link"}
	return &ignored, nil
}

func slash(value string) string {
	return strings.ReplaceAll(filepath.ToSlash(value), "\\", "/")
}
