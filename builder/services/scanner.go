package services

import (
	"bytes"
	"context"
	"io/fs"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

var yamlDelim = []byte("---")

type LightPostMetadata struct {
	Path        string
	Version     string
	Title       string
	DateObj     time.Time
	Tags        []string
	Pinned      bool
	Weight      int
	ReadingTime int
	Draft       bool
	Description string
	Link        string
	HTMLPath    string
}

type MetadataScannerResult struct {
	Metadata       []LightPostMetadata
	TagMap         map[string][]LightPostMetadata
	PostsByVersion map[string][]LightPostMetadata
	Files          []ScannedFile
	ContentAssets  []ScannedAsset
	Has404         bool
}

// ScannedFile carries minimal file info to avoid a second filesystem walk in post processing.
type ScannedFile struct {
	Path            string
	Version         string
	Info            fs.FileInfo
	BodyHash        string
	FrontmatterHash string
	ReadingTime     int
	Source          []byte         // Pre-read source bytes to avoid double-read
	PreParsedMeta   map[string]any // Pre-parsed frontmatter to avoid double-parse
}

type ScannedAsset struct {
	Path string
	Info fs.FileInfo
}

type MetadataScanner interface {
	Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config) (*MetadataScannerResult, error)
}

type metadataScanner struct{}

func NewMetadataScanner() MetadataScanner {
	return &metadataScanner{}
}

func (s *metadataScanner) Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config) (*MetadataScannerResult, error) {
	var (
		mu             sync.Mutex
		has404         bool
		assets         []ScannedAsset
		scannedFiles   []ScannedFile
		tagMap         = make(map[string][]LightPostMetadata)
		postsByVersion = make(map[string][]LightPostMetadata)
		allMetadata    []LightPostMetadata
	)

	workerCount := min(runtime.NumCPU(),
		// Cap scanning workers to prevent I/O saturation
		8)
	g, gCtx := errgroup.WithContext(ctx)

	// Task channel for parsing workers
	type scanTask struct {
		path    string
		version string
		info    fs.FileInfo
	}
	tasks := make(chan scanTask, 1024)

	// Start parsing workers
	for i := 0; i < workerCount; i++ {
		g.Go(func() error {
			for t := range tasks {
				relPath, err := utils.SafeRel(contentDir, t.path)
				if err != nil {
					continue
				}
				htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

				cleanHtmlRelPath := htmlRelPath
				versionLower := strings.ToLower(t.version)
				if t.version != "" {
					cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, versionLower+"/")
				}

				source, err := afero.ReadFile(sourceFs, t.path)
				if err != nil {
					continue
				}

				meta, preParsedMeta, frontmatterHash, readingTime := s.extractFrontmatter(source, relPath, t.version, cleanHtmlRelPath, htmlRelPath, cfg)
				if meta.Path == "" {
					continue
				}

				sf := ScannedFile{
					Path:            t.path,
					Version:         t.version,
					Info:            t.info,
					BodyHash:        utils.GetBodyHash(source),
					FrontmatterHash: frontmatterHash,
					ReadingTime:     readingTime,
					Source:          source,
					PreParsedMeta:   preParsedMeta,
				}

				mu.Lock()
				allMetadata = append(allMetadata, meta)
				postsByVersion[meta.Version] = append(postsByVersion[meta.Version], meta)
				for _, tag := range meta.Tags {
					key := strings.ToLower(strings.TrimSpace(tag))
					tagMap[key] = append(tagMap[key], meta)
				}
				scannedFiles = append(scannedFiles, sf)
				mu.Unlock()
			}
			return nil
		})
	}

	// Run discovery walk
	g.Go(func() error {
		defer close(tasks)
		return utils.ParallelWalk(gCtx, sourceFs, contentDir, 0, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info == nil {
				return nil
			}
			if !info.IsDir() {
				if strings.HasSuffix(strings.ToLower(path), ".md") && !strings.Contains(path, "_index.md") {
					if strings.Contains(path, "404.md") {
						mu.Lock()
						has404 = true
						mu.Unlock()
						return nil
					}
					ver, _ := utils.GetVersionFromPath(path)
					select {
					case tasks <- scanTask{path, ver, info}:
					case <-gCtx.Done():
						return gCtx.Err()
					}

					// If raw markdown is enabled, also treat it as an asset to be copied
					if cfg.Features.RawMarkdown {
						mu.Lock()
						assets = append(assets, ScannedAsset{Path: path, Info: info})
						mu.Unlock()
					}
					return nil
				}
				mu.Lock()
				assets = append(assets, ScannedAsset{Path: path, Info: info})
				mu.Unlock()
			}
			return nil
		})
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &MetadataScannerResult{
		Metadata:       allMetadata,
		TagMap:         tagMap,
		PostsByVersion: postsByVersion,
		Files:          scannedFiles,
		ContentAssets:  assets,
		Has404:         has404,
	}, nil
}

func (s *metadataScanner) extractFrontmatter(source []byte, relPath, version, cleanHtmlRelPath, htmlRelPath string, cfg *config.Config) (LightPostMetadata, map[string]any, string, int) {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) < 3 {
		return LightPostMetadata{}, nil, "", 0
	}

	frontmatter := bytes.TrimSpace(parts[1])
	var fmMap map[string]any
	if err := yaml.Unmarshal(frontmatter, &fmMap); err != nil {
		return LightPostMetadata{}, nil, "", 0
	}

	title := utils.GetString(fmMap, "title")
	description := utils.GetString(fmMap, "description")
	dateStr := utils.GetString(fmMap, "date")
	tags := utils.GetSlice(fmMap, "tags")
	isPinned := utils.GetBool(fmMap, "pinned")
	weight, _ := fmMap["weight"].(int)
	if w, ok := fmMap["weight"].(float64); ok && weight == 0 {
		weight = int(w)
	}
	isDraft := utils.GetBool(fmMap, "draft")

	dateObj, _ := time.Parse("2006-01-02", dateStr)

	// Calculate word count from the body (part 3)
	wordCount := utils.CountWords(parts[2])
	readingTime := int(math.Ceil(float64(wordCount) / wordsPerMinute))

	postLink := utils.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)

	frontmatterHash := utils.GetFrontmatterHashFromValues(
		title,
		description,
		dateStr,
		tags,
		isPinned,
	)

	return LightPostMetadata{
		Path:        relPath,
		Version:     version,
		Title:       title,
		DateObj:     dateObj,
		Tags:        tags,
		Pinned:      isPinned,
		Weight:      weight,
		ReadingTime: readingTime,
		Draft:       isDraft,
		Description: description,
		Link:        postLink,
		HTMLPath:    htmlRelPath,
	}, fmMap, frontmatterHash, readingTime
}
