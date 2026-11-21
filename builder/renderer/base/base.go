package base

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/css"
	"github.com/tdewolff/minify/v2/js"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
)

// BaseTemplate contains the embedded base HTML template.
//
//go:embed base.html
var BaseTemplate string

//go:embed default-404.html
var Default404Template string

// Core CSS assets
//
//go:embed css/kosh-core.css
var KoshCoreCSS []byte

// KoshMathFont is the subset math font referenced by kosh-core.css.
//
//go:embed css/fonts/NewCMMath-Subset.woff2
var KoshMathFont []byte

// Graph page CSS
//
//go:embed css/graph.css
var GraphCSS []byte

// Core JS assets
//
//go:embed js/kosh-main.js
var KoshMainJS []byte

//go:embed js/kosh-search.js
var KoshSearchJS []byte

//go:embed js/graph.js
var GraphJS []byte

const assetHashLength = 8

// assetName returns "<name>.<content-hash>.<ext>", matching the content-hashed
// naming used for theme assets so hashes survive binary-only changes.
func assetName(name string, content []byte, ext string) string {
	return fmt.Sprintf("%s.%s.%s", name, core.HashContent(content)[:assetHashLength], ext)
}

// minifyWithFallback minifies src, falling back to the raw embed when the
// minifier fails — better a larger file than a broken one.
func minifyWithFallback(mimetype string, src []byte, minifier func(*minify.M)) []byte {
	m := minify.New()
	minifier(m)
	out, err := m.String(mimetype, string(src))
	if err != nil {
		return src
	}
	return []byte(out)
}

func minifyCSS(src []byte) []byte {
	return minifyWithFallback("text/css", src, func(m *minify.M) {
		m.AddFunc("text/css", css.Minify)
	})
}

func minifyJS(src []byte) []byte {
	return minifyWithFallback("application/javascript", src, func(m *minify.M) {
		m.AddFunc("application/javascript", js.Minify)
	})
}

// koshMathFontName is the content-hashed filename for the math font.
var koshMathFontName = assetName("NewCMMath-Subset", KoshMathFont, "woff2")

// koshCoreCSS is the minified core stylesheet with the @font-face URL rewritten
// to the hashed font filename, so CSS and font cache-bust together.
var koshCoreCSS = bytes.ReplaceAll(minifyCSS(KoshCoreCSS),
	[]byte("fonts/NewCMMath-Subset.woff2"), []byte("fonts/"+koshMathFontName))

var koshCoreCSSName = assetName("kosh-core", koshCoreCSS, "css")

// graphCSS is the minified graph-page stylesheet.
var graphCSS = minifyCSS(GraphCSS)
var graphCSSName = assetName("graph", graphCSS, "css")

// koshMainJS/koshSearchJS/graphJS are the minified base scripts.
var koshMainJS = minifyJS(KoshMainJS)
var koshMainJSName = assetName("kosh-main", koshMainJS, "js")

var koshSearchJS = minifyJS(KoshSearchJS)
var koshSearchJSName = assetName("kosh-search", koshSearchJS, "js")

var graphJS = minifyJS(GraphJS)
var graphJSName = assetName("graph", graphJS, "js")

// BaseAssets maps the legacy well-known asset paths to the content-hashed paths
// deployed in output. Keys use the theme-hashed-asset conventions so templates
// can index them uniformly.
func BaseAssets() map[string]string {
	return map[string]string{
		"/static/css/kosh-core.css": "/static/css/" + koshCoreCSSName,
		"/static/css/graph.css":     "/static/css/" + graphCSSName,
		"/static/js/kosh-main.js":   "/static/js/" + koshMainJSName,
		"/static/js/kosh-search.js": "/static/js/" + koshSearchJSName,
		"/static/js/graph.js":       "/static/js/" + graphJSName,
	}
}
