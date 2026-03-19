//go:build !windows && !linux && !wasm && !js

package fs

// CopyFileInternal delegates to the centralized pooled stream copier.
func CopyFileInternal(src, dst string) error {
	return StreamCopyFile(src, dst)
}
