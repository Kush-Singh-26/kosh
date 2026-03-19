package services

import (
	"bytes"
	"context"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/hashing"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
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
	return string(bytes.TrimSpace(match[1])) == "true"
}

// extractFrontmatterInt extracts an integer field from frontmatter
func extractFrontmatterInt(frontmatter []byte, re *regexp.Regexp) int {
	match := re.FindSubmatch(frontmatter)
	if len(match) < 2 {
		return 0
	}
	val, _ := strconv.Atoi(string(bytes.TrimSpace(match[1])))
	return val
}

// extractFrontmatterTags extracts tags from frontmatter [tag1, tag2]
func extractFrontmatterTags(frontmatter []byte) []string {
	match := tagsLineRegex.FindSubmatch(frontmatter)
	if len(match) < 2 {
		return nil
	}
	rawTags := string(match[1])
	parts := strings.Split(rawTags, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		tag := strings.Trim(strings.TrimSpace(p), "\"'")
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

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
	// Limit concurrency for filesystem walk/read
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
				return nil // Skip individual file errors
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

// Remove ScanVersioned entirely as it's unused and superseded by Scan

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

	// Extract frontmatter and body quickly
	parts := bytes.SplitN(data, yamlDelim, 3)
	var frontmatter, body []byte
	var bodyOffset int
	if len(parts) >= 3 {
		frontmatter = bytes.TrimSpace(parts[1])
		body = bytes.TrimSpace(parts[2])
		// Calculate offset for search snippet context
		bodyOffset = bytes.Index(data, parts[2])
	} else {
		body = bytes.TrimSpace(data)
		bodyOffset = 0
	}

	title, _ := extractFrontmatterField(frontmatter, titleRegex)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	description, _ := extractFrontmatterField(frontmatter, descriptionRegex)
	date, _ := extractFrontmatterField(frontmatter, dateRegex)
	draft := extractFrontmatterBool(frontmatter, draftRegex)
	pinned := extractFrontmatterBool(frontmatter, pinnedRegex)
	weight := extractFrontmatterInt(frontmatter, weightRegex)
	tags := extractFrontmatterTags(frontmatter)

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
		FrontmatterHash: frontmatterHash,
		BodyHash:        bodyHash,
		BodyOffset:      bodyOffset,
		Link:            postLink,
		Info:            info,
		Source:          data, // Pre-read source bytes
	}, nil
}
