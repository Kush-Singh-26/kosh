package orchestration

import (
	"context"
	"log/slog"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/spf13/afero"
)

type mockPostService struct {
	Sink             fspkg.ArtifactSink
	SourceFs         afero.Fs
	ProcessResult    *post.PostResult
	ProcessErr       error
	ProcessSingleErr error
}

func (m *mockPostService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.Sink = sink
	m.SourceFs = fs
}

func (m *mockPostService) SetAssetsGate(ch <-chan struct{}) {}

func (m *mockPostService) ReconfigureWithReporter(r ui.Reporter, l *slog.Logger) {}

func (m *mockPostService) Process(ctx context.Context, shouldForce, forceSocialRebuild, outputMissing bool, files []models.ScannedFile) (*post.PostResult, error) {
	if m.ProcessResult != nil {
		return m.ProcessResult, m.ProcessErr
	}
	return &post.PostResult{}, m.ProcessErr
}

func (m *mockPostService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return m.ProcessSingleErr
}

func (m *mockPostService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, res *post.ParsedMarkdownResult) error {
	return m.ProcessSingleErr
}

func (m *mockPostService) WaitForCacheCommit() {}
