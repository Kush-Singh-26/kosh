package run

import (
	"os"
	"path/filepath"
	"strings"
)

// CleanupOrphans removes files from the output directory that were not
// registered by the Sink during the current build session.
func (b *Builder) CleanupOrphans() {
	if b.Sink == nil {
		return
	}

	// Only clean orphans in development mode when writing directly to output.
	// Clean builds use staging directories which start empty, so orphans
	// are naturally removed during the atomic swap.
	if !b.cfg.IsDev || b.isCleanBuild {
		return
	}

	outputDir := b.cfg.OutputDir
	written := b.Sink.GetWrittenFiles()

	// Convert written map keys to absolute paths for comparison
	absWritten := make(map[string]bool)
	for path := range written {
		abs, _ := filepath.Abs(path)
		absWritten[filepath.ToSlash(abs)] = true
	}

	b.logger.Debug("Starting orphan cleanup", "outputDir", outputDir, "trackedFiles", len(absWritten))

	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories like .git
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip specific files that are managed elsewhere or should persist
		base := info.Name()
		if base == ".nojekyll" || base == "search.wasm" || base == "search.wasm.br" || base == "search.bin" {
			return nil
		}
		if strings.HasPrefix(base, ".") {
			return nil
		}

		absPath, _ := filepath.Abs(path)
		absPath = filepath.ToSlash(absPath)

		if !absWritten[absPath] {
			b.logger.Debug("🗑️ Removing orphaned output file", "path", path)
			_ = os.Remove(path)
		}
		return nil
	})

	if err != nil {
		b.logger.Warn("Orphan cleanup encountered errors", "error", err)
	}

	// Remove empty directories
	_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() || path == outputDir {
			return nil
		}
		entries, _ := os.ReadDir(path)
		if len(entries) == 0 {
			_ = os.Remove(path)
		}
		return nil
	})
}
