package render

import (
	"context"
	"html/template"
	"log/slog"

	"github.com/spf13/afero"

	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
)

// Dependencies holds all dependencies for RenderService.
type Dependencies struct {
	Ctx      *buildctx.BuildContext
	Renderer *renderer.Renderer
	Logger   *slog.Logger
}

// PageRenderer handles specific content rendering operations.
type PageRenderer interface {
	RenderPage(path string, data models.PageData) error
	RenderIndex(path string, data models.PageData) error
	Render404(path string, data models.PageData) error
	RenderGraph(path string, data models.PageData) error
	RenderFragment(context string, blockName string, data models.PageData) (template.HTML, error)
}

// AssetRegistry manages the registration and retrieval of build assets.
type AssetRegistry interface {
	RegisterFile(path string)
	SetAssets(assets map[string]string)
	GetAssets() map[string]string
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
}

// LifecycleManager handles the configuration and maintenance of the rendering service.
type LifecycleManager interface {
	ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs)
	ReconfigureWithLogger(logger *slog.Logger)
	SetAssetsGate(signal <-chan struct{})
	ReloadTemplates()
	Has404Template() bool
	FlushFragments(ctx context.Context) error
}

// Service is a composite interface that bundles all rendering capabilities.
type Service interface {
	PageRenderer
	AssetRegistry
	LifecycleManager
}
