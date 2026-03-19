package mocks

import (
	"html/template"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"

	"github.com/spf13/afero"
)

type MockRenderService struct {
	Sink            fspkg.ArtifactSink
	SourceFs        afero.Fs
	RenderedPages   sync.Map
	RenderedIndex   sync.Map
	Rendered404     sync.Map
	RenderedGraph   sync.Map
	RegisteredFiles sync.Map
	Assets          sync.Map
	CallCount       sync.Map
}

func NewMockRenderService() *MockRenderService {
	return &MockRenderService{}
}

func (m *MockRenderService) recordCall(method string) {
	val, _ := m.CallCount.LoadOrStore(method, 0)
	m.CallCount.Store(method, val.(int)+1)
}

func (m *MockRenderService) SetSink(sink fspkg.ArtifactSink) {
	m.recordCall("SetSink")
	m.Sink = sink
}

func (m *MockRenderService) SetSourceFs(fs afero.Fs) {
	m.recordCall("SetSourceFs")
	m.SourceFs = fs
}

func (m *MockRenderService) SetAssetsGate(ch <-chan struct{}) {
	m.recordCall("SetAssetsGate")
}

func (m *MockRenderService) ReloadTemplates() {
	m.recordCall("ReloadTemplates")
}

func (m *MockRenderService) RenderPage(path string, data models.PageData) error {
	m.recordCall("RenderPage")
	m.RenderedPages.Store(path, data)
	return nil
}

func (m *MockRenderService) RenderIndex(path string, data models.PageData) error {
	m.recordCall("RenderIndex")
	m.RenderedIndex.Store(path, data)
	return nil
}

func (m *MockRenderService) Render404(path string, data models.PageData) error {
	m.recordCall("Render404")
	m.Rendered404.Store(path, data)
	return nil
}

func (m *MockRenderService) RenderGraph(path string, data models.PageData) error {
	m.recordCall("RenderGraph")
	m.RenderedGraph.Store(path, data)
	return nil
}

func (m *MockRenderService) RenderSidebar(tree []*models.TreeNode) template.HTML {
	m.recordCall("RenderSidebar")
	return ""
}

func (m *MockRenderService) RegisterFile(path string) {
	m.recordCall("RegisterFile")
	m.RegisteredFiles.Store(path, true)
}

func (m *MockRenderService) SetAssets(assets map[string]string) {
	m.recordCall("SetAssets")
	for k, v := range assets {
		m.Assets.Store(k, v)
	}
}

func (m *MockRenderService) GetAssets() map[string]string {
	m.recordCall("GetAssets")
	res := make(map[string]string)
	m.Assets.Range(func(k, v any) bool {
		res[k.(string)] = v.(string)
		return true
	})
	return res
}

func (m *MockRenderService) GetRenderedFiles() map[string]bool {
	m.recordCall("GetRenderedFiles")
	res := make(map[string]bool)
	m.RegisteredFiles.Range(func(k, v any) bool {
		res[k.(string)] = v.(bool)
		return true
	})
	return res
}

func (m *MockRenderService) ClearRenderedFiles() {
	m.recordCall("ClearRenderedFiles")
	m.RegisteredFiles = sync.Map{}
}

func (m *MockRenderService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.recordCall("ReconfigureForBuild")
	m.Sink = sink
	m.SourceFs = fs
}
