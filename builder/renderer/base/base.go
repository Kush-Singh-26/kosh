package base

import (
	"embed"
	"io/fs"
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

//go:embed css/katex.min.css
var KatexMinCSS []byte

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

// KaTeX fonts (embedded as embed.FS for dynamic read access)
//
//go:embed css/fonts
var katexFontsDir embed.FS

// KatexFontNames is the list of all embedded KaTeX font filenames.
var KatexFontNames []string

func init() {
	entries, err := fs.ReadDir(katexFontsDir, "css/fonts")
	if err != nil {
		panic("failed to read embedded KaTeX fonts directory: " + err.Error())
	}
	for _, e := range entries {
		if !e.IsDir() {
			KatexFontNames = append(KatexFontNames, e.Name())
		}
	}
}
