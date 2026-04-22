package render

// Error Handling Strategy:
// - Recoverable errors: Return error to caller (triggers fallback to full build)
// - Non-recoverable errors: Return error to abort build
// - Fire-and-forget errors: Log only (cache writes, social card generation)

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"time"

	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"

	"github.com/spf13/afero"
)

const assetWaitTimeout = 30 * time.Second

type renderService struct {
	ctx         *buildctx.BuildContext
	renderer    *renderer.Renderer
	logger      *slog.Logger
	assetsReady <-chan struct{}
}

// NewService creates a new RenderService with the given dependencies.
// Using dependency struct pattern for API coherence.
func NewService(dependencies Dependencies) Service {
	return &renderService{
		ctx:      dependencies.Ctx,
		renderer: dependencies.Renderer,
		logger:   dependencies.Logger,
	}
}

// ReconfigureForBuild swaps sink and source filesystem for a new build.
func (service *renderService) ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs) {
	service.renderer.SetSink(sink)
	service.renderer.SourceFs = sourceFs
	service.renderer.ReloadTemplates()
}

// SetAssetsGate sets the signal that assets are ready for rendering.
func (service *renderService) SetAssetsGate(assetsReadySignal <-chan struct{}) {
	service.assetsReady = assetsReadySignal
}

// ReconfigureWithLogger updates the logger used by the render service.
func (service *renderService) ReconfigureWithLogger(logger *slog.Logger) {
	service.logger = logger
	service.renderer.SetLogger(logger)
}

// RenderFragment renders a named fragment block through the renderer.
func (service *renderService) RenderFragment(context string, blockName string, data models.PageData) (template.HTML, error) {
	return service.renderer.RenderFragment(context, blockName, data)
}

// RenderPage renders a standard page after assets are ready.
func (service *renderService) RenderPage(path string, data models.PageData) error {
	if err := service.waitForAssets(path); err != nil {
		return err
	}
	service.preparePageData(&data)
	return service.renderer.RenderPage(path, data)
}

// RenderIndex renders the index page after assets are ready.
func (service *renderService) RenderIndex(path string, data models.PageData) error {
	if err := service.waitForAssets(path); err != nil {
		return err
	}
	service.preparePageData(&data)
	return service.renderer.RenderIndex(path, data)
}

// Render404 renders the 404 page without waiting for assets.
func (service *renderService) Render404(path string, data models.PageData) error {
	// 404 typically doesn't wait for assets to avoid recursive waits or hangs
	// but we still prepare data for consistency.
	service.preparePageData(&data)
	return service.renderer.Render404(path, data)
}

// RenderGraph renders the graph page after assets are ready.
func (service *renderService) RenderGraph(path string, data models.PageData) error {
	if err := service.waitForAssets(path); err != nil {
		return err
	}
	service.preparePageData(&data)
	return service.renderer.RenderGraph(path, data)
}

func (service *renderService) preparePageData(data *models.PageData) {
	data.IsCleanBuild = service.ctx.IsCleanBuild
	service.renderer.PreparePageData(data)
}

func (service *renderService) waitForAssets(path string) error {
	if service.assetsReady != nil {
		select {
		case <-service.assetsReady:
		case <-time.After(assetWaitTimeout):
			return fmt.Errorf("asset build timed out after %s for %s - esbuild may be hung", assetWaitTimeout, path)
		}
	}
	return nil
}

// RegisterFile records a rendered file path.
func (service *renderService) RegisterFile(path string) {
	service.renderer.RegisterFile(path)
}

// SetAssets snapshots the asset map for rendering.
func (service *renderService) SetAssets(assets map[string]string) {
	service.renderer.SetAssets(assets)
}

// GetAssets returns a snapshot of the asset map.
func (service *renderService) GetAssets() map[string]string {
	return service.renderer.GetAssets()
}

// GetRenderedFiles returns a snapshot of rendered files.
func (service *renderService) GetRenderedFiles() map[string]bool {
	return service.renderer.GetRenderedFiles()
}

// ClearRenderedFiles clears the recorded rendered files.
func (service *renderService) ClearRenderedFiles() {
	service.renderer.ClearRenderedFiles()
}

// ReloadTemplates reloads renderer templates from disk or cache.
func (service *renderService) ReloadTemplates() {
	service.renderer.ReloadTemplates()
}

// Has404Template reports whether a 404 template was loaded.
func (service *renderService) Has404Template() bool {
	return service.renderer.Has404Template()
}

// FlushFragments flushes the fragment cache to BoltDB.
func (service *renderService) FlushFragments(ctx context.Context) error {
	if service.renderer.Cache == nil {
		return nil
	}
	return service.renderer.Cache.Flush(ctx)
}
