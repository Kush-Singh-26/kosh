package minify

import (
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/svg"
)

var (
	globalHTMLMinifier     *minify.M
	globalHTMLMinifierOnce sync.Once
)

func InitHTMLMinifier() {
	GetHTMLMinifier()
}

func GetHTMLMinifier() *minify.M {
	globalHTMLMinifierOnce.Do(func() {
		globalHTMLMinifier = minify.New()
		htmlMinifier := &html.Minifier{
			KeepEndTags: true,
		}
		globalHTMLMinifier.Add("text/html", htmlMinifier)
		globalHTMLMinifier.AddFunc("image/svg+xml", svg.Minify)
	})
	return globalHTMLMinifier
}
