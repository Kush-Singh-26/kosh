package fs

import (
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

type assetEntryPoints struct {
	js  []string
	css []string
}

type assetScanResult struct {
	points assetEntryPoints
	hash   string
	metas  []fileMeta
}

type fileMeta struct {
	path  string
	size  int64
	mtime int64
}

func (a *assetScanResult) hasJS() bool  { return len(a.points.js) > 0 }
func (a *assetScanResult) hasCSS() bool { return len(a.points.css) > 0 }

func scanAssets(srcFs afero.Fs, srcDir string) (*assetScanResult, error) {
	srcDir = NormalizePath(srcDir)

	var js, css []string
	var metas []fileMeta
	var walkMu sync.Mutex

	err := ParallelWalk(WalkOptions{
		Ctx:         context.Background(),
		SourceFs:    srcFs,
		Root:        filepath.FromSlash(srcDir),
		Concurrency: 0,
		WalkFn: func(path string, info fs.FileInfo, err error) error {

			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			baseName := filepath.Base(path)

			if baseName == "wasm_engine.js" || baseName == "wasm_exec.js" || baseName == "engine.js" {
				return nil
			}

			walkMu.Lock()
			defer walkMu.Unlock()

			switch ext {
			case ".js":
				// For JS, we keep all as entry points for now
				js = append(js, path)
			case ".css":
				// For CSS, we only bundle layout.css and graph.css
				// All other CSS should be imported via these entry points
				if baseName == "layout.css" || baseName == "graph.css" {
					css = append(css, path)
				}
			}

			metas = append(metas, fileMeta{
				path:  path,
				size:  info.Size(),
				mtime: info.ModTime().UnixNano(),
			})
			return nil
		}})
	if err != nil {
		return nil, fmt.Errorf("failed to scan for assets: %w", err)
	}

	slices.SortFunc(metas, func(a, b fileMeta) int {
		return strings.Compare(a.path, b.path)
	})

	inputHash := xxh3.New()
	for _, m := range metas {
		if _, err := fmt.Fprintf(inputHash, "%s:%d:%d;", m.path, m.size, m.mtime); err != nil {
			return nil, fmt.Errorf("failed to write to input hash: %w", err)
		}
	}

	sum := inputHash.Sum128()
	b := sum.Bytes()
	hash := hex.EncodeToString(b[:])

	return &assetScanResult{
		points: assetEntryPoints{js: js, css: css},
		hash:   hash,
		metas:  metas,
	}, nil
}
