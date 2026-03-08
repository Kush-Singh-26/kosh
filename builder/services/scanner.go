package services

import (
	"bytes"
	"context"
	"io/fs"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
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
    Has404         bool
}

// ScannedFile carries minimal file info to avoid a second filesystem walk in post processing.
type ScannedFile struct {
    Path    string
    Version string
    Info    fs.FileInfo
}

type MetadataScanner interface {
	Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config) (*MetadataScannerResult, error)
}

type metadataScanner struct{}

func NewMetadataScanner() MetadataScanner {
	return &metadataScanner{}
}

type scanResult struct {
	idx  int
	meta LightPostMetadata
}

func (s *metadataScanner) Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config) (*MetadataScannerResult, error) {
    var files []string
    var fileVersions []string
    var fileInfos []fs.FileInfo
    var has404 bool

    if err := afero.Walk(sourceFs, contentDir, func(path string, info fs.FileInfo, err error) error {
        if err != nil {
            return nil
        }
        if !info.IsDir() && strings.HasSuffix(path, ".md") && !strings.Contains(path, "_index.md") {
            if strings.Contains(path, "404.md") {
                has404 = true
                return nil
            }
            ver, _ := utils.GetVersionFromPath(path)
            files = append(files, path)
            fileVersions = append(fileVersions, ver)
            fileInfos = append(fileInfos, info)
        }
        return nil
    }); err != nil {
        return nil, err
    }

    results := make([]scanResult, len(files))
    var wg sync.WaitGroup
    workerCount := utils.GetDefaultWorkerCount()

	jobs := make(chan int, len(files))
	resultsChan := make(chan scanResult, len(files))

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				path := files[idx]
				version := fileVersions[idx]

				relPath, _ := utils.SafeRel(contentDir, path)
				htmlRelPath := strings.ToLower(strings.Replace(relPath, ".md", ".html", 1))

				cleanHtmlRelPath := htmlRelPath
				versionLower := strings.ToLower(version)
				if version != "" {
					cleanHtmlRelPath = strings.TrimPrefix(htmlRelPath, versionLower+"/")
				}

				source, err := afero.ReadFile(sourceFs, path)
				if err != nil {
					continue
				}

				meta := s.extractFrontmatter(source, relPath, version, cleanHtmlRelPath, htmlRelPath, cfg)
				resultsChan <- scanResult{idx: idx, meta: meta}
			}
		}()
	}

	for i := range files {
		jobs <- i
	}
	close(jobs)

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for r := range resultsChan {
		results[r.idx] = r
	}

	tagMap := make(map[string][]LightPostMetadata)
	postsByVersion := make(map[string][]LightPostMetadata)

    scannedFiles := make([]ScannedFile, 0, len(files))
    for i, r := range results {
        meta := r.meta
        if meta.Path != "" {
            postsByVersion[meta.Version] = append(postsByVersion[meta.Version], meta)
            for _, t := range meta.Tags {
                key := strings.ToLower(strings.TrimSpace(t))
                tagMap[key] = append(tagMap[key], meta)
            }
            scannedFiles = append(scannedFiles, ScannedFile{Path: files[i], Version: fileVersions[i], Info: fileInfos[i]})
        }
    }

    return &MetadataScannerResult{
        Metadata:       extractMetadataSlice(results),
        TagMap:         tagMap,
        PostsByVersion: postsByVersion,
        Files:          scannedFiles,
        Has404:         has404,
    }, nil
}

func extractMetadataSlice(results []scanResult) []LightPostMetadata {
	metadata := make([]LightPostMetadata, 0, len(results))
	for _, r := range results {
		if r.meta.Path != "" {
			metadata = append(metadata, r.meta)
		}
	}
	return metadata
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
