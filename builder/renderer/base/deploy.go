package base

import (
	"fmt"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

// DeployCoreAssets writes all embedded core assets (JS, CSS, math font) to
// the artifact sink under their content-hashed filenames so repeat visitors
// can cache them immutably.
func DeployCoreAssets(sink fspkg.ArtifactSink) error {
	// Deploy CSS
	if err := sink.WriteFile("static/css/"+koshCoreCSSName, koshCoreCSS); err != nil {
		return fmt.Errorf("failed to deploy kosh-core.css: %w", err)
	}
	if err := sink.WriteFile("static/css/"+graphCSSName, graphCSS); err != nil {
		return fmt.Errorf("failed to deploy graph.css: %w", err)
	}

	// Deploy math font
	if err := sink.WriteFile("static/css/fonts/"+koshMathFontName, KoshMathFont); err != nil {
		return fmt.Errorf("failed to deploy math font: %w", err)
	}

	// Deploy JS
	if err := sink.WriteFile("static/js/"+koshMainJSName, koshMainJS); err != nil {
		return fmt.Errorf("failed to deploy kosh-main.js: %w", err)
	}
	if err := sink.WriteFile("static/js/"+koshSearchJSName, koshSearchJS); err != nil {
		return fmt.Errorf("failed to deploy kosh-search.js: %w", err)
	}
	if err := sink.WriteFile("static/js/"+graphJSName, graphJS); err != nil {
		return fmt.Errorf("failed to deploy graph.js: %w", err)
	}

	return nil
}
