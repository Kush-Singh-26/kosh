package mocks

import (
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/spf13/afero"
)

type MockScanner struct {
	Result *models.MetadataScannerResult
	Err    error
}

func (m *MockScanner) Scan(opts scanner.ScanOptions) (*models.MetadataScannerResult, error) {
	resChan, errChan := m.ScanStreaming(opts)
	return <-resChan, <-errChan
}

func (m *MockScanner) ScanStreaming(opts scanner.ScanOptions) (<-chan *models.MetadataScannerResult, <-chan error) {
	fileChan := opts.FileChan
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
