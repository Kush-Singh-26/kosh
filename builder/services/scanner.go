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
	Source          []byte // Pre-read source bytes to avoid double-read
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

	workerCount := runtime.NumCPU()
	if workerCount > 8 {
		workerCount = 8 // Cap scanning workers to prevent I/O saturation
	}
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

				meta := s.extractFrontmatter(source, relPath, t.version, cleanHtmlRelPath, htmlRelPath, cfg)
				if meta.Path == "" {
					continue
				}

				sf := ScannedFile{
					Path:            t.path,
					Version:         t.version,
					Info:            t.info,
					BodyHash:        utils.GetBodyHash(source),
					FrontmatterHash: mustFrontmatterHash(source),
					Source:          source,
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

func mustFrontmatterHash(source []byte) string {
	h, _ := utils.GetFrontmatterHashFromSource(source)
	return h
}

func (s *metadataScanner) extractFrontmatter(source []byte, relPath, version, cleanHtmlRelPath, htmlRelPath string, cfg *config.Config) LightPostMetadata {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) < 3 {
		return LightPostMetadata{}
	}

	frontmatter := bytes.TrimSpace(parts[1])
	fm, err := parseFrontmatterFast(frontmatter)
	if err != nil || fm == nil {
		return LightPostMetadata{}
	}

	dateStr := fm.Date
	dateObj, _ := time.Parse("2006-01-02", dateStr)

	isPinned := fm.Pinned
	weight := fm.Weight
	isDraft := fm.Draft

	// Calculate word count from the body (part 3)
	wordCount := utils.CountWords(parts[2])
	readingTime := int(math.Ceil(float64(wordCount) / wordsPerMinute))

	postLink := utils.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)

	return LightPostMetadata{
		Path:        relPath,
		Version:     version,
		Title:       fm.Title,
		DateObj:     dateObj,
		Tags:        fm.Tags,
		Pinned:      isPinned,
		Weight:      weight,
		ReadingTime: readingTime,
		Draft:       isDraft,
		Description: fm.Description,
		Link:        postLink,
		HTMLPath:    htmlRelPath,
	}
}

type frontmatterLite struct {
	Title       string   `yaml:"title"`
	Date        string   `yaml:"date"`
	Tags        []string `yaml:"tags"`
	Pinned      bool     `yaml:"pinned"`
	Weight      int      `yaml:"weight"`
	Draft       bool     `yaml:"draft"`
	Description string   `yaml:"description"`
}

func parseFrontmatterFast(data []byte) (*frontmatterLite, error) {
	if len(data) == 0 {
		return nil, nil
	}
	fm := &frontmatterLite{}
	if err := yaml.Unmarshal(data, fm); err != nil {
		return nil, err
	}
	return fm, nil
}
