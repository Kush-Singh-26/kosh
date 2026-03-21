package fs

import (
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/svg"
)

var (
	globalMinifier     *minify.M
	globalMinifierOnce sync.Once
)

func InitMinifier() {
	GetMinifier()
}

func GetMinifier() *minify.M {
	globalMinifierOnce.Do(func() {
		globalMinifier = minify.New()
		htmlMinifier := &html.Minifier{
			KeepEndTags: true,
		}
		globalMinifier.Add("text/html", htmlMinifier)
		globalMinifier.AddFunc("image/svg+xml", svg.Minify)
	})
	return globalMinifier
}
