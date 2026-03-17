package run

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	fspkg "github.com/Kush-Singh-26/kosh/builder/utils/fs"
	"github.com/spf13/afero"
)

type mockPostService struct {
	Sink             fspkg.ArtifactSink
	SourceFs         afero.Fs
	ProcessResult    *services.PostResult
	ProcessErr       error
	ProcessSingleErr error
}

func (m *mockPostService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.Sink = sink
	m.SourceFs = fs
}

func (m *mockPostService) SetAssetsGate(ch <-chan struct{}) {}

func (m *mockPostService) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, fileChan <-chan models.ScannedFile) (*services.PostResult, error) {
	if m.ProcessResult != nil {
		return m.ProcessResult, m.ProcessErr
	}
	return &services.PostResult{}, m.ProcessErr
}

func (m *mockPostService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return m.ProcessSingleErr
}

func (m *mockPostService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, res *services.ParsedMarkdownResult) error {
	return m.ProcessSingleErr
}

func (m *mockPostService) WaitForCacheCommit() {}
