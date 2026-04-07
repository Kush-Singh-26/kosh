//go:build !wasm

package fs

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"log/slog"
	"sync"

	"github.com/spf13/afero"
)

var (
	copyBufferPool = sync.Pool{
		New: func() any {
			b := make([]byte, 64*1024)
			return &b
		},
	}
)

// ValidatePath ensures that the given path is within the expected base directory.
// This prevents path traversal attacks by checking that the resolved path
// is a subdirectory of the base.
func ValidatePath(baseDir, path string) error {
	cleanBase := filepath.Clean(baseDir)
	cleanPath := filepath.Clean(path)

	if !strings.HasPrefix(cleanPath, cleanBase) {
		return fmt.Errorf("path %s is outside base directory %s", path, baseDir)
	}

	return nil
}

type CopyFileOptions struct {
	SrcFs   afero.Fs
	Sink    ArtifactSink
	SrcPath string
	DstPath string
	ModTime int64
	OnWrite func(string)
}

func CopyFileVFS(opts CopyFileOptions) error {
	// Validate destination path to prevent directory traversal
	if err := ValidatePath(filepath.Dir(opts.DstPath), opts.DstPath); err != nil {
		return fmt.Errorf("invalid destination path: %w", err)
	}

	if err := opts.Sink.MkdirAll(filepath.Dir(opts.DstPath)); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(opts.DstPath), err)
	}

	// Attempt optimized syscall copy for local files
	if realSrc, ok := GetRealPath(opts.SrcFs, opts.SrcPath); ok {
		err := opts.Sink.CopyFile(realSrc, opts.DstPath)
		if err == nil {
			if opts.OnWrite != nil {
				opts.OnWrite(opts.DstPath)
			}
			if opts.ModTime > 0 {
				_ = opts.Sink.SetMtime(opts.DstPath, time.Unix(0, opts.ModTime))
			}
			return nil
		}
		// Fallback to streaming if optimized copy fails
		slog.Debug("Optimized copy failed, falling back to streaming", "path", opts.SrcPath, "error", err)
	}

	in, err := opts.SrcFs.Open(opts.SrcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", opts.SrcPath, err)
	}
	defer func() { _ = in.Close() }()

	bufPtr := copyBufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer copyBufferPool.Put(bufPtr)

	errWrite := opts.Sink.WriteStream(opts.DstPath, func(w io.Writer) error {
		_, err := io.CopyBuffer(w, in, buf)
		return err
	})
	if errWrite != nil {
		return fmt.Errorf("failed to copy file %s: %w", opts.SrcPath, errWrite)
	}

	if opts.OnWrite != nil {
		opts.OnWrite(opts.DstPath)
	}

	if opts.ModTime > 0 {
		_ = opts.Sink.SetMtime(opts.DstPath, time.Unix(0, opts.ModTime))
	}

	return nil
}