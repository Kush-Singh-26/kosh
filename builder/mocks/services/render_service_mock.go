package mocks

import (
	"context"
	"html/template"
	"log/slog"
	"sync"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"

	"github.com/spf13/afero"
)

// MockRenderService is a test double for the render service.
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

// NewMockRenderService returns a new mock render service.
func NewMockRenderService() *MockRenderService {
	return &MockRenderService{}
}

func (m *MockRenderService) recordCall(method string) {
	val, _ := m.CallCount.LoadOrStore(method, 0)
	m.CallCount.Store(method, val.(int)+1)
}

// SetSink sets the sink used by the mock render service.
func (m *MockRenderService) SetSink(sink fspkg.ArtifactSink) {
	m.recordCall("SetSink")
	m.Sink = sink
}

// SetSourceFs sets the source filesystem for the mock render service.
func (m *MockRenderService) SetSourceFs(fs afero.Fs) {
	m.recordCall("SetSourceFs")
	m.SourceFs = fs
}

// SetAssetsGate records the assets gate channel.
func (m *MockRenderService) SetAssetsGate(ch <-chan struct{}) {
	m.recordCall("SetAssetsGate")
}

// ReloadTemplates records a template reload call.
func (m *MockRenderService) ReloadTemplates() {
	m.recordCall("ReloadTemplates")
}

// ReconfigureWithLogger sets the logger on the mock render service.
func (m *MockRenderService) ReconfigureWithLogger(l *slog.Logger) {
	m.recordCall("ReconfigureWithLogger")
}

// RenderFragment records a fragment rendering call.
func (m *MockRenderService) RenderFragment(context string, blockName string, data models.PageData) (template.HTML, error) {
	m.recordCall("RenderFragment")
	return template.HTML("mock fragment"), nil
}

// RenderPage records a rendered page.
func (m *MockRenderService) RenderPage(path string, data models.PageData) error {
	m.recordCall("RenderPage")
	m.RenderedPages.Store(path, data)
	return nil
}

// RenderIndex records a rendered index page.
func (m *MockRenderService) RenderIndex(path string, data models.PageData) error {
	m.recordCall("RenderIndex")
	m.RenderedIndex.Store(path, data)
	return nil
}

// Render404 records a rendered 404 page.
func (m *MockRenderService) Render404(path string, data models.PageData) error {
	m.recordCall("Render404")
	m.Rendered404.Store(path, data)
	return nil
}

// RenderGraph records a rendered graph page.
func (m *MockRenderService) RenderGraph(path string, data models.PageData) error {
	m.recordCall("RenderGraph")
	m.RenderedGraph.Store(path, data)
	return nil
}

// RegisterFile records a file registration.
func (m *MockRenderService) RegisterFile(path string) {
	m.recordCall("RegisterFile")
	m.RegisteredFiles.Store(path, true)
}

// SetAssets stores the provided asset map.
func (m *MockRenderService) SetAssets(assets map[string]string) {
	m.recordCall("SetAssets")
	for k, v := range assets {
		m.Assets.Store(k, v)
	}
}

// GetAssets returns the recorded asset map.
func (m *MockRenderService) GetAssets() map[string]string {
	m.recordCall("GetAssets")
	res := make(map[string]string)
	m.Assets.Range(func(k, v any) bool {
		res[k.(string)] = v.(string)
		return true
	})
	return res
}

// GetRenderedFiles returns the set of registered files.
func (m *MockRenderService) GetRenderedFiles() map[string]bool {
	m.recordCall("GetRenderedFiles")
	res := make(map[string]bool)
	m.RegisteredFiles.Range(func(k, v any) bool {
		res[k.(string)] = v.(bool)
		return true
	})
	return res
}

// ClearRenderedFiles clears the registered file set.
func (m *MockRenderService) ClearRenderedFiles() {
	m.recordCall("ClearRenderedFiles")
	m.RegisteredFiles = sync.Map{}
}

// ReconfigureForBuild sets the sink and source filesystem for the mock.
func (m *MockRenderService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.recordCall("ReconfigureForBuild")
	m.Sink = sink
	m.SourceFs = fs
}

// Has404Template reports whether a 404 template is available.
func (m *MockRenderService) Has404Template() bool {
	m.recordCall("Has404Template")
	return true
}

// FlushFragments flushes the fragment cache.
func (m *MockRenderService) FlushFragments(_ context.Context) error {
	m.recordCall("FlushFragments")
	return nil
}
