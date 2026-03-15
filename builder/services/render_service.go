package services

import (
	"fmt"
	"html/template"
	"log/slog"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

type renderServiceImpl struct {
	rnd         *renderer.Renderer
	logger      *slog.Logger
	assetsReady <-chan struct{}
}

func NewRenderService(rnd *renderer.Renderer, logger *slog.Logger) RenderService {
	return &renderServiceImpl{
		rnd:    rnd,
		logger: logger,
	}
}

func (s *renderServiceImpl) ReconfigureForBuild(sink utils.ArtifactSink, fs afero.Fs) {
	s.rnd.SetSink(sink)
	s.rnd.SourceFs = fs
	s.rnd.ReloadTemplates()
}

func (s *renderServiceImpl) SetAssetsGate(ch <-chan struct{}) {
	s.assetsReady = ch
}

func (s *renderServiceImpl) RenderPage(path string, data models.PageData) error {
	if s.assetsReady != nil {
		select {
		case <-s.assetsReady:
		case <-time.After(30 * time.Second):
			return fmt.Errorf("asset build timed out after 30s for page %s - esbuild may be hung", path)
		}
	}
	return s.rnd.RenderPage(path, data)
}

func (s *renderServiceImpl) RenderIndex(path string, data models.PageData) error {
	if s.assetsReady != nil {
		select {
		case <-s.assetsReady:
		case <-time.After(30 * time.Second):
			return fmt.Errorf("asset build timed out after 30s for index %s - esbuild may be hung", path)
		}
	}
	return s.rnd.RenderIndex(path, data)
}

func (s *renderServiceImpl) Render404(path string, data models.PageData) error {
	return s.rnd.Render404(path, data)
}

func (s *renderServiceImpl) RenderGraph(path string, data models.PageData) error {
	if s.assetsReady != nil {
		select {
		case <-s.assetsReady:
		case <-time.After(30 * time.Second):
			return fmt.Errorf("asset build timed out after 30s for graph %s - esbuild may be hung", path)
		}
	}
	return s.rnd.RenderGraph(path, data)
}

func (s *renderServiceImpl) RenderSidebar(tree []*models.TreeNode) template.HTML {
	return s.rnd.RenderSidebar(tree)
}

func (s *renderServiceImpl) RegisterFile(path string) {
	s.rnd.RegisterFile(path)
}

func (s *renderServiceImpl) SetAssets(assets map[string]string) {
	s.rnd.SetAssets(assets)
}

func (s *renderServiceImpl) GetAssets() map[string]string {
	return s.rnd.GetAssets()
}

func (s *renderServiceImpl) GetRenderedFiles() map[string]bool {
	return s.rnd.GetRenderedFiles()
}

func (s *renderServiceImpl) ClearRenderedFiles() {
	s.rnd.ClearRenderedFiles()
}

func (s *renderServiceImpl) ReloadTemplates() {
	s.rnd.ReloadTemplates()
}
