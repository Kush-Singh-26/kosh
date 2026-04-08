package orchestration

import (
	"context"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

// buildScanResult holds channels for the parallel metadata scan.
type buildScanResult struct {
	fileChan           <-chan models.ScannedFile
	scannerReady       <-chan struct{}
	metadataResultChan <-chan *models.MetadataScannerResult
	scannerErrChan     <-chan error
}

// scanPhase launches the parallel metadata scanner.
func (b *Engine) scanPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildScanResult {
	if b.Deps.Reporter != nil {
		b.Deps.Reporter.StartPhase(ui.PhaseScan)
	}
	fileChan := make(chan models.ScannedFile, 1024)
	scannerReady := make(chan struct{})
	metadataResultChan := make(chan *models.MetadataScannerResult, 1)
	scannerErrChan := make(chan error, 1)

	logger := b.Deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	async.FireAndForget(ctx, logger, "metadata scan", func() error {
		defer close(scannerReady)
		defer close(fileChan)
		defer close(metadataResultChan)
		defer close(scannerErrChan)

		metadataResult, scannerErr := b.Deps.Scanner.Scan(scanner.ScanOptions{
			Ctx:        ctx,
			ContentDir: b.Cfg.ContentDir,
			SrcFs:      b.Deps.SourceFs,
			Cfg:        b.Cfg,
			FileChan:   fileChan,
		})
		if scannerErr == nil {
			contentAssetsChan <- metadataResult.ContentAssets
		}
		// Always send result and error (even if nil).
		metadataResultChan <- metadataResult
		scannerErrChan <- scannerErr
		return nil
	})

	return &buildScanResult{
		fileChan:           fileChan,
		scannerReady:       scannerReady,
		metadataResultChan: metadataResultChan,
		scannerErrChan:     scannerErrChan,
	}
}
