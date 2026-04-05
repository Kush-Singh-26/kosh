package models

import (
	"html/template"
	"io"
	"time"
)

// ArtifactSink is an interface for writing build artifacts.
// Mirrors fs.ArtifactSink for use by models-layer consumers.
type ArtifactSink interface {
	WriteFile(path string, data []byte) error
	WriteStream(path string, writer func(io.Writer) error) error
	CopyFile(srcPath, destPath string) error
	MkdirAll(path string) error
	Register(path string)
	GetWrittenFiles() map[string]bool
	GetOutputDir() string
	SetMtime(path string, mtime time.Time) error
}

// HTML is a type alias for template.HTML to avoid importing html/template everywhere
type HTML = template.HTML

// RenderService handles template rendering and HTML generation.
// Mirrors render.Service for use by models-layer consumers.
type RenderService interface {
	RenderPage(path string, data PageData) error
	RenderIndex(path string, data PageData) error
	Render404(path string, data PageData) error
	RenderGraph(path string, data PageData) error
	RegisterFile(path string)
	SetAssets(assets map[string]string)
	GetAssets() map[string]string
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
	ReloadTemplates()
	Has404Template() bool
	SetAssetsGate(ch <-chan struct{})
}

// PostCache provides post metadata access.
type PostCache interface {
	GetPost(id string) (*PostMeta, error)
	ListAllPosts() ([]string, error)
	GetPostByPath(path string) (*PostMeta, error)
	GetPostsByIDs(ids []string) (map[string]*PostMeta, error)
	GetPostsByTemplate(templatePath string) ([]string, error)
	GetAllPostsMetadata() ([]PostListMeta, error)
}

// SearchCache provides access to search indices.
type SearchCache interface {
	GetSearchRecords(ids []string) (map[string]*SearchRecord, error)
	GetSearchRecord(id string) (*SearchRecord, error)
}

// SocialCardCache tracks social card generation state.
type SocialCardCache interface {
	GetSocialCardHash(path string) (string, error)
	SetSocialCardHash(path, hash string) error
	BatchSetSocialCardHashes(hashes map[string]string) error
}

// BuildArtifactCache provides operations for storing build results (HTML, etc).
type BuildArtifactCache interface {
	GetHTMLContent(post *PostMeta) ([]byte, error)
	StoreHTML(content []byte) (string, error)
	StoreHTMLForPost(post *PostMeta, content []byte) error
	BatchCommit(posts []*PostMeta, records map[string]*SearchRecord, deps map[string]*Dependencies) error
	DeletePost(postID string) error
	MarkDirty(postID string)
	IsDirty(postID string) bool
	ClearDirty()
}
