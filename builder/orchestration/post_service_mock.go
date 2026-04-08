package orchestration

import (
	"context"
	"log/slog"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
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

// ReconfigureForBuild updates the mock with build-time artifacts.
func (m *mockPostService) ReconfigureForBuild(sink fspkg.ArtifactSink, fs afero.Fs) {
	m.Sink = sink
	m.SourceFs = fs
}

// SetAssetsGate sets the assets gate for the mock.
func (m *mockPostService) SetAssetsGate(ch <-chan struct{}) {}

// ReconfigureWithReporter is a no-op for the mock.
func (m *mockPostService) ReconfigureWithReporter(r ui.Reporter, l *slog.Logger) {}

// Process returns the configured result for the mock.
func (m *mockPostService) Process(opts post.ProcessOptions) (*post.PostResult, error) {
	if m.ProcessResult != nil {
		return m.ProcessResult, m.ProcessErr
	}
	return &post.PostResult{}, m.ProcessErr
}

// ProcessStreaming returns the configured result after draining the file channel.
func (m *mockPostService) ProcessStreaming(opts post.ProcessOptions) (*post.PostResult, error) {
	// Drain the channel to simulate consumption
	for range opts.FileChan {
	}
	if m.ProcessResult != nil {
		return m.ProcessResult, m.ProcessErr
	}
	return &post.PostResult{}, m.ProcessErr
}

// ProcessSingle returns the configured error for the mock.
func (m *mockPostService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return m.ProcessSingleErr
}

// ProcessSingleWithResult returns the configured error for the mock.
func (m *mockPostService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, res *post.ParsedMarkdownResult) error {
	return m.ProcessSingleErr
}

// WaitForCacheCommit is a no-op for the mock.
func (m *mockPostService) WaitForCacheCommit() {}
