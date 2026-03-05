package utils

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

// Global Minifier Instance
var Minifier *minify.M

func InitMinifier() {
	Minifier = minify.New()
	// Configure HTML minifier to keep end tags for tables
	// Without this, </td>, </th>, </tr> are stripped which breaks table rendering
	htmlMinifier := &html.Minifier{
		KeepEndTags: true,
	}
	Minifier.Add("text/html", htmlMinifier)
}

var imgRe = regexp.MustCompile(`(?i)(<img[^>]+src=["'])([^"']*)(["'])`)

func ProcessHTML(htmlStr string, baseURL string, prefix string, compress bool) string {
	return imgRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		parts := imgRe.FindStringSubmatch(m)
		if len(parts) < 4 {
			return m
		}
		src := parts[2]

		if src == "" || strings.HasPrefix(src, "http") || strings.HasPrefix(src, "//") || strings.HasPrefix(src, "data:") {
			return m
		}

		// 1. WebP conversion (only for local images)
		ext := strings.ToLower(filepath.Ext(src))
		if compress && (ext == ".jpg" || ext == ".jpeg" || ext == ".png") {
			src = src[:len(src)-len(ext)] + ".webp"
		}

		// 2. Path correction (Relativize or Prepend BaseURL)
		if strings.HasPrefix(src, "/") {
			if baseURL == "" {
				src = prefix + strings.TrimPrefix(src, "/")
			} else {
				src = baseURL + src
			}
		}

		// 3. Lowercase local images to match NormalizePath behavior on Windows
		// This ensures /static/images/CNN1.webp matches cnn1.webp
		if !strings.HasPrefix(src, "http") && !strings.HasPrefix(src, "//") {
			src = strings.ToLower(src)
		}

		return parts[1] + src + parts[3]
	})
}
