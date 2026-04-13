package mocks

import (
	"context"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/spf13/afero"
)

// MockScanner is a test double for the metadata scanner.
type MockScanner struct {
	Result *models.MetadataScannerResult
	Err    error
}

// Scan runs a scan and returns a single result.
func (m *MockScanner) Scan(opts scanner.ScanOptions) (*models.MetadataScannerResult, error) {
	resChan, errChan := m.ScanStreaming(opts)
	return <-resChan, <-errChan
}

// ScanStreaming streams scan results and errors.
func (m *MockScanner) ScanStreaming(opts scanner.ScanOptions) (<-chan *models.MetadataScannerResult, <-chan error) {
	fileChan := opts.FileChan
	resChan := make(chan *models.MetadataScannerResult, 1)
	errChan := make(chan error, 1)
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	async.FireAndForget(ctx, slog.Default(), "mock scanner stream", func() error {
		if m.Result != nil && fileChan != nil {
			for _, f := range m.Result.Files {
				fileChan <- f
			}
		}
		resChan <- m.Result
		errChan <- m.Err
		return nil
	})
	return resChan, errChan
}

// ScanFile scans a single file and returns a scanned resource record.
func (m *MockScanner) ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedResource, error) {
	return models.ScannedResource{}, nil
}
