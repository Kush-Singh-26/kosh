package orchestration

import (
	"context"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

// WaitScannerAssetsOptions configures coordination between scanning and assets.
type WaitScannerAssetsOptions struct {
	Ctx                context.Context
	ScannerReady       <-chan struct{}
	MetadataResultChan <-chan *models.MetadataScannerResult
	ScannerErrChan     <-chan error
	AssetWg            *sync.WaitGroup
	AssetErrChan       <-chan error
	DiscoveryReady     <-chan struct{}
}

// waitForScannerAndAssets waits for scanner and asset building to complete.
// The discoveryReady signal unblocks post-processing while image compression continues.
func (b *Engine) waitForScannerAndAssets(opts WaitScannerAssetsOptions) (*models.MetadataScannerResult, <-chan struct{}, error, error, error) {
	ctx := opts.Ctx
	scannerReady := opts.ScannerReady
	metadataResultChan := opts.MetadataResultChan
	scannerErrChan := opts.ScannerErrChan
	assetErrChan := opts.AssetErrChan
	discoveryReady := opts.DiscoveryReady

	select {
	case <-scannerReady:
	case <-ctx.Done():
		return nil, nil, nil, nil, ctx.Err()
	}

	// Receive scanner result and error.
	var metadataResult *models.MetadataScannerResult
	var scannerErr error
	select {
	case metadataResult = <-metadataResultChan:
		scannerErr = <-scannerErrChan
	case <-ctx.Done():
		return nil, nil, nil, nil, ctx.Err()
	}

	// Return discoveryReady separately so post-processing can unblock on it.
	// The caller will wait for assetWg separately if needed.
	discoverySignal := discoveryReady

	var assetErr error
	select {
	case err := <-assetErrChan:
		assetErr = err
	case <-ctx.Done():
		return nil, nil, nil, nil, ctx.Err()
	default:
	}

	return metadataResult, discoverySignal, scannerErr, assetErr, nil
}
