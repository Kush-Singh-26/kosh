package mocks

import (
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type MockRenderService struct {
	RenderedPages   map[string]models.PageData
	RenderedIndex   map[string]models.PageData
	Rendered404     map[string]models.PageData
	RenderedGraph   map[string]models.PageData
	RegisteredFiles map[string]bool
	Assets          map[string]string
	CallCount       map[string]int
}

func NewMockRenderService() *MockRenderService {
	return &MockRenderService{
		RenderedPages:   make(map[string]models.PageData),
		RenderedIndex:   make(map[string]models.PageData),
		Rendered404:     make(map[string]models.PageData),
		RenderedGraph:   make(map[string]models.PageData),
		RegisteredFiles: make(map[string]bool),
		Assets:          make(map[string]string),
		CallCount:       make(map[string]int),
	}
}

func (m *MockRenderService) recordCall(method string) {
	if m.CallCount == nil {
		m.CallCount = make(map[string]int)
	}
	m.CallCount[method]++
}

func (m *MockRenderService) RenderPage(path string, data models.PageData) {
	m.recordCall("RenderPage")
	m.RenderedPages[path] = data
}

func (m *MockRenderService) RenderIndex(path string, data models.PageData) {
	m.recordCall("RenderIndex")
	m.RenderedIndex[path] = data
}

func (m *MockRenderService) Render404(path string, data models.PageData) {
	m.recordCall("Render404")
	m.Rendered404[path] = data
}

func (m *MockRenderService) RenderGraph(path string, data models.PageData) {
	m.recordCall("RenderGraph")
	m.RenderedGraph[path] = data
}

func (m *MockRenderService) RegisterFile(path string) {
	m.recordCall("RegisterFile")
	m.RegisteredFiles[path] = true
}

func (m *MockRenderService) SetAssets(assets map[string]string) {
	m.recordCall("SetAssets")
	m.Assets = assets
}

func (m *MockRenderService) GetAssets() map[string]string {
	m.recordCall("GetAssets")
	return m.Assets
}

func (m *MockRenderService) GetRenderedFiles() map[string]bool {
	m.recordCall("GetRenderedFiles")
	return m.RegisteredFiles
}

func (m *MockRenderService) ClearRenderedFiles() {
	m.recordCall("ClearRenderedFiles")
	m.RegisteredFiles = make(map[string]bool)
}
