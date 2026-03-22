package scanner

import (
	"context"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// Scanner scans content directory for markdown files and extracts metadata.
type Scanner interface {
	Scan(ctx context.Context, contentDir string, srcFs afero.Fs, cfg *config.Config, fileChan chan<- models.ScannedFile) (*models.MetadataScannerResult, error)
	ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error)
}
