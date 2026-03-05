package run

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// cleanOrphans removes files in the output directory that were not rendered in the current build
func (b *Builder) cleanOrphans(sourceFs afero.Fs, renderedFiles map[string]bool) {
	outputDir := b.cfg.OutputDir
	if outputDir == "" {
		return
	}

	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		return
	}

	fmt.Println("🧹 Cleaning orphaned files from output directory...")

	// Create a normalized set of rendered files for comparison
	renderedSet := make(map[string]bool)
	for path := range renderedFiles {
		absPath, err := filepath.Abs(path)
		if err == nil {
			renderedSet[filepath.ToSlash(absPath)] = true
		}
	}

	// Always preserve version folders and special files
	versionFolders := make(map[string]bool)
	for _, v := range b.cfg.Versions {
		if v.Path != "" {
			versionFolders[v.Path] = true
		}
	}

	err = filepath.WalkDir(absOutputDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			// Skip version folders
			rel, _ := filepath.Rel(absOutputDir, path)
			if versionFolders[rel] {
				return filepath.SkipDir
			}
			return nil
		}

		pathSlash := filepath.ToSlash(path)
		if renderedSet[pathSlash] {
			return nil
		}

		// Check if it exists in sourceFs (VFS)
		relPath, _ := filepath.Rel(absOutputDir, path)
		vfsPath := filepath.ToSlash(filepath.Join(outputDir, relPath))
		if exists, _ := afero.Exists(sourceFs, vfsPath); exists {
			return nil
		}

		// Check special files that should never be deleted
		rel, _ := filepath.Rel(absOutputDir, path)
		relSlash := filepath.ToSlash(rel)

		// If it's a special path like sitemap or rss, it might not be in renderedSet if using cache
		// But SyncVFS knows about them.

		// Logic: if it's in SyncVFS alwaysSyncPaths, preserve it
		if utils.IsAlwaysSyncPath(relSlash) {
			return nil
		}

		// Additional check: preserve files that are managed by git or other tools if needed
		if strings.HasPrefix(relSlash, ".git") || relSlash == ".kosh-build.lock" {
			return nil
		}

		// Delete orphaned file
		b.logger.Info("🗑️ Deleting orphaned file", "path", relSlash)
		_ = os.Remove(path)
		return nil
	})

	if err != nil {
		b.logger.Warn("Failed to clean orphans", "error", err)
	}
}
