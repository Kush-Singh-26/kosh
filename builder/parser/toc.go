package parser

import (
	"github.com/yuin/goldmark/parser"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// Package-level context keys, isolated by being unexported.
var (
	tocKey          = parser.NewContextKey()
	plainTextKey    = parser.NewContextKey()
	ssrHashesKey    = parser.NewContextKey()
	contextKeyBuild = parser.NewContextKey()
)

// GetTOC returns the table of contents from the parser context.
func GetTOC(pc parser.Context) []models.TOCEntry {
	if v := pc.Get(tocKey); v != nil {
		if entries, ok := v.([]models.TOCEntry); ok {
			return entries
		}
	}
	return nil
}

// GetPlainText returns the extracted plain text from the parser context.
func GetPlainText(pc parser.Context) string {
	if v := pc.Get(plainTextKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetSSRHashes returns all SSR input hashes (D2 diagrams, LaTeX math) for cache tracking
func GetSSRHashes(pc parser.Context) []string {
	if v := pc.Get(ssrHashesKey); v != nil {
		if hashes, ok := v.([]string); ok {
			return hashes
		}
	}
	return nil
}

// AddSSRHash adds an SSR input hash to the context
func AddSSRHash(pc parser.Context, hash string) {
	var hashes []string
	if v := pc.Get(ssrHashesKey); v != nil {
		if existing, ok := v.([]string); ok {
			hashes = existing
		}
	}
	hashes = append(hashes, hash)
	pc.Set(ssrHashesKey, hashes)
}
