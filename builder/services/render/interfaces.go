package render

import (
	"log/slog"

	"github.com/spf13/afero"

	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
)

// Dependencies holds all dependencies for RenderService.
type Dependencies struct {
	Ctx      *buildCtx.BuildContext
	Renderer *renderer.Renderer
	Logger   *slog.Logger
}

// Service handles template rendering and HTML generation.
type Service interface {
	ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs)
	SetAssetsGate(ch <-chan struct{})
	ReconfigureWithLogger(l *slog.Logger)
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
}
