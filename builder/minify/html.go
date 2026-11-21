package minify

import (
	"regexp"
	"sync"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/html"
	"github.com/tdewolff/minify/v2/js"
	"github.com/tdewolff/minify/v2/svg"
)

var (
	globalHTMLMinifier     *minify.M
	globalHTMLMinifierOnce sync.Once
	jsMimeTypeRe           = regexp.MustCompile("^(application|text)/(x-)?(java|ecma)script$")
)

// InitHTMLMinifier initializes the shared HTML minifier.
func InitHTMLMinifier() {
	GetHTMLMinifier()
}

// GetHTMLMinifier returns the initialized HTML minifier.
func GetHTMLMinifier() *minify.M {
	globalHTMLMinifierOnce.Do(func() {
		globalHTMLMinifier = minify.New()
		htmlMinifier := &html.Minifier{
			KeepEndTags: true,
		}
		globalHTMLMinifier.Add("text/html", htmlMinifier)
		globalHTMLMinifier.AddFunc("text/css", css.Minify)
		globalHTMLMinifier.AddFuncRegexp(jsMimeTypeRe, js.Minify)
		globalHTMLMinifier.AddFunc("image/svg+xml", svg.Minify)
	})
	return globalHTMLMinifier
}
