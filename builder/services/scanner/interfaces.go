package scanner

import (
	"context"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

type ScanOptions struct {
	Ctx        context.Context
	ContentDir string
	SrcFs      afero.Fs
	Cfg        *config.Config
	FileChan   chan<- models.ScannedFile
}

// Scanner scans content directory for markdown files and extracts metadata.
type Scanner interface {
	Scan(opts ScanOptions) (*models.MetadataScannerResult, error)
	ScanStreaming(opts ScanOptions) (<-chan *models.MetadataScannerResult, <-chan error)
	ScanFile(srcFs afero.Fs, cfg *config.Config, path string) (models.ScannedFile, error)
}
