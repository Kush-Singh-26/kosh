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
	resultChan := make(chan *models.MetadataScannerResult, 1)
	errorChan := make(chan error, 1)

	contextValue := options.Ctx
	if contextValue == nil {
		contextValue = context.Background()
	}

	async.FireAndForget(contextValue, slog.Default(), "metadata scan stream", func() error {
		sections := service.discoverSections(contextValue, options.SrcFs, options.ContentDir, options.Cfg)

		result := &models.MetadataScannerResult{
			Files:         make([]models.ScannedResource, 0, scanResultFilesCap),
			ContentAssets: make([]models.ScannedAsset, 0, scanResultAssetsCap),
		}

		var mutex sync.Mutex
		errorGroup, groupCtx := errgroup.WithContext(contextValue)
		errorGroup.SetLimit(runtime.NumCPU() * scanConcurrencyMultiplier)

		err := fspkg.ParallelWalk(fspkg.WalkOptions{
			Ctx:         contextValue,
			SourceFs:    options.SrcFs,
			Root:        options.ContentDir,
			Concurrency: runtime.NumCPU() * scanConcurrencyMultiplier,
			WalkFn: func(path string, info fs.FileInfo, err error) error {
				return service.processScanPath(groupCtx, path, info, err, options, sections, result, &mutex, errorGroup)
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

// discoverSections performs Pass 1: discover all _index.md files to build data cascade.
func (service *metadataScanner) discoverSections(ctx context.Context, sourceFs afero.Fs, contentDir string, siteConfig *config.Config) map[string]map[string]any {
	sections := make(map[string]map[string]any)
	var sectionsMu sync.Mutex

	err := fspkg.ParallelWalk(fspkg.WalkOptions{
		Ctx:         ctx,
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

	if err != nil {
		slog.Warn("Scanner Pass 1 (sections) failed", "error", err)
	}

	return sections
}

// processScanPath processes a single path during the scan walk.
func (service *metadataScanner) processScanPath(ctx context.Context, path string, info fs.FileInfo, err error, options ScanOptions, sections map[string]map[string]any, result *models.MetadataScannerResult, mutex *sync.Mutex, errorGroup *errgroup.Group) error {
	if err != nil || info.IsDir() {
		return nil
	}

	filename := filepath.Base(path)
	if filename == "_index.md" {
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

	if filename == "404.md" {
		mutex.Lock()
		result.Has404 = true
		mutex.Unlock()
	}

	errorGroup.Go(func() error {
		scannedFile, err := service.ScanFile(options.SrcFs, options.Cfg, path)
		if err != nil {
			return nil
		}

		merged := service.applyDataCascade(scannedFile, sections)
		scannedFile.PreParsedMeta = merged

		if fileChan := options.FileChan; fileChan != nil {
			select {
			case fileChan <- scannedFile:
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		service.aggregateScanResult(result, scannedFile, mutex)
		return nil
	})

	return nil
}

// applyDataCascade merges section metadata into the scanned file's metadata.
func (service *metadataScanner) applyDataCascade(scannedFile models.ScannedResource, sections map[string]map[string]any) map[string]any {
	relDir := filepath.Dir(scannedFile.RelPath)
	if relDir == "." {
		relDir = ""
	}

	merged := make(map[string]any)
	pathParts := strings.Split(relDir, string(filepath.Separator))
	currentPath := ""

	for i := 0; i <= len(pathParts); i++ {
		if i > 0 && pathParts[i-1] == "" {
			continue
		}

		if sectionMeta, ok := sections[currentPath]; ok {
			if cascade, ok := sectionMeta["cascade"].(map[string]any); ok {
				for k, v := range cascade {
					merged[k] = v
				}
			}

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

	for k, v := range scannedFile.PreParsedMeta {
		merged[k] = v
	}

	if layout, ok := merged["layout"].(string); ok {
		scannedFile.Layout = layout
	}
	if title, ok := merged["title"].(string); ok && scannedFile.Title == "" {
		scannedFile.Title = title
	}

	return merged
}

// aggregateScanResult aggregates the scanned file into the result.
func (service *metadataScanner) aggregateScanResult(result *models.MetadataScannerResult, scannedFile models.ScannedResource, mutex *sync.Mutex) {
	mutex.Lock()
	defer mutex.Unlock()

	result.Files = append(result.Files, scannedFile)

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
}

// ScanFile scans a single markdown file for metadata.
func (service *metadataScanner) ScanFile(sourceFs afero.Fs, siteConfig *config.Config, path string) (models.ScannedResource, error) {
	fileData, bytesRead, err := service.readFileStart(sourceFs, path)
	if err != nil {
		return models.ScannedResource{}, err
	}

	info, err := sourceFs.Stat(path)
	if err != nil {
		return models.ScannedResource{}, err
	}

	relativePath, _ := filepath.Rel(siteConfig.ContentDir, path)
	frontmatter, bodyOffset, _ := service.extractFrontmatterAndBodyOffset(sourceFs, path, fileData, bytesRead)
	preparsedMetadata, _ := hashing.ParseFrontmatter(frontmatter)

	scanned := service.parseScannedMetadata(siteConfig, path, relativePath, preparsedMetadata)
	scanned.Info = info
	scanned.BodyOffset = bodyOffset
	scanned.SourceLoader = func() ([]byte, error) { return afero.ReadFile(sourceFs, path) }
	scanned.PreParsedMeta = preparsedMetadata

	return scanned, nil
}

func (service *metadataScanner) readFileStart(sourceFs afero.Fs, path string) ([]byte, int, error) {
	file, err := sourceFs.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = file.Close() }()
	buffer := make([]byte, scanBufferSize)
	bytesRead, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, 0, err
	}
	return buffer[:bytesRead], bytesRead, nil
}

func (service *metadataScanner) extractFrontmatterAndBodyOffset(sourceFs afero.Fs, path string, fileData []byte, bytesRead int) ([]byte, int, []byte) {
	parts := bytes.SplitN(fileData, hashing.YAMLDelim, frontmatterParts)
	if len(parts) < frontmatterParts && bytesRead == scanBufferSize {
		fullData, err := afero.ReadFile(sourceFs, path)
		if err == nil {
			fileData = fullData
			parts = bytes.SplitN(fileData, hashing.YAMLDelim, frontmatterParts)
		}
	}
	if len(parts) >= frontmatterParts {
		return bytes.TrimSpace(parts[1]), bytes.Index(fileData, parts[2]), fileData
	}
	return nil, 0, fileData
}

func (service *metadataScanner) parseScannedMetadata(siteConfig *config.Config, path, relativePath string, metadata map[string]any) models.ScannedResource {
	title := timeutil.ExtractStringFromMap(metadata, "title")
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}
	description := timeutil.ExtractStringFromMap(metadata, "description")
	date := timeutil.ExtractDateStringFromMap(metadata, "date")
	isDraft := timeutil.ExtractBoolFromMap(metadata, "draft")
	isPinned := timeutil.ExtractBoolFromMap(metadata, "pinned")
	weight := extractWeight(metadata)

	dateObj, _ := time.ParseInLocation("2006-01-02", date, time.UTC)

	taxonomies := make(map[string][]string)
	for taxKey := range siteConfig.Taxonomies {
		if terms := timeutil.ExtractSliceFromMap(metadata, taxKey); len(terms) > 0 {
			taxonomies[taxKey] = terms
		}
	}

	cleanHTMLRelPath := strings.TrimSuffix(relativePath, filepath.Ext(relativePath)) + ".html"
	postLink := navigation.BuildAbsoluteURL(siteConfig.BaseURL, cleanHTMLRelPath)

	frontmatterHash := hashing.GetFrontmatterHashFromValues(hashing.FrontmatterHashOptions{
		Title:       title,
		Description: description,
		Date:        date,
		Taxonomies:  taxonomies,
		IsPinned:    isPinned,
		IsDraft:     isDraft,
		Weight:      weight,
		Other:       metadata,
	})

	return models.ScannedResource{
		RelPath:         relativePath,
		Path:            path,
		Title:           title,
		Description:     description,
		Date:            date,
		DateObj:         dateObj,
		IsDraft:         isDraft,
		IsPinned:        isPinned,
		Weight:          weight,
		Taxonomies:      taxonomies,
		FrontmatterHash: frontmatterHash,
		Link:            postLink,
	}
}

func extractWeight(metadata map[string]any) int {
	if weightValue, ok := metadata["weight"].(int); ok {
		return weightValue
	} else if weightValue, ok := metadata["weight"].(float64); ok {
		return int(weightValue)
	}
	return 0
}
