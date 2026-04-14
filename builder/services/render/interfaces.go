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

// Service handles template rendering and HTML generation.
type Service interface {
	ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs)
	SetAssetsGate(signal <-chan struct{})
	ReconfigureWithLogger(logger *slog.Logger)
	RenderFragment(context string, blockName string, data models.PageData) (template.HTML, error)
	RenderPage(path string, data models.PageData) error
	RenderIndex(path string, data models.PageData) error
	Render404(path string, data models.PageData) error
	RenderGraph(path string, data models.PageData) error
	RegisterFile(path string)
	SetAssets(assets map[string]string)
	GetAssets() map[string]string
	GetRenderedFiles() map[string]bool
	ClearRenderedFiles()
	ReloadTemplates()
	Has404Template() bool
	FlushFragments(ctx context.Context) error
}
