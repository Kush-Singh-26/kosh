package render

// Error Handling Strategy:
// - Recoverable errors: Return error to caller (triggers fallback to full build)
// - Non-recoverable errors: Return error to abort build
// - Fire-and-forget errors: Log only (cache writes, social card generation)

import (
	"fmt"
	"log/slog"
	"time"

	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"

	"github.com/spf13/afero"
)

type renderService struct {
	ctx         *buildCtx.BuildContext
	rnd         *renderer.Renderer
	logger      *slog.Logger
	assetsReady <-chan struct{}
}

// NewService creates a new RenderService with the given dependencies.
// Using dependency struct pattern for API coherence.
func NewService(deps Dependencies) Service {
	return &renderService{
		ctx:    deps.Ctx,
		rnd:    deps.Renderer,
		logger: deps.Logger,
	}
}

// ReconfigureForBuild swaps sink and source filesystem for a new build.
func (s *renderService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.rnd.SetSink(sink)
	s.rnd.SourceFs = fs
	s.rnd.ReloadTemplates()
}

// SetAssetsGate sets the signal that assets are ready for rendering.
func (s *renderService) SetAssetsGate(ch <-chan struct{}) {
	s.assetsReady = ch
}

// ReconfigureWithLogger updates the logger used by the render service.
func (s *renderService) ReconfigureWithLogger(l *slog.Logger) {
	s.logger = l
	s.rnd.SetLogger(l)
}

// RenderPage renders a standard page after assets are ready.
func (s *renderService) RenderPage(path string, data models.PageData) error {
	if err := s.waitForAssets(path); err != nil {
		return err
	}
	s.rnd.PreparePageData(&data)
	return s.rnd.RenderPage(path, data)
}

// RenderIndex renders the index page after assets are ready.
func (s *renderService) RenderIndex(path string, data models.PageData) error {
	if err := s.waitForAssets(path); err != nil {
		return err
	}
	s.rnd.PreparePageData(&data)
	return s.rnd.RenderIndex(path, data)
}

// Render404 renders the 404 page without waiting for assets.
func (s *renderService) Render404(path string, data models.PageData) error {
	// 404 typically doesn't wait for assets to avoid recursive waits or hangs
	// but we still prepare data for consistency.
	s.rnd.PreparePageData(&data)
	return s.rnd.Render404(path, data)
}

// RenderGraph renders the graph page after assets are ready.
func (s *renderService) RenderGraph(path string, data models.PageData) error {
	if err := s.waitForAssets(path); err != nil {
		return err
	}
	s.rnd.PreparePageData(&data)
	return s.rnd.RenderGraph(path, data)
}

func (s *renderService) waitForAssets(path string) error {
	if s.assetsReady != nil {
		select {
		case <-s.assetsReady:
		case <-time.After(30 * time.Second):
			return fmt.Errorf("asset build timed out after 30s for %s - esbuild may be hung", path)
		}
	}
	return nil
}

// RegisterFile records a rendered file path.
func (s *renderService) RegisterFile(path string) {
	s.rnd.RegisterFile(path)
}

// SetAssets snapshots the asset map for rendering.
func (s *renderService) SetAssets(assets map[string]string) {
	s.rnd.SetAssets(assets)
}

// GetAssets returns a snapshot of the asset map.
func (s *renderService) GetAssets() map[string]string {
	return s.rnd.GetAssets()
}

// GetRenderedFiles returns a snapshot of rendered files.
func (s *renderService) GetRenderedFiles() map[string]bool {
	return s.rnd.GetRenderedFiles()
}

// ClearRenderedFiles clears the recorded rendered files.
func (s *renderService) ClearRenderedFiles() {
	s.rnd.ClearRenderedFiles()
}

// ReloadTemplates reloads renderer templates from disk or cache.
func (s *renderService) ReloadTemplates() {
	s.rnd.ReloadTemplates()
}

// Has404Template reports whether a 404 template was loaded.
func (s *renderService) Has404Template() bool {
	return s.rnd.Has404Template()
}
