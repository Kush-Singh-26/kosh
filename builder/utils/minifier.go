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
	// Configure HTML minifier to keep end tags for tables
	// Without this, </td>, </th>, </tr> are stripped which breaks table rendering
	htmlMinifier := &html.Minifier{
		KeepEndTags: true,
	}
	Minifier.Add("text/html", htmlMinifier)
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
