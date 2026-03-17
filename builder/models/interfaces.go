package models

import (
	"html/template"
	"io"
)

// HTML is a type alias for template.HTML to avoid importing html/template everywhere
type HTML = template.HTML

// RenderService handles template rendering and HTML generation.
type RenderService interface {
	RenderIndex(path string, data PageData) error
	RenderPage(path string, data PageData) error
	RenderSidebar(tree []*TreeNode) HTML
	RegisterFile(path string)
}

// CacheService provides cache operations for site-wide generators.
type CacheService interface {
	GetSocialCardHash(path string) (string, error)
	SetSocialCardHash(path, hash string) error
}

// ArtifactSink is an interface for writing build artifacts.
// This matches the implementation in builder/utils/fs/sink.go
type ArtifactSink interface {
	WriteFile(path string, data []byte) error
	WriteStream(path string, writer func(io.Writer) error) error
	MkdirAll(path string) error
	Register(path string)
}
