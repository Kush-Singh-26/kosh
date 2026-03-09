package mocks

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

type MockAssetService struct {
	Sink    utils.ArtifactSink
	Metrics *metrics.BuildMetrics
}

func (m *MockAssetService) SetSink(sink utils.ArtifactSink) {
	m.Sink = sink
}

func (m *MockAssetService) SetSourceFs(fs afero.Fs) {}

func (m *MockAssetService) SetMetrics(m2 *metrics.BuildMetrics) {
	m.Metrics = m2
}

func (m *MockAssetService) SetAssetsReadySignal(ch chan struct{}) {}

func (m *MockAssetService) Build(ctx context.Context) error {
	return nil
}

func (m *MockAssetService) BuildForAssetChange(ctx context.Context) (map[string]string, error) {
	return map[string]string{}, nil
}
