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

func (s *renderService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	s.rnd.SetSink(sink)
	s.rnd.SourceFs = fs
	s.rnd.ReloadTemplates()
}

func (s *renderService) SetAssetsGate(ch <-chan struct{}) {
	s.assetsReady = ch
}

func (s *renderService) RenderPage(path string, data models.PageData) error {
	if err := s.waitForAssets(path); err != nil {
		return err
	}
	s.rnd.PreparePageData(&data)
	return s.rnd.RenderPage(path, data)
}

func (s *renderService) RenderIndex(path string, data models.PageData) error {
	if err := s.waitForAssets(path); err != nil {
		return err
	}
	s.rnd.PreparePageData(&data)
	return s.rnd.RenderIndex(path, data)
}

func (s *renderService) Render404(path string, data models.PageData) error {
	// 404 typically doesn't wait for assets to avoid recursive waits or hangs
	// but we still prepare data for consistency.
	s.rnd.PreparePageData(&data)
	return s.rnd.Render404(path, data)
}

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

func (s *renderService) RegisterFile(path string) {
	s.rnd.RegisterFile(path)
}

func (s *renderService) SetAssets(assets map[string]string) {
	s.rnd.SetAssets(assets)
}

func (s *renderService) GetAssets() map[string]string {
	return s.rnd.GetAssets()
}

func (s *renderService) GetRenderedFiles() map[string]bool {
	return s.rnd.GetRenderedFiles()
}

func (s *renderService) ClearRenderedFiles() {
	s.rnd.ClearRenderedFiles()
}

func (s *renderService) ReloadTemplates() {
	s.rnd.ReloadTemplates()
}

func (s *renderService) Has404Template() bool {
	return s.rnd.Has404Template()
}
