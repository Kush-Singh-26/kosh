package orchestration

import (
	"context"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// WaitScannerAssetsOptions configures coordination between scanning and assets.
type WaitScannerAssetsOptions struct {
	Ctx                  context.Context
	ScannerReady         <-chan struct{}
	MetadataResultChan   <-chan *models.MetadataScannerResult
	ScannerErrChan       <-chan error
	AssetWaitGroup       *sync.WaitGroup
	AssetErrorChan       <-chan error
	DiscoveryReadySignal <-chan struct{}
}

// waitForScannerAndAssets waits for scanner and asset building to complete.
// The discoveryReady signal unblocks Content-processing while image compression continues.
func (engineInstance *Engine) waitForScannerAndAssets(options WaitScannerAssetsOptions) (*models.MetadataScannerResult, <-chan struct{}, error, error, error) {
	workingContext := options.Ctx
	scannerReady := options.ScannerReady
	metadataResultChan := options.MetadataResultChan
	scannerErrChan := options.ScannerErrChan
	assetErrorChan := options.AssetErrorChan
	discoveryReadySignal := options.DiscoveryReadySignal

	select {
	case <-scannerReady:
	case <-workingContext.Done():
		return nil, nil, nil, nil, workingContext.Err()
	}

	// Receive scanner result and error.
	var metadataResult *models.MetadataScannerResult
	var scannerError error
	select {
	case metadataResult = <-metadataResultChan:
		scannerError = <-scannerErrChan
	case <-workingContext.Done():
		return nil, nil, nil, nil, workingContext.Err()
	}

	// Return discoveryReady separately so Content-processing can unblock on it.
	// The caller will wait for assetWaitGroup separately if needed.
	discoverySignal := discoveryReadySignal

	var assetError error
	select {
	case err := <-assetErrorChan:
		assetError = err
	default:
		select {
		case err := <-assetErrorChan:
			assetError = err
		case <-discoverySignal:
			// Discovery ready, continue
		case <-workingContext.Done():
			return nil, nil, nil, nil, workingContext.Err()
		}
	}

	return metadataResult, discoverySignal, scannerError, assetError, nil
}
