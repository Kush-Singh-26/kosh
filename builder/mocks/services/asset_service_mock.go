package mocks

import (
	"context"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"

	"github.com/spf13/afero"
)

type MockAssetService struct {
	Sink              fspkg.ArtifactSink
	Metrics           *metrics.BuildMetrics
	assetsReady       chan struct{}
	discoveryReady    chan struct{}
	contentAssetsChan <-chan []models.ScannedAsset
	FailBuild         bool // When true, Build() returns an error
}

func (m *MockAssetService) SetSink(sink fspkg.ArtifactSink) {
	m.Sink = sink
}

func (m *MockAssetService) SetSourceFs(fs afero.Fs) {}

func (m *MockAssetService) SetMetrics(m2 *metrics.BuildMetrics) {
	m.Metrics = m2
}

func (m *MockAssetService) SetAssetsReadySignal(ch chan struct{}) {
	m.assetsReady = ch
}

func (m *MockAssetService) SetDiscoveryReady(ch chan struct{}) {
	m.discoveryReady = ch
}

func (m *MockAssetService) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	m.contentAssetsChan = ch
}

func (m *MockAssetService) Build(ctx context.Context) error {
	return m.BuildWithOptions(ctx, false)
}

func (m *MockAssetService) BuildWithOptions(ctx context.Context, skipImages bool) error {
	if m.FailBuild {
		return context.Canceled // Simulate build failure
	}
	if m.contentAssetsChan != nil {
		<-m.contentAssetsChan
	}
	if m.assetsReady != nil {
		close(m.assetsReady)
		m.assetsReady = nil
	}
	if m.discoveryReady != nil {
		close(m.discoveryReady)
		m.discoveryReady = nil
	}
	return nil
}

func (m *MockAssetService) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *MockAssetService) BuildForAssetChangeWithOptions(ctx context.Context, forceImages bool) (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *MockAssetService) DiscoveryReady() <-chan struct{} {
	if m.discoveryReady != nil {
		return m.discoveryReady
	}
	return m.assetsReady // Fallback: discoveryReady == assetsReady (instant)
}

func (m *MockAssetService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.Sink = sink
}
