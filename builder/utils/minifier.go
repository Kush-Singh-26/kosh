package utils

import (
	"regexp"
	"strings"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

// Global Minifier Instance
var Minifier *minify.M

func InitMinifier() {
	Minifier = minify.New()
	Minifier.AddFunc("text/html", html.Minify)
}

// NormalizeCacheKey converts a file path to a normalized cache key
// Uses forward slashes for cross-platform compatibility
func NormalizeCacheKey(path string) string {
	// Convert Windows backslashes to forward slashes
	return strings.ReplaceAll(path, "\\", "/")
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
