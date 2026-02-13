package services

import (
	"log/slog"

	"my-ssg/builder/models"
	"my-ssg/builder/renderer"
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

func (s *renderServiceImpl) RegisterFile(path string) {
	s.rnd.RegisterFile(path)
}

func (s *renderServiceImpl) SetAssets(assets map[string]string) {
	s.rnd.SetAssets(assets)
}

func (s *renderServiceImpl) GetAssets() map[string]string {
	return s.rnd.Assets
}

func (s *renderServiceImpl) GetRenderedFiles() map[string]bool {
	return s.rnd.GetRenderedFiles()
}

func (s *renderServiceImpl) ClearRenderedFiles() {
	s.rnd.ClearRenderedFiles()
}
