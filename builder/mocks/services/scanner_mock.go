package mocks

import (
	"context"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/spf13/afero"
)

type MockScanner struct {
	Result *models.MetadataScannerResult
	Err    error
}

func (m *MockScanner) Scan(ctx context.Context, contentDir string, srcFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error) {
	resChan, errChan := m.ScanStreaming(ctx, contentDir, srcFs, cfg, fileChan)
	return <-resChan, <-errChan
}

func (m *MockScanner) ScanStreaming(ctx context.Context, contentDir string, srcFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (<-chan *models.MetadataScannerResult, <-chan error) {
	resChan := make(chan *models.MetadataScannerResult, 1)
	errChan := make(chan error, 1)
	go func() {
		if m.Result != nil && fileChan != nil {
			for _, f := range m.Result.Files {
				fileChan <- f
			}
		}
		resChan <- m.Result
		errChan <- m.Err
	}()
	return resChan, errChan
}

func (m *MockScanner) ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error) {
	return models.ScannedFile{}, nil
}
