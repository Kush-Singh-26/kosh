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

func (m *MockAssetService) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	m.contentAssetsChan = ch
}

func (m *MockAssetService) Build(ctx context.Context) error {
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
	return nil
}

func (m *MockAssetService) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

func (m *MockAssetService) DiscoveryReady() <-chan struct{} {
	return m.assetsReady // For mock, discoveryReady == assetsReady (instant)
}

func (m *MockAssetService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.Sink = sink
}
