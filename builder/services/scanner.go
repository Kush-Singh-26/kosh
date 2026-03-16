package services

import (
	"bytes"
	"context"
	"io/fs"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

var yamlDelim = []byte("---")

// Lightweight frontmatter field extractors using regex/string parsing
// These avoid full YAML unmarshaling for the common case of needing only a few fields
var (
	titleRegex       = regexp.MustCompile(`(?m)^title:\s*["']?([^"'\n]+)["']?\s*$`)
	descriptionRegex = regexp.MustCompile(`(?m)^description:\s*["']?([^"'\n]+)["']?\s*$`)
	dateRegex        = regexp.MustCompile(`(?m)^date:\s*["']?(\d{4}-\d{2}-\d{2})["']?\s*$`)
	draftRegex       = regexp.MustCompile(`(?m)^draft:\s*(true|false)\s*$`)
	pinnedRegex      = regexp.MustCompile(`(?m)^pinned:\s*(true|false)\s*$`)
	weightRegex      = regexp.MustCompile(`(?m)^weight:\s*(\d+)\s*$`)
	tagsLineRegex    = regexp.MustCompile(`(?m)^tags:\s*\[(.*?)\]\s*$`)
)

// extractFrontmatterField extracts a single string field from frontmatter using regex
func extractFrontmatterField(frontmatter []byte, re *regexp.Regexp) (string, bool) {
	match := re.FindSubmatch(frontmatter)
	if len(match) < 2 {
		return "", false
	}
	return string(bytes.TrimSpace(match[1])), true
}

// extractFrontmatterBool extracts a boolean field from frontmatter
func extractFrontmatterBool(frontmatter []byte, re *regexp.Regexp) bool {
	match := re.FindSubmatch(frontmatter)
	if len(match) < 2 {
		return false
	}
	return strings.ToLower(string(bytes.TrimSpace(match[1]))) == "true"
}

// extractFrontmatterInt extracts an integer field from frontmatter
func extractFrontmatterInt(frontmatter []byte, re *regexp.Regexp) (int, bool) {
	match := re.FindSubmatch(frontmatter)
	if len(match) < 2 {
		return 0, false
	}
	val, err := strconv.Atoi(string(bytes.TrimSpace(match[1])))
	if err != nil {
		return 0, false
	}
	return val, true
}

// extractTagsSimple extracts tags from frontmatter using simple string parsing
// Handles formats: tags: [tag1, tag2] or tags: [tag1,tag2]
func extractTagsSimple(frontmatter []byte) []string {
	match := tagsLineRegex.FindSubmatch(frontmatter)
	if len(match) < 2 {
		return nil
	}
	tagsStr := string(bytes.TrimSpace(match[1]))
	if tagsStr == "" {
		return nil
	}
	parts := strings.Split(tagsStr, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		tag := strings.TrimSpace(p)
		tag = strings.Trim(tag, `"'`)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

type MetadataScanner interface {
	Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error)
}

type metadataScanner struct{}

func NewMetadataScanner() MetadataScanner {
	return &metadataScanner{}
}

func (s *metadataScanner) Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error) {
	var (
		mu             sync.Mutex
		has404         bool
		assets         []models.ScannedAsset
		scannedFiles   []models.ScannedFile
		tagMap         = make(map[string][]models.LightPostMetadata)
		postsByVersion = make(map[string][]models.LightPostMetadata)
		allMetadata    []models.LightPostMetadata
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
				relPath, err := fspkg.SafeRel(contentDir, t.path)
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

				meta, preParsedMeta, frontmatterHash, bodyHash, readingTime, bodyOffset := s.extractFrontmatter(source, relPath, t.version, cleanHtmlRelPath, htmlRelPath, cfg)
				if meta.Path == "" {
					continue
				}

				sf := models.ScannedFile{
					Path:            t.path,
					Version:         t.version,
					Info:            t.info,
					BodyHash:        bodyHash,
					FrontmatterHash: frontmatterHash,
					ReadingTime:     readingTime,
					BodyOffset:      bodyOffset,
					Source:          source,
					PreParsedMeta:   preParsedMeta,
				}

				if fileChan != nil {
					select {
					case fileChan <- sf:
					case <-gCtx.Done():
						return gCtx.Err()
					}
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

	// Run discovery walk with optimized concurrency
	g.Go(func() error {
		defer close(tasks)
		// Use higher concurrency for discovery on modern SSDs
		walkConcurrency := max(workerCount*2, 8)
		if walkConcurrency > 32 {
			walkConcurrency = 32
		}
		return utils.ParallelWalk(gCtx, sourceFs, contentDir, walkConcurrency, func(path string, info fs.FileInfo, err error) error {
			if err != nil || info == nil {
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
						assets = append(assets, models.ScannedAsset{Path: path, Info: info})
						mu.Unlock()
					}
					return nil
				}
				mu.Lock()
				assets = append(assets, models.ScannedAsset{Path: path, Info: info})
				mu.Unlock()
			}
			return nil
		})
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &models.MetadataScannerResult{
		Metadata:       allMetadata,
		TagMap:         tagMap,
		PostsByVersion: postsByVersion,
		Files:          scannedFiles,
		ContentAssets:  assets,
		Has404:         has404,
	}, nil
}

func (s *metadataScanner) extractFrontmatter(source []byte, relPath, version, cleanHtmlRelPath, htmlRelPath string, cfg *config.Config) (models.LightPostMetadata, map[string]any, string, string, int, int) {
	parts := bytes.SplitN(source, yamlDelim, 3)
	if len(parts) < 3 {
		return models.LightPostMetadata{}, nil, "", "", 0, 0
	}

	frontmatter := bytes.TrimSpace(parts[1])

	// Optimized: Use lightweight regex/string extraction for common fields
	// This avoids full YAML unmarshaling overhead for the common case
	title, _ := extractFrontmatterField(frontmatter, titleRegex)
	description, _ := extractFrontmatterField(frontmatter, descriptionRegex)
	dateStr, _ := extractFrontmatterField(frontmatter, dateRegex)
	tags := extractTagsSimple(frontmatter)
	isPinned := extractFrontmatterBool(frontmatter, pinnedRegex)
	isDraft := extractFrontmatterBool(frontmatter, draftRegex)
	weight, _ := extractFrontmatterInt(frontmatter, weightRegex)

	dateObj, _ := time.Parse("2006-01-02", dateStr)

	// Optimized: Defer word count/reading time to post processing to allow cache reuse
	readingTime := 0

	body := bytes.TrimSpace(parts[2])
	bodyHash := utils.HashBytes(body)

	// Compute body offset relative to source
	bodyOffset := len(parts[0]) + len(yamlDelim) + len(parts[1]) + len(yamlDelim)

	postLink := utils.BuildURL(cfg.BaseURL, version, cleanHtmlRelPath)

	frontmatterHash := utils.GetFrontmatterHashFromValues(
		title,
		description,
		dateStr,
		tags,
		isPinned,
	)

	// Build minimal fmMap for compatibility with downstream code
	// Only include fields that were successfully extracted
	fmMap := make(map[string]any)
	if title != "" {
		fmMap["title"] = title
	}
	if description != "" {
		fmMap["description"] = description
	}
	if dateStr != "" {
		fmMap["date"] = dateStr
	}
	if len(tags) > 0 {
		fmMap["tags"] = tags
	}
	if isPinned {
		fmMap["pinned"] = true
	}
	if isDraft {
		fmMap["draft"] = true
	}
	if weight != 0 {
		fmMap["weight"] = weight
	}

	return models.LightPostMetadata{
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
	}, fmMap, frontmatterHash, bodyHash, readingTime, bodyOffset
}
