package mocks

import (
	"context"
	"sync"

	"log/slog"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/ui"

	"github.com/spf13/afero"
)

// MockAssetService is a test double for the asset service.
type MockAssetService struct {
	mu                sync.Mutex // protects fields below during concurrent tests
	Sink              fspkg.ArtifactSink
	Metrics           *metrics.BuildMetrics
	assetsReady       chan struct{}
	discoveryReady    chan struct{}
	contentAssetsChan <-chan []models.ScannedAsset
	FailBuild         bool // When true, Build() returns an error
}

// SetSink sets the sink used by the mock asset service.
func (m *MockAssetService) SetSink(sink fspkg.ArtifactSink) {
	m.Sink = sink
}

// SetSourceFs sets the source filesystem for the mock asset service.
func (m *MockAssetService) SetSourceFs(_ afero.Fs) {}

// SetMetrics sets the metrics collector for the mock asset service.
func (m *MockAssetService) SetMetrics(m2 *metrics.BuildMetrics) {
	m.Metrics = m2
}

// SetAssetsReadySignal sets the readiness channel for asset build completion.
func (m *MockAssetService) SetAssetsReadySignal(ch chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assetsReady = ch
}

// SetDiscoveryReady sets the discovery-ready channel for the mock.
func (m *MockAssetService) SetDiscoveryReady(ch chan struct{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discoveryReady = ch
}

// SetContentAssetsChannel sets the channel used for content asset discovery.
func (m *MockAssetService) SetContentAssetsChannel(ch <-chan []models.ScannedAsset) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.contentAssetsChan = ch
}

// Build runs the mock build.
func (m *MockAssetService) Build(ctx context.Context) error {
	return m.BuildWithOptions(ctx, false)
}

// BuildWithOptions runs the mock build with options.
func (m *MockAssetService) BuildWithOptions(ctx context.Context, _ bool) error {
	m.mu.Lock()
	failBuild := m.FailBuild
	contentAssetsChan := m.contentAssetsChan
	assetsReady := m.assetsReady
	discoveryReady := m.discoveryReady
	if assetsReady != nil {
		m.assetsReady = nil
	}
	if discoveryReady != nil {
		m.discoveryReady = nil
	}
	m.mu.Unlock()

	defer func() {
		if assetsReady != nil {
			close(assetsReady)
		}
		if discoveryReady != nil {
			close(discoveryReady)
		}
	}()

	if failBuild {
		return context.Canceled
	}
	if contentAssetsChan != nil {
		select {
		case <-contentAssetsChan:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// BuildForAssetChange simulates an incremental asset change build.
func (m *MockAssetService) BuildForAssetChange(_ context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}

// BuildForAssetChangeWithOptions simulates an incremental asset change build with options.
func (m *MockAssetService) BuildForAssetChangeWithOptions(_ context.Context, _ bool) (map[string]string, error) {
	return map[string]string{}, nil
}

// DiscoveryReady returns the discovery-ready channel, falling back to assets-ready.
func (m *MockAssetService) DiscoveryReady() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.discoveryReady != nil {
		return m.discoveryReady
	}
	return m.assetsReady // Fallback: discoveryReady == assetsReady (instant)
}

// ReconfigureForBuild updates the sink for the mock build.
func (m *MockAssetService) ReconfigureForBuild(sink fspkg.ArtifactSink, _ afero.Fs) {
	m.Sink = sink
}

// ReconfigureWithReporter sets the reporter and logger for the mock.
func (m *MockAssetService) ReconfigureWithReporter(_ ui.Reporter, _ *slog.Logger) {}
