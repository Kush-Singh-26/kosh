package base

import (
	"fmt"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

// DeployCoreAssets writes all embedded core assets (JS, CSS, KaTeX fonts) to
// the artifact sink so they are available at well-known paths in the output.
func DeployCoreAssets(sink fspkg.ArtifactSink) error {
	// Deploy CSS
	if err := sink.WriteFile("static/css/kosh-core.css", KoshCoreCSS); err != nil {
		return fmt.Errorf("failed to deploy kosh-core.css: %w", err)
	}
	if err := sink.WriteFile("static/css/katex.min.css", KatexMinCSS); err != nil {
		return fmt.Errorf("failed to deploy katex.min.css: %w", err)
	}
	if err := sink.WriteFile("static/css/graph.css", GraphCSS); err != nil {
		return fmt.Errorf("failed to deploy graph.css: %w", err)
	}

	// Deploy KaTeX fonts
	for _, name := range KatexFontNames {
		data, err := katexFontsDir.ReadFile("css/fonts/" + name)
		if err != nil {
			return fmt.Errorf("failed to read KaTeX font %s: %w", name, err)
		}
		if err := sink.WriteFile("static/css/fonts/"+name, data); err != nil {
			return fmt.Errorf("failed to deploy KaTeX font %s: %w", name, err)
		}
	}

	// Deploy JS
	if err := sink.WriteFile("static/js/kosh-main.js", KoshMainJS); err != nil {
		return fmt.Errorf("failed to deploy kosh-main.js: %w", err)
	}
	if err := sink.WriteFile("static/js/kosh-search.js", KoshSearchJS); err != nil {
		return fmt.Errorf("failed to deploy kosh-search.js: %w", err)
	}
	if err := sink.WriteFile("static/js/graph.js", GraphJS); err != nil {
		return fmt.Errorf("failed to deploy graph.js: %w", err)
	}

	return nil
}
