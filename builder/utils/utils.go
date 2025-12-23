// Helper functions for file copying, image processing, and data sorting
package utils

import (
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
	"my-ssg/builder/models"
)

// CopyDir copies a directory recursively.
func CopyDir(src, dst string, compressImages bool) error {
	return filepath.Walk(src, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dst, relPath)
		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}
		
		// Check for modifications to skip copy if possible
		if destInfo, err := os.Stat(destPath); err == nil {
			if destInfo.ModTime().After(info.ModTime()) {
				return nil
			}
		}

		ext := strings.ToLower(filepath.Ext(path))
		if (ext == ".jpg" || ext == ".jpeg" || ext == ".png") && compressImages {
			return processImage(path, destPath, ext)
		}
		return CopyFileStandard(path, destPath)
	})
}

func processImage(srcPath, dstPath, ext string) error {
	src, err := imaging.Open(srcPath)
	if err != nil {
		return CopyFileStandard(srcPath, dstPath)
	}
	if src.Bounds().Dx() > 1200 {
		src = imaging.Resize(src, 1200, 0, imaging.Lanczos)
	}
	if ext == ".png" {
		return imaging.Save(src, dstPath, imaging.PNGCompressionLevel(png.BestCompression))
	}
	return imaging.Save(src, dstPath, imaging.JPEGQuality(75))
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