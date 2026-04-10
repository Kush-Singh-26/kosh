package scanner

import (
	"bytes"
	"context"
	"errors"
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

const (
	scanResultFilesCap        = 50
	scanResultAssetsCap       = 10
	scanConcurrencyMultiplier = 2
	scanBufferSize            = 16 * 1024
	frontmatterParts          = 3
)

type metadataScanner struct{}

// NewScanner returns a metadata scanner implementation.
func NewScanner() Scanner {
	return &metadataScanner{}
}

// Scan performs a full scan and returns aggregated results.
func (service *metadataScanner) Scan(options ScanOptions) (*models.MetadataScannerResult, error) {
	resultChan, errorChan := service.ScanStreaming(options)
	return <-resultChan, <-errorChan
}

// ScanStreaming performs a scan and returns result and error channels.
func (service *metadataScanner) ScanStreaming(options ScanOptions) (<-chan *models.MetadataScannerResult, <-chan error) {
	contextValue := options.Ctx
	contentDir := options.ContentDir
	sourceFs := options.SrcFs
	siteConfig := options.Cfg
	fileChan := options.FileChan

	resultChan := make(chan *models.MetadataScannerResult, 1)
	errorChan := make(chan error, 1)

	if contextValue == nil {
		contextValue = context.Background()
	}
	async.FireAndForget(contextValue, slog.Default(), "metadata scan stream", func() error {
		result := &models.MetadataScannerResult{
			Files:         make([]models.ScannedFile, 0, scanResultFilesCap),
			ContentAssets: make([]models.ScannedAsset, 0, scanResultAssetsCap),
		}

		var mutex sync.Mutex
		errorGroup, groupCtx := errgroup.WithContext(contextValue)
		errorGroup.SetLimit(runtime.NumCPU() * scanConcurrencyMultiplier)

		err := afero.Walk(sourceFs, contentDir, func(path string, info fs.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			if filepath.Ext(path) != ".md" {
				mutex.Lock()
				result.ContentAssets = append(result.ContentAssets, models.ScannedAsset{
					Path: path,
					Info: info,
				})
				mutex.Unlock()
				return nil
			}

			if filepath.Base(path) == "404.md" {
				mutex.Lock()
				result.Has404 = true
				mutex.Unlock()
			}

			errorGroup.Go(func() error {
				scannedFile, err := service.ScanFile(sourceFs, siteConfig, path)
				if err != nil {
					return nil
				}

				if fileChan != nil {
					select {
					case fileChan <- scannedFile:
					case <-groupCtx.Done():
						return groupCtx.Err()
					}
				}

				mutex.Lock()
				result.Files = append(result.Files, scannedFile)
				mutex.Unlock()
				return nil
			})

			return nil
		})

		if err != nil {
			resultChan <- nil
			errorChan <- err
			return err
		}

		if err := errorGroup.Wait(); err != nil {
			resultChan <- nil
			errorChan <- err
			return err
		}

		resultChan <- result
		errorChan <- nil
		return nil
	})

	return resultChan, errorChan
}

// ScanFile scans a single markdown file for metadata.
func (service *metadataScanner) ScanFile(sourceFs afero.Fs, siteConfig *config.Config, path string) (models.ScannedFile, error) {
	file, err := sourceFs.Open(path)
	if err != nil {
		return models.ScannedFile{}, err
	}
	buffer := make([]byte, scanBufferSize)
	bytesRead, err := io.ReadFull(file, buffer)
	_ = file.Close()
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return models.ScannedFile{}, err
	}
	fileData := buffer[:bytesRead]

	info, err := sourceFs.Stat(path)
	if err != nil {
		return models.ScannedFile{}, err
	}

	relativePath, _ := filepath.Rel(siteConfig.ContentDir, path)

	var frontmatter []byte
	var bodyOffset int
	parts := bytes.SplitN(fileData, hashing.YAMLDelim, frontmatterParts)
	if len(parts) < frontmatterParts && bytesRead == scanBufferSize {
		fullData, err := afero.ReadFile(sourceFs, path)
		if err == nil {
			fileData = fullData
			parts = bytes.SplitN(fileData, hashing.YAMLDelim, frontmatterParts)
		}
	}
	if len(parts) >= frontmatterParts {
		frontmatter = bytes.TrimSpace(parts[1])
		bodyOffset = bytes.Index(fileData, parts[2])
	} else {
		bodyOffset = 0
	}

	preparsedMetadata, _ := hashing.ParseFrontmatter(frontmatter)

	title := timeutil.ExtractStringFromMap(preparsedMetadata, "title")
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	description := timeutil.ExtractStringFromMap(preparsedMetadata, "description")
	date := timeutil.ExtractDateStringFromMap(preparsedMetadata, "date")
	isDraft := timeutil.ExtractBoolFromMap(preparsedMetadata, "draft")
	isPinned := timeutil.ExtractBoolFromMap(preparsedMetadata, "pinned")

	weight := 0
	if weightValue, ok := preparsedMetadata["weight"].(int); ok {
		weight = weightValue
	} else if weightValue, ok := preparsedMetadata["weight"].(float64); ok {
		weight = int(weightValue)
	}

	tags := timeutil.ExtractSliceFromMap(preparsedMetadata, "tags")
	bodyHash := ""
	cleanHtmlRelPath := strings.TrimSuffix(relativePath, filepath.Ext(relativePath)) + ".html"
	postLink := navigation.BuildAbsoluteURL(siteConfig.BaseURL, cleanHtmlRelPath)

	frontmatterHash := hashing.GetFrontmatterHashFromValues(hashing.FrontmatterHashOptions{
		Title:       title,
		Description: description,
		Date:        date,
		Tags:        tags,
		IsPinned:    isPinned,
		IsDraft:     isDraft,
		Weight:      weight,
		Other:       preparsedMetadata,
	})

	return models.ScannedFile{
		Path:            path,
		RelPath:         relativePath,
		Title:           title,
		Description:     description,
		Date:            date,
		IsDraft:         isDraft,
		IsPinned:        isPinned,
		Weight:          weight,
		Tags:            tags,
		Info:            info,
		BodyHash:        bodyHash,
		FrontmatterHash: frontmatterHash,
		BodyOffset:      bodyOffset,
		Link:            postLink,
		SourceLoader:    func() ([]byte, error) { return afero.ReadFile(sourceFs, path) },
		PreParsedMeta:   preparsedMetadata,
	}, nil
}
