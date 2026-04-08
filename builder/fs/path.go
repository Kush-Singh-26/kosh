package fs

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/text/unicode/norm"
)

// NormalizePath normalizes a path for consistent handling across platforms.
// - Converts backslashes to forward slashes
// - Cleans the path (removes . and ..)
// - Applies NFC normalization for Unicode
// - On Windows, capitalizes drive letters
func NormalizePath(path string) string {
	if path == "" || path == "." {
		return "."
	}

	// Fast path for already clean paths with forward slashes
	// Note: We must check for backslashes even in fast path
	if !strings.Contains(path, "\\") && !strings.Contains(path, "//") && !strings.Contains(path, "./") {
		if hasNonASCII(path) {
			path = norm.NFC.String(path)
		}
		// Just check for drive letter on Windows
		if runtime.GOOS == "windows" && len(path) >= 2 && path[1] == ':' && path[0] >= 'a' && path[0] <= 'z' {
			return strings.ToUpper(path[:1]) + path[1:]
		}
		return path
	}

	// Apply NFC normalization for consistent Unicode handling across platforms
	path = norm.NFC.String(path)

	// Normalize separators before cleaning so backslashes are handled on all OSes.
	if strings.Contains(path, "\\") {
		path = strings.ReplaceAll(path, "\\", "/")
	}

	// Clean the path and convert to forward slashes for internal consistency
	path = filepath.ToSlash(filepath.Clean(path))

	// On Windows, capitalize drive letter for consistency (e.g., "c:" -> "C:")
	if runtime.GOOS == "windows" {
		if len(path) >= 2 && path[1] == ':' {
			if path[0] >= 'a' && path[0] <= 'z' {
				path = strings.ToUpper(path[:1]) + path[1:]
			}
		}
	}

	return path
}

// AbsNormalizePath returns the absolute normalized path.
func AbsNormalizePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return NormalizePath(abs), nil
}

// NormalizeURLPath normalizes a URL path.
func NormalizeURLPath(p string) string {
	if p == "" {
		return "."
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return pathpkg.Clean(p)
}

// MarkdownToHTMLPath converts a .md file path to its .html equivalent (lowercased).
func MarkdownToHTMLPath(path string) string {
	return strings.ToLower(strings.Replace(path, ".md", ".html", 1))
}

// NormalizeWatchPath normalizes a path from a file watcher, making it relative to the working directory if needed.
func NormalizeWatchPath(path, wd string) string {
	nativePath := filepath.Clean(path)
	if wd != "" {
		nativeWd := filepath.Clean(wd)
		if !filepath.IsAbs(nativeWd) {
			if absWd, err := filepath.Abs(nativeWd); err == nil {
				nativeWd = absWd
			}
		}
		if filepath.IsAbs(nativePath) {
			if rel, err := filepath.Rel(nativeWd, nativePath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				return NormalizePath(rel)
			}
		}
	}
	return NormalizePath(nativePath)
}

// SafeRel returns a relative path from base to target, ensuring no path traversal.
func SafeRel(base, target string) (string, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return "", err
	}

	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("path traversal detected: result escapes base directory")
	}

	return NormalizePath(rel), nil
}

// WriteFileVFS writes data to a file in an afero filesystem, creating directories as needed.
func WriteFileVFS(fs afero.Fs, path string, data []byte) error {
	if err := fs.MkdirAll(filepath.Dir(path), defaultDirMode); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}
	return afero.WriteFile(fs, path, data, defaultFileMode)
}

// GetRelativePrefix calculates the relative prefix needed to go up from a path.
// E.g., "foo/bar/baz.html" -> "../../"
func GetRelativePrefix(htmlPath string) string {
	if len(htmlPath) == 0 {
		return ""
	}

	// Clean path and ensure forward slashes
	path := NormalizePath(htmlPath)

	// Ignore leading slash for depth calculation
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	if path == "index.html" || path == "." || path == "" {
		return ""
	}

	depth := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			depth++
		}
	}

	if depth == 0 {
		return ""
	}

	switch depth {
	case 1:
		return "../"
	case 2:
		return "../../"
	case depthThree:
		return "../../../"
	default:
		return strings.Repeat("../", depth)
	}
}

const depthThree = 3

// GetRealPath attempts to find the underlying OS path for a given afero.Fs and virtual path.
// Returns (realPath, true) if it's a local filesystem, (path, false) otherwise.
func GetRealPath(fs afero.Fs, path string) (string, bool) {
	if fs == nil {
		return "", false
	}

	currFs := fs
	for {
		switch currFs.(type) {
		case *afero.OsFs:
			return path, true
		case *afero.ReadOnlyFs:
			// ReadOnlyFs wraps another Fs
			break
		}
		break
	}

	// Fallback: check if the FS is a known wrapper by name or behavior
	fsName := fmt.Sprintf("%T", fs)
	if strings.Contains(fsName, "OsFs") {
		return path, true
	}

	return "", false
}

// IsPathInOrSame checks if a path is inside or the same as a target directory.
func IsPathInOrSame(path, targetDir string) bool {
	return strings.HasPrefix(path, targetDir+"/") || path == targetDir
}

var (
	repoRoot     string
	repoRootOnce sync.Once
)

// SetRepoRoot explicitly sets the repository root path.
// This should be called early in the application lifecycle to ensure
// all path calculations are based on the correct root.
func SetRepoRoot(root string) {
	repoRoot = root
	repoRootOnce.Do(func() {}) // Prevent DetectTestingMode/runtime.Caller from overriding
}

// RepoRoot returns the absolute path to the repository root directory.
// It checks KOSH_REPO_ROOT, explicitly set repo root, or falls back to
// runtime.Caller(0) only in testing mode.
func RepoRoot() string {
	repoRootOnce.Do(func() {
		// 1. Check environment variable
		if envRoot := os.Getenv("KOSH_REPO_ROOT"); envRoot != "" {
			repoRoot = envRoot
			return
		}

		// 2. Fallback to runtime.Caller ONLY if in testing mode
		if DetectTestingMode() {
			_, file, _, ok := runtime.Caller(0)
			if ok {
				// builder/fs/path.go -> ../../ = repo root
				repoRoot = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
			}
		}
	})

	if repoRoot == "" {
		return "." // Fallback for standard users (production)
	}
	return repoRoot
}

// RepoPath joins path parts to the repository root.
func RepoPath(parts ...string) string {
	all := append([]string{RepoRoot()}, parts...)
	return filepath.Join(all...)
}

// DetectTestingMode inspects os.Args to determine if we are running in a test context.
func DetectTestingMode() bool {
	if len(os.Args) > 0 {
		if strings.HasSuffix(os.Args[0], ".test") || strings.HasSuffix(os.Args[0], ".test.exe") || strings.Contains(os.Args[0], "_test") {
			return true
		}
	}
	return false
}

func hasNonASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}
