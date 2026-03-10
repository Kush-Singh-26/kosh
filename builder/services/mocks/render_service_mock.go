package mocks

import (
	"html/template"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

type MockRenderService struct {
	mu              sync.Mutex
	Sink            utils.ArtifactSink
	SourceFs        afero.Fs
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

func (m *MockRenderService) SetSink(sink utils.ArtifactSink) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("SetSink")
	m.Sink = sink
}

func (m *MockRenderService) SetSourceFs(fs afero.Fs) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("SetSourceFs")
	m.SourceFs = fs
}

func (m *MockRenderService) SetAssetsGate(ch <-chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("SetAssetsGate")
}

func (m *MockRenderService) ReloadTemplates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("ReloadTemplates")
}

func (m *MockRenderService) RenderPage(path string, data models.PageData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("RenderPage")
	m.RenderedPages[path] = data
}

func (m *MockRenderService) RenderIndex(path string, data models.PageData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("RenderIndex")
	m.RenderedIndex[path] = data
}

func (m *MockRenderService) Render404(path string, data models.PageData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("Render404")
	m.Rendered404[path] = data
}

func (m *MockRenderService) RenderGraph(path string, data models.PageData) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("RenderGraph")
}

func (m *MockRenderService) RenderSidebar(tree []*models.TreeNode) template.HTML {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("RenderSidebar")
	return ""
}

func (m *MockRenderService) RegisterFile(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("RegisterFile")
	m.RegisteredFiles[path] = true
}

func (m *MockRenderService) SetAssets(assets map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("SetAssets")
	m.Assets = assets
}

func (m *MockRenderService) GetAssets() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("GetAssets")
	return m.Assets
}

func (m *MockRenderService) GetRenderedFiles() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("GetRenderedFiles")
	return m.RegisteredFiles
}

func (m *MockRenderService) ClearRenderedFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recordCall("ClearRenderedFiles")
	m.RegisteredFiles = make(map[string]bool)
}
