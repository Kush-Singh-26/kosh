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
	// Case-insensitive check without full ToLower allocation
	hasImg := false
	for i := 0; i < len(htmlStr)-4; i++ {
		if htmlStr[i] == '<' &&
			(htmlStr[i+1] == 'i' || htmlStr[i+1] == 'I') &&
			(htmlStr[i+2] == 'm' || htmlStr[i+2] == 'M') &&
			(htmlStr[i+3] == 'g' || htmlStr[i+3] == 'G') {
			hasImg = true
			break
		}
	}
	if !hasImg {
		return htmlStr
	}
	return string(ProcessHTMLBytes([]byte(htmlStr), baseURL, prefix, compress))
}

func ProcessHTMLBytes(htmlBytes []byte, baseURL string, prefix string, compress bool) []byte {
	// Case-insensitive check without full ToLower allocation
	hasImg := false
	for i := 0; i < len(htmlBytes)-4; i++ {
		if htmlBytes[i] == '<' &&
			(htmlBytes[i+1] == 'i' || htmlBytes[i+1] == 'I') &&
			(htmlBytes[i+2] == 'm' || htmlBytes[i+2] == 'M') &&
			(htmlBytes[i+3] == 'g' || htmlBytes[i+3] == 'G') {
			hasImg = true
			break
		}
	}
	if !hasImg {
		return htmlBytes
	}

	return imgRe.ReplaceAllFunc(htmlBytes, func(m []byte) []byte {
		parts := imgRe.FindSubmatch(m)
		if len(parts) < 4 {
			return m
		}
		src := string(parts[2])

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
		if !strings.HasPrefix(src, "http") && !strings.HasPrefix(src, "//") {
			src = strings.ToLower(src)
		}

		res := make([]byte, 0, len(parts[1])+len(src)+len(parts[3]))
		res = append(res, parts[1]...)
		res = append(res, src...)
		res = append(res, parts[3]...)
		return res
	})
}
