package services

import (
	"bytes"
	"context"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

var yamlDelim = []byte("---")

type metadataScanner struct{}

func NewMetadataScanner() MetadataScanner {
	return &metadataScanner{}
}

func (s *metadataScanner) Scan(ctx context.Context, contentDir string, srcFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error) {
	result := &models.MetadataScannerResult{
		Files:         make([]models.ScannedFile, 0, 50),
		ContentAssets: make([]models.ScannedAsset, 0, 10),
	}

	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
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
				case <-ctx.Done():
					return ctx.Err()
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
		return nil, err
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *metadataScanner) ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error) {
	data, err := afero.ReadFile(srcFs, path)
	if err != nil {
		return models.ScannedFile{}, err
	}

	info, err := srcFs.Stat(path)
	if err != nil {
		return models.ScannedFile{}, err
	}

	relPath, _ := filepath.Rel(cfg.ContentDir, path)
	version, _ := navigation.GetVersionFromPath(path)

	var frontmatter []byte
	var body []byte
	var bodyOffset int
	parts := bytes.SplitN(data, yamlDelim, 3)
	if len(parts) >= 3 {
		frontmatter = bytes.TrimSpace(parts[1])
		body = bytes.TrimSpace(parts[2])
		bodyOffset = bytes.Index(data, parts[2])
	} else {
		body = bytes.TrimSpace(data)
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
	bodyHash := hashing.HashBytes(body)
	cleanHtmlRelPath := strings.TrimSuffix(relPath, filepath.Ext(relPath)) + ".html"
	postLink := navigation.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)

	frontmatterHash := hashing.GetFrontmatterHashFromValues(
		title, description, date, tags, pinned,
	)

	return models.ScannedFile{
		Path:            path,
		RelPath:         relPath,
		Version:         version,
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
		Source:          data,
		PreParsedMeta:   preParsedMeta,
	}, nil
}
