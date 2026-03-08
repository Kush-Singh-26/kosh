package mocks

import (
	"context"

	"github.com/spf13/afero"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/utils"
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

func (m *MockAssetService) Build(ctx context.Context) error {
	return nil
}
