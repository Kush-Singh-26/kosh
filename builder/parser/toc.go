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

func GetTOC(pc parser.Context) []models.TOCEntry {
	if v := pc.Get(tocKey); v != nil {
		return v.([]models.TOCEntry)
	}
	return nil
}

func GetPlainText(pc parser.Context) string {
	if v := pc.Get(plainTextKey); v != nil {
		return v.(string)
	}
	return ""
}

// GetSSRHashes returns all SSR input hashes (D2 diagrams, LaTeX math) for cache tracking
func GetSSRHashes(pc parser.Context) []string {
	if v := pc.Get(ssrHashesKey); v != nil {
		return v.([]string)
	}
	return nil
}

// AddSSRHash adds an SSR input hash to the context
func AddSSRHash(pc parser.Context, hash string) {
	var hashes []string
	if v := pc.Get(ssrHashesKey); v != nil {
		hashes = v.([]string)
	}
	hashes = append(hashes, hash)
	pc.Set(ssrHashesKey, hashes)
}
