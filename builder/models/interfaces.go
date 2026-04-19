package models

import (
	"context"
	"html/template"
	"io"
	"os"
	"time"
)

// ArtifactSink is an interface for writing build artifacts.
type ArtifactSink interface {
	WriteFile(path string, data []byte) error
	WriteStream(path string, writer func(io.Writer) error) error
	CopyFile(srcPath, destPath string) error
	MkdirAll(path string) error
	Register(path string)
	GetWrittenFiles() map[string]bool
	GetOutputDir() string
	SetMtime(path string, mtime time.Time) error
	Stat(path string) (os.FileInfo, error)
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

// ContentCache provides post metadata access.
type ContentCache interface {
	GetItemByID(ContentID string) (*ContentMeta, error)
	ListAllItems() ([]string, error)
	GetItemByPath(path string) (*ContentMeta, error)
	GetItemsByIDs(ids []string) (map[string]*ContentMeta, error)
	GetItemsByTemplate(templatePath string) ([]string, error)
	GetAllItemsMetadata() ([]ContentListMeta, error)
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
	GetHTMLContent(item *ContentMeta) ([]byte, error)
	StoreHTML(content []byte) (string, error)
	StoreHTMLForItem(item *ContentMeta, content []byte) error
	BatchCommit(items []*ContentMeta, records map[string]*SearchRecord, deps map[string]*Dependencies) error
	DeleteItem(ContentID string) error
	MarkDirty(ContentID string)
	IsDirty(ContentID string) bool
	ClearDirty()
}

// FragmentCache provides persistent storage for pre-rendered UI components.
type FragmentCache interface {
	GetFragment(key string) ([]byte, error)
	StoreFragment(key string, data []byte) error
	Flush(ctx context.Context) error
}

// HealthRecorder provides operations for recording build health events and metrics.
type HealthRecorder interface {
	AddWarning(message string)
	AddError(message string)
	RecordMathFailure()
}
