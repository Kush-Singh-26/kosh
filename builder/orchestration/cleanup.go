package orchestration

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/fs"
)

// cleanupOrphans removes files from the output directory that were not
// registered by the artifactSink during the current build session.
func (engineInstance *Engine) cleanupOrphans() {
	if engineInstance.artifactSink == nil {
		return
	}

	// Only clean orphans in development mode when writing directly to output.
	// Clean builds use staging directories which start empty, so orphans
	// are naturally removed during the atomic swap.
	// Skip cleanup during asset-only incremental builds to prevent deleting images/fonts.
	if !engineInstance.Cfg.IsDev || engineInstance.State.IsCleanBuild || engineInstance.State.IsAssetOnlyBuild {
		return
	}

	outputDir := engineInstance.Cfg.OutputDir
	writtenFiles := engineInstance.artifactSink.GetWrittenFiles()

	// Convert written map keys to absolute paths for comparison
	absoluteWrittenFiles := make(map[string]bool)
	for path := range writtenFiles {
		absolutePath, _ := filepath.Abs(path)
		absoluteWrittenFiles[strings.ToLower(fs.NormalizePath(absolutePath))] = true
	}

	err := filepath.Walk(outputDir, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fileInfo.IsDir() {
			// Skip hidden directories like .git
			if strings.HasPrefix(fileInfo.Name(), ".") && fileInfo.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip specific files that are managed elsewhere or should persist
		baseName := fileInfo.Name()
		if baseName == ".nojekyll" || baseName == "search.wasm" || baseName == "search.wasm.br" || baseName == "search.bin" || baseName == "manifest.json" || baseName == "icon-192.png" || baseName == "icon-512.png" || baseName == "sw.js" ||
			baseName == "graph.json" || baseName == "graph.html" || baseName == "rss.xml" || baseName == "sitemap.xml" || baseName == "404.html" {
			return nil
		}
		if strings.HasPrefix(baseName, ".") {
			return nil
		}

		absolutePath, _ := filepath.Abs(path)
		absolutePath = strings.ToLower(fs.NormalizePath(absolutePath))

		if !absoluteWrittenFiles[absolutePath] {
			_ = os.Remove(path)
		}
		return nil
	})

	if err != nil {
		engineInstance.Deps.Logger.Warn("Orphan cleanup encountered errors", "error", err)
	}

	// Remove empty directories
	_ = filepath.Walk(outputDir, func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil || !fileInfo.IsDir() || path == outputDir {
			return nil
		}
		entries, _ := os.ReadDir(path)
		if len(entries) == 0 {
			_ = os.Remove(path)
		}
		return nil
	})
}
