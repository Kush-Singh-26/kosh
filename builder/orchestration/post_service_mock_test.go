package orchestration

import (
	"context"
	"log/slog"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/services/content"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/spf13/afero"
)

type mockPostService struct {
	Sink             fspkg.ArtifactSink
	SourceFs         afero.Fs
	ProcessResult    *content.Result
	ProcessErr       error
	ProcessSingleErr error
}

// ReconfigureForBuild updates the mock with build-time artifacts.
func (serviceMock *mockPostService) ReconfigureForBuild(sink fspkg.ArtifactSink, sourceFs afero.Fs) {
	serviceMock.Sink = sink
	serviceMock.SourceFs = sourceFs
}

// SetAssetsGate sets the assets gate for the mock.
func (serviceMock *mockPostService) SetAssetsGate(_ <-chan struct{}) {}

// ReconfigureWithReporter is a no-op for the mock.
func (serviceMock *mockPostService) ReconfigureWithReporter(_ ui.Reporter, _ *slog.Logger) {}

// Process returns the configured result for the mock.
func (serviceMock *mockPostService) Process(_ content.ProcessOptions) (*content.Result, error) {
	if serviceMock.ProcessResult != nil {
		return serviceMock.ProcessResult, serviceMock.ProcessErr
	}
	return &content.Result{}, serviceMock.ProcessErr
}

// ProcessStreaming returns the configured result after draining the file channel.
func (serviceMock *mockPostService) ProcessStreaming(opts content.ProcessOptions) (*content.Result, error) {
	// Drain the channel to simulate consumption
	for range opts.FileChan { //nolint:revive // intentionally drain channel
	}
	if serviceMock.ProcessResult != nil {
		return serviceMock.ProcessResult, serviceMock.ProcessErr
	}
	return &content.Result{}, serviceMock.ProcessErr
}

// ProcessSingle returns the configured error for the mock.
func (serviceMock *mockPostService) ProcessSingle(_ context.Context, _ string, _ []byte) error {
	return serviceMock.ProcessSingleErr
}

func (serviceMock *mockPostService) ProcessSingleWithResult(_ context.Context, _ string, _ []byte, _ *content.ParsedMarkdownResult) error {
	return serviceMock.ProcessSingleErr
}

// WaitForCacheCommit is a no-op for the mock.
func (serviceMock *mockPostService) WaitForCacheCommit() {}

// GetMetadataContext returns the configured metadata context for the mock.
func (serviceMock *mockPostService) GetMetadataContext(_ context.Context) (*content.Context, error) {
	if serviceMock.ProcessResult != nil {
		return serviceMock.ProcessResult.ToContext(), nil
	}
	return &content.Context{}, nil
}
