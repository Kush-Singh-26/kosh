package scanner

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

type metadataScanner struct{}

// NewScanner returns a metadata scanner implementation.
func NewScanner() Scanner {
	return &metadataScanner{}
}

// Scan performs a full scan and returns aggregated results.
func (s *metadataScanner) Scan(opts ScanOptions) (*models.MetadataScannerResult, error) {
	resChan, errChan := s.ScanStreaming(opts)
	return <-resChan, <-errChan
}

// ScanStreaming performs a scan and returns result and error channels.
func (s *metadataScanner) ScanStreaming(opts ScanOptions) (<-chan *models.MetadataScannerResult, <-chan error) {
	ctx := opts.Ctx
	contentDir := opts.ContentDir
	srcFs := opts.SrcFs
	cfg := opts.Cfg
	fileChan := opts.FileChan

	resultChan := make(chan *models.MetadataScannerResult, 1)
	errChan := make(chan error, 1)

	if ctx == nil {
		ctx = context.Background()
	}
	async.FireAndForget(ctx, slog.Default(), "metadata scan stream", func() error {
		result := &models.MetadataScannerResult{
			Files:         make([]models.ScannedFile, 0, 50),
			ContentAssets: make([]models.ScannedAsset, 0, 10),
		}

		var mu sync.Mutex
		g, gCtx := errgroup.WithContext(ctx)
		g.SetLimit(runtime.NumCPU() * 2)

		err := afero.Walk(srcFs, contentDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			if filepath.Ext(path) != ".md" {
				mu.Lock()
				result.ContentAssets = append(result.ContentAssets, models.ScannedAsset{
					Path: path,
					Info: info,
				})
				mu.Unlock()
				return nil
			}

			if filepath.Base(path) == "404.md" {
				mu.Lock()
				result.Has404 = true
				mu.Unlock()
			}

			g.Go(func() error {
				f, err := s.ScanFile(srcFs, cfg, path)
				if err != nil {
					return nil
				}

				if fileChan != nil {
					select {
					case fileChan <- f:
					case <-gCtx.Done():
						return gCtx.Err()
					}
				}

				mu.Lock()
				result.Files = append(result.Files, f)
				mu.Unlock()
				return nil
			})

			return nil
		})

		if err != nil {
			resultChan <- nil
			errChan <- err
			return
		}

		if err := g.Wait(); err != nil {
			resultChan <- nil
			errChan <- err
			return
		}

		resultChan <- result
		errChan <- nil
		return nil
	})

	return resultChan, errChan
}

// ScanFile scans a single markdown file for metadata.
func (s *metadataScanner) ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error) {
	file, err := srcFs.Open(path)
	if err != nil {
		return models.ScannedFile{}, err
	}
	buf := make([]byte, 16384)
	n, err := io.ReadFull(file, buf)
	file.Close()
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return models.ScannedFile{}, err
	}
	data := buf[:n]

	info, err := srcFs.Stat(path)
	if err != nil {
		return models.ScannedFile{}, err
	}

	relPath, _ := filepath.Rel(cfg.ContentDir, path)

	var frontmatter []byte
	var bodyOffset int
	parts := bytes.SplitN(data, hashing.YAMLDelim, 3)
	if len(parts) < 3 && n == 16384 {
		fullData, err := afero.ReadFile(srcFs, path)
		if err == nil {
			data = fullData
			parts = bytes.SplitN(data, hashing.YAMLDelim, 3)
		}
	}
	if len(parts) >= 3 {
		frontmatter = bytes.TrimSpace(parts[1])
		bodyOffset = bytes.Index(data, parts[2])
	} else {
		bodyOffset = 0
	}

	preParsedMeta, _ := hashing.ParseFrontmatter(frontmatter)

	title := timeutil.ExtractStringFromMap(preParsedMeta, "title")
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	description := timeutil.ExtractStringFromMap(preParsedMeta, "description")
	date := timeutil.ExtractDateStringFromMap(preParsedMeta, "date")
	draft := timeutil.ExtractBoolFromMap(preParsedMeta, "draft")
	pinned := timeutil.ExtractBoolFromMap(preParsedMeta, "pinned")

	weight := 0
	if w, ok := preParsedMeta["weight"].(int); ok {
		weight = w
	} else if w, ok := preParsedMeta["weight"].(float64); ok {
		weight = int(w)
	}

	tags := timeutil.ExtractSliceFromMap(preParsedMeta, "tags")
	bodyHash := ""
	cleanHtmlRelPath := strings.TrimSuffix(relPath, filepath.Ext(relPath)) + ".html"
	postLink := navigation.BuildAbsoluteURL(cfg.BaseURL, cleanHtmlRelPath)

	frontmatterHash := hashing.GetFrontmatterHashFromValues(hashing.FrontmatterHashOptions{
		Title:       title,
		Description: description,
		Date:        date,
		Tags:        tags,
		Pinned:      pinned,
		Draft:       draft,
		Weight:      weight,
		Other:       preParsedMeta,
	})

	return models.ScannedFile{
		Path:            path,
		RelPath:         relPath,
		Title:           title,
		Description:     description,
		Date:            date,
		Draft:           draft,
		Pinned:          pinned,
		Weight:          weight,
		Tags:            tags,
		Info:            info,
		BodyHash:        bodyHash,
		FrontmatterHash: frontmatterHash,
		BodyOffset:      bodyOffset,
		Link:            postLink,
		SourceLoader:    func() ([]byte, error) { return afero.ReadFile(srcFs, path) },
		PreParsedMeta:   preParsedMeta,
	}, nil
}
