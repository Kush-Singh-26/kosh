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
	"time"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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
		// Pass 1: Discover sections (_index.md files) to build the Data Cascade.
		sections := make(map[string]map[string]any)
		var sectionsMu sync.Mutex
		errPass1 := fspkg.ParallelWalk(fspkg.WalkOptions{
			Ctx:         contextValue,
			SourceFs:    sourceFs,
			Root:        contentDir,
			Concurrency: runtime.NumCPU() * scanConcurrencyMultiplier,
			WalkFn: func(path string, info fs.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if filepath.Base(path) == "_index.md" {
					scanned, err := service.ScanFile(sourceFs, siteConfig, path)
					if err == nil {
						relDir := filepath.Dir(scanned.RelPath)
						if relDir == "." {
							relDir = ""
						}
						sectionsMu.Lock()
						sections[relDir] = scanned.PreParsedMeta
						sectionsMu.Unlock()
					}
				}
				return nil
			},
		})
		if errPass1 != nil {
			slog.Warn("Scanner Pass 1 (sections) failed", "error", errPass1)
		}

		result := &models.MetadataScannerResult{
			Files:         make([]models.ScannedResource, 0, scanResultFilesCap),
			ContentAssets: make([]models.ScannedAsset, 0, scanResultAssetsCap),
		}

		var mutex sync.Mutex
		errorGroup, groupCtx := errgroup.WithContext(contextValue)
		errorGroup.SetLimit(runtime.NumCPU() * scanConcurrencyMultiplier)

		err := fspkg.ParallelWalk(fspkg.WalkOptions{
			Ctx:         contextValue,
			SourceFs:    sourceFs,
			Root:        contentDir,
			Concurrency: runtime.NumCPU() * scanConcurrencyMultiplier,
			WalkFn: func(path string, info fs.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}

				filename := filepath.Base(path)
				if filename == "_index.md" {
					return nil // Already handled in Pass 1
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

				if filename == "404.md" {
					mutex.Lock()
					result.Has404 = true
					mutex.Unlock()
				}

				errorGroup.Go(func() error {
					scannedFile, err := service.ScanFile(sourceFs, siteConfig, path)
					if err != nil {
						return nil
					}

					// Apply Data Cascade: merge section metadata into page metadata.
					relDir := filepath.Dir(scannedFile.RelPath)
					if relDir == "." {
						relDir = ""
					}

					// Resolve cascaded metadata by walking up the directory tree to the root.
					// We merge both global root metadata and 'cascade' blocks.
					merged := make(map[string]any)
					pathParts := strings.Split(relDir, string(filepath.Separator))
					currentPath := ""
					for i := 0; i <= len(pathParts); i++ {
						if i > 0 && pathParts[i-1] == "" {
							continue
						}
						if sectionMeta, ok := sections[currentPath]; ok {
							// Cascade Block: Only propagate fields inside the 'cascade' key
							if cascade, ok := sectionMeta["cascade"].(map[string]any); ok {
								for k, v := range cascade {
									merged[k] = v
								}
							}

							// Specific handling for root metadata (optional: allow some root fields to cascade)
							if currentPath == "" {
								if desc, ok := sectionMeta["description"].(string); ok {
									merged["description"] = desc
								}
							}
						}
						if i < len(pathParts) {
							if currentPath != "" {
								currentPath += string(filepath.Separator)
							}
							currentPath += pathParts[i]
						}
					}

					// Finally merge the page's own metadata (overwrites cascaded values)
					for k, v := range scannedFile.PreParsedMeta {
						merged[k] = v
					}
					scannedFile.PreParsedMeta = merged

					// Re-evaluate core fields from the merged metadata
					if layout, ok := merged["layout"].(string); ok {
						scannedFile.Layout = layout
					}
					if title, ok := merged["title"].(string); ok && scannedFile.Title == "" {
						scannedFile.Title = title
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

					// Update site-wide discovery metadata
					light := models.LightResourceMetadata{
						Path:        scannedFile.Path,
						Title:       scannedFile.Title,
						DateObj:     scannedFile.DateObj,
						Taxonomies:  scannedFile.Taxonomies,
						IsPinned:    scannedFile.IsPinned,
						Weight:      scannedFile.Weight,
						ReadingTime: scannedFile.ReadingTime,
						IsDraft:     scannedFile.IsDraft,
						Description: scannedFile.Description,
						Link:        scannedFile.Link,
						Layout:      scannedFile.Layout,
					}
					result.Metadata = append(result.Metadata, light)

					// Aggregate Taxonomies
					if result.TaxonomyMap == nil {
						result.TaxonomyMap = make(map[string]map[string][]models.LightResourceMetadata)
					}
					for taxK, terms := range scannedFile.Taxonomies {
						if _, ok := result.TaxonomyMap[taxK]; !ok {
							result.TaxonomyMap[taxK] = make(map[string][]models.LightResourceMetadata)
						}
						for _, t := range terms {
							result.TaxonomyMap[taxK][t] = append(result.TaxonomyMap[taxK][t], light)
						}
					}
					mutex.Unlock()
					return nil
				})

				return nil
			},
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
func (service *metadataScanner) ScanFile(sourceFs afero.Fs, siteConfig *config.Config, path string) (models.ScannedResource, error) {
	file, err := sourceFs.Open(path)
	if err != nil {
		return models.ScannedResource{}, err
	}
	buffer := make([]byte, scanBufferSize)
	bytesRead, err := io.ReadFull(file, buffer)
	_ = file.Close()
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return models.ScannedResource{}, err
	}
	fileData := buffer[:bytesRead]

	info, err := sourceFs.Stat(path)
	if err != nil {
		return models.ScannedResource{}, err
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

	dateObj, _ := time.ParseInLocation("2006-01-02", date, time.UTC)

	// Extract ALL taxonomies: check for "tags" and "categories" etc. based on config.
	taxonomies := make(map[string][]string)
	// Default: Tags
	if tags := timeutil.ExtractSliceFromMap(preparsedMetadata, "tags"); len(tags) > 0 {
		taxonomies["tags"] = tags
	}
	// Categories
	if cats := timeutil.ExtractSliceFromMap(preparsedMetadata, "categories"); len(cats) > 0 {
		taxonomies["categories"] = cats
	}
	// Series
	if series := timeutil.ExtractSliceFromMap(preparsedMetadata, "series"); len(series) > 0 {
		taxonomies["series"] = series
	}

	bodyHash := ""
	cleanHtmlRelPath := strings.TrimSuffix(relativePath, filepath.Ext(relativePath)) + ".html"
	postLink := navigation.BuildAbsoluteURL(siteConfig.BaseURL, cleanHtmlRelPath)

	frontmatterHash := hashing.GetFrontmatterHashFromValues(hashing.FrontmatterHashOptions{
		Title:       title,
		Description: description,
		Date:        date,
		Taxonomies:  taxonomies,
		IsPinned:    isPinned,
		IsDraft:     isDraft,
		Weight:      weight,
		Other:       preparsedMetadata,
	})

	return models.ScannedResource{
		Path:            path,
		RelPath:         relativePath,
		Title:           title,
		Description:     description,
		Date:            date,
		DateObj:         dateObj,
		IsDraft:         isDraft,
		IsPinned:        isPinned,
		Weight:          weight,
		Taxonomies:      taxonomies,
		Info:            info,
		BodyHash:        bodyHash,
		FrontmatterHash: frontmatterHash,
		BodyOffset:      bodyOffset,
		Link:            postLink,
		SourceLoader:    func() ([]byte, error) { return afero.ReadFile(sourceFs, path) },
		PreParsedMeta:   preparsedMetadata,
	}, nil
}
