package fs

import (
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
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
	})
	return globalMinifier
}
