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

func (m *MockScanner) Scan(ctx context.Context, contentDir string, sourceFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error) {
	if m.Result != nil && fileChan != nil {
		for _, f := range m.Result.Files {
			fileChan <- f
		}
	}
	return m.Result, m.Err
}

func (m *MockScanner) ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error) {
	return models.ScannedFile{}, nil
}
