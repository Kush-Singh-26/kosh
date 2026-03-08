package services

import (
	"html/template"
	"log/slog"

	"github.com/spf13/afero"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

type renderServiceImpl struct {
	rnd    *renderer.Renderer
	logger *slog.Logger
}

func NewRenderService(rnd *renderer.Renderer, logger *slog.Logger) RenderService {
	return &renderServiceImpl{
		rnd:    rnd,
		logger: logger,
	}
}

func (s *renderServiceImpl) SetSink(sink utils.ArtifactSink) {
	s.rnd.SetSink(sink)
}

func (s *renderServiceImpl) SetSourceFs(fs afero.Fs) {
	s.rnd.SourceFs = fs
	s.rnd.ReloadTemplates()
}

func (s *renderServiceImpl) RenderPage(path string, data models.PageData) {
	s.rnd.RenderPage(path, data)
}

func (s *renderServiceImpl) RenderIndex(path string, data models.PageData) {
	s.rnd.RenderIndex(path, data)
}

func (s *renderServiceImpl) Render404(path string, data models.PageData) {
	s.rnd.Render404(path, data)
}

func (s *renderServiceImpl) RenderGraph(path string, data models.PageData) {
	s.rnd.RenderGraph(path, data)
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
