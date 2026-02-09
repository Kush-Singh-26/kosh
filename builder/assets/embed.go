package assets

import "embed"

//go:embed fonts/*.ttf
var fontsFS embed.FS

// GetFont returns the bytes for a requested font file from the embedded FS.
func GetFont(filename string) ([]byte, error) {
	return fontsFS.ReadFile("fonts/" + filename)
}
