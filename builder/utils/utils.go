// Helper functions for file copying, image processing, and data sorting
package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"my-ssg/builder/models"
)

// Global Minifier Instance
var Minifier *minify.M

func InitMinifier() {
	Minifier = minify.New()
	Minifier.AddFunc("text/html", html.Minify)
	Minifier.AddFunc("text/css", css.Minify)
	Minifier.AddFunc("text/javascript", js.Minify)
}

// CopyDir copies a directory recursively with incremental build support.
func CopyDir(src, dst string, compress bool) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		ext := strings.ToLower(filepath.Ext(path))
		isImage := (ext == ".jpg" || ext == ".jpeg" || ext == ".png")

		// 1. Determine the Final Destination Path
		if compress && isImage {
			extLen := len(filepath.Ext(destPath))
			destPath = destPath[:len(destPath)-extLen] + ".webp"
		}

		// 2. Incremental Build Check
		if destInfo, err := os.Stat(destPath); err == nil {
			if destInfo.ModTime().After(info.ModTime()) {
				return nil
			}
		}

		// 3. Process Files (Minify or Convert)
		if compress {
			if ext == ".css" {
				return minifyFile("text/css", path, destPath)
			}
			if ext == ".js" {
				return minifyFile("text/javascript", path, destPath)
			}
			if isImage {
				return processImage(path, destPath)
			}
		}

		// Fallback: Standard Copy
		return CopyFileStandard(path, destPath)
	})
}

func minifyFile(mediaType, srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	return Minifier.Minify(mediaType, dst, src)
}

func processImage(srcPath, dstPath string) error {
	src, err := imaging.Open(srcPath)
	if err != nil {
		return err
	}

	if src.Bounds().Dx() > 1200 {
		src = imaging.Resize(src, 1200, 0, imaging.Lanczos)
	}

	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return webp.Encode(f, src, &webp.Options{Lossless: false, Quality: 80})
}

func CopyFileStandard(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer d.Close()
	_, err = io.Copy(d, s)
	return err
}

var imgRe = regexp.MustCompile(`(?i)(<img[^>]+src=["'])([^"']+)((?:\.jpg|\.jpeg|\.png))(["'])`)

func ReplaceToWebP(html string) string {
	return imgRe.ReplaceAllStringFunc(html, func(m string) string {
		parts := imgRe.FindStringSubmatch(m)
		if strings.HasPrefix(parts[2], "http") || strings.HasPrefix(parts[2], "//") {
			return m
		}
		return parts[1] + parts[2] + ".webp" + parts[4]
	})
}

func SortPosts(posts []models.PostMetadata) {
	sort.Slice(posts, func(i, j int) bool { return posts[i].DateObj.After(posts[j].DateObj) })
}

func GetString(m map[string]interface{}, k string) string {
	if v, ok := m[k]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func GetSlice(m map[string]interface{}, k string) []string {
	var res []string
	if v, ok := m[k]; ok {
		if l, ok := v.([]interface{}); ok {
			for _, i := range l {
				res = append(res, fmt.Sprintf("%v", i))
			}
		}
	}
	return res
}

type SocialCardCache struct {
	Hashes    map[string]string `json:"hashes"`
	GraphHash string            `json:"graph_hash"`
}

func NewSocialCardCache() *SocialCardCache {
	return &SocialCardCache{
		Hashes: make(map[string]string),
	}
}

func LoadSocialCardCache(path string) (*SocialCardCache, error) {
	cache := NewSocialCardCache()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cache, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(data, cache); err != nil {
		return nil, err
	}

	return cache, nil
}

func SaveSocialCardCache(path string, cache *SocialCardCache) error {
	os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func GetFrontmatterHash(metaData map[string]interface{}) (string, error) {
	socialMeta := map[string]interface{}{
		"title":       metaData["title"],
		"description": metaData["description"],
		"date":        metaData["date"],
		"tags":        metaData["tags"],
		"pinned":      metaData["pinned"],
	}

	data, err := json.Marshal(socialMeta)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

type GraphHashData struct {
	Posts []PostGraphInfo `json:"posts"`
}

type PostGraphInfo struct {
	Title string   `json:"title"`
	Link  string   `json:"link"`
	Tags  []string `json:"tags"`
}

func GetGraphHash(posts []models.PostMetadata) (string, error) {
	graphInfo := make([]PostGraphInfo, 0, len(posts))
	for _, p := range posts {
		graphInfo = append(graphInfo, PostGraphInfo{
			Title: p.Title,
			Link:  p.Link,
			Tags:  p.Tags,
		})
	}

	data, err := json.Marshal(graphInfo)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
