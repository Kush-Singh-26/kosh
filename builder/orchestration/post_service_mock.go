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
	ProcessResult    *post.ContentResult
	ProcessErr       error
	ProcessSingleErr error
}

// ReconfigureForBuild updates the mock with build-time artifacts.
func (serviceMock *mockPostService) ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs) {
	serviceMock.Sink = sink
	serviceMock.SourceFs = sourceFs
}

// SetAssetsGate sets the assets gate for the mock.
func (serviceMock *mockPostService) SetAssetsGate(readySignal <-chan struct{}) {}

// ReconfigureWithReporter is a no-op for the mock.
func (serviceMock *mockPostService) ReconfigureWithReporter(reporter ui.Reporter, logger *slog.Logger) {}

// Process returns the configured result for the mock.
func (serviceMock *mockPostService) Process(opts post.ProcessOptions) (*post.ContentResult, error) {
	if serviceMock.ProcessResult != nil {
		return serviceMock.ProcessResult, serviceMock.ProcessErr
	}
	return &post.ContentResult{}, serviceMock.ProcessErr
}

// ProcessStreaming returns the configured result after draining the file channel.
func (serviceMock *mockPostService) ProcessStreaming(opts post.ProcessOptions) (*post.ContentResult, error) {
	// Drain the channel to simulate consumption
	for range opts.FileChan {
	}
	if serviceMock.ProcessResult != nil {
		return serviceMock.ProcessResult, serviceMock.ProcessErr
	}
	return &post.ContentResult{}, serviceMock.ProcessErr
}

// ProcessSingle returns the configured error for the mock.
func (serviceMock *mockPostService) ProcessSingle(ctx context.Context, path string, source []byte) error {
	return serviceMock.ProcessSingleErr
}

// ProcessSingleWithResult returns the configured error for the mock.
func (serviceMock *mockPostService) ProcessSingleWithResult(ctx context.Context, path string, source []byte, result *post.ParsedMarkdownResult) error {
	return serviceMock.ProcessSingleErr
}

// WaitForCacheCommit is a no-op for the mock.
func (serviceMock *mockPostService) WaitForCacheCommit() {}

// GetMetadataContext returns the configured metadata context for the mock.
func (serviceMock *mockPostService) GetMetadataContext(ctx context.Context) (*post.ContentContext, error) {
	if serviceMock.ProcessResult != nil {
		return serviceMock.ProcessResult.ToContentContext(), nil
	}
	return &post.ContentContext{}, nil
}
