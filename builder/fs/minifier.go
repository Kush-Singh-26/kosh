package fs

import (
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
