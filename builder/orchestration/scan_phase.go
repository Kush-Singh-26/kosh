package orchestration

import (
	"context"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

const (
	scanFileChanBuffer = 1024
	scanResultBuffer   = 1
)

// buildScanResult holds channels for the parallel metadata scan.
type buildScanResult struct {
	fileChan           <-chan models.ScannedFile
	scannerReady       <-chan struct{}
	metadataResultChan <-chan *models.MetadataScannerResult
	scannerErrChan     <-chan error
}

// scanPhase launches the parallel metadata scanner.
func (engineInstance *Engine) scanPhase(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) *buildScanResult {
	if engineInstance.Deps.Reporter != nil {
		engineInstance.Deps.Reporter.StartPhase(ui.PhaseScan)
	}
	fileChannel := make(chan models.ScannedFile, scanFileChanBuffer)
	scannerReady := make(chan struct{})
	metadataResultChan := make(chan *models.MetadataScannerResult, scanResultBuffer)
	scannerErrChan := make(chan error, scanResultBuffer)

	logger := engineInstance.Deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
		async.FireAndForget(ctx, logger, "metadata scan", func() error {
		defer close(scannerReady)
		defer close(fileChannel)
		defer close(metadataResultChan)
		defer close(scannerErrChan)

		metadataResult, scannerError := engineInstance.Deps.Scanner.Scan(scanner.ScanOptions{
			Ctx:        ctx,
			ContentDir: engineInstance.Cfg.ContentDir,
			SrcFs:      engineInstance.Deps.SourceFs,
			Cfg:        engineInstance.Cfg,
			FileChan:   fileChannel,
		})
		if scannerError == nil {
			contentAssetsChan <- metadataResult.ContentAssets
		}
		// Always send result and error (even if nil).
		metadataResultChan <- metadataResult
		scannerErrChan <- scannerError
		return nil
	})

	return &buildScanResult{
		fileChan:           fileChannel,
		scannerReady:       scannerReady,
		metadataResultChan: metadataResultChan,
		scannerErrChan:     scannerErrChan,
	}
}
