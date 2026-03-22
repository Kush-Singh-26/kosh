package orchestration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

func TestSetupPhase(t *testing.T) {
	fs := afero.NewMemMapFs()
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			OutputDir: "public",
		},
	}
	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()

	wasmSvc := &mocks.MockWasmService{}
	renderSvc := mocks.NewMockRenderService()
	assetSvc := &mocks.MockAssetService{}
	postSvc := &mockPostService{}
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := NewEngineFromManual(cfg, renderSvc, assetSvc, postSvc, nil, wasmSvc, logger, buildMetrics, fs, nil, nil)
	b.Sink = sink
	b.Tx = tx

	ctx := context.Background()
	res, err := b.setupPhase(ctx)
	if err != nil {
		t.Fatalf("setupPhase failed: %v", err)
	}

	if res.wasmWg == nil {
		t.Error("expected wasmWg to be non-nil")
	}
}

func TestAssetPhase(t *testing.T) {
	logger := InitLogger()
	renderSvc := mocks.NewMockRenderService()
	assetSvc := &mocks.MockAssetService{}
	cfg := &config.Config{}

	b := NewEngineFromManual(cfg, renderSvc, assetSvc, nil, nil, nil, logger, nil, nil, nil, nil)

	contentAssetsChan := make(chan []models.ScannedAsset, 1)
	ctx := context.Background()

	res := b.assetPhase(ctx, contentAssetsChan)
	if res.assetsReady == nil {
		t.Error("expected assetsReady channel")
	}
	if res.assetWg == nil {
		t.Error("expected assetWg")
	}
}

func TestScanPhase(t *testing.T) {
	fs := afero.NewMemMapFs()
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			ContentDir: "content",
		},
	}
	_ = fs.MkdirAll("content", 0755)

	scanner := &mocks.MockScanner{
		Result: &models.MetadataScannerResult{
			ContentAssets: []models.ScannedAsset{},
		},
	}

	logger := InitLogger()

	b := NewEngineFromManual(cfg, nil, nil, nil, scanner, nil, logger, nil, fs, nil, nil)

	contentAssetsChan := make(chan []models.ScannedAsset, 1)
	ctx := context.Background()

	res := b.scanPhase(ctx, contentAssetsChan)
	if res.fileChan == nil {
		t.Error("expected fileChan")
	}

	// Wait for scanner to complete
	<-res.scannerReady

	select {
	case <-res.metadataResultChan:
		// success
	case err := <-res.scannerErrChan:
		if err != nil {
			t.Errorf("scanner failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("scanPhase timed out")
	}
}

func TestCheckAssetsChanged(t *testing.T) {
	logger := InitLogger()
	renderSvc := mocks.NewMockRenderService()
	renderSvc.SetAssets(map[string]string{
		"style.css": "hash1",
	})
	cfg := &config.Config{}

	b := NewEngineFromManual(cfg, renderSvc, nil, nil, nil, nil, logger, nil, nil, nil, nil)

	assetsReady := make(chan struct{})
	close(assetsReady)
	ctx := context.Background()

	// First call - should be true as lastAssetHash is 0
	changed := b.Assets.CheckChanged(ctx, assetsReady)
	if !changed {
		t.Error("expected assets to be marked as changed on first call")
	}

	// Second call - should be false as hash matches
	changed = b.Assets.CheckChanged(ctx, assetsReady)
	if changed {
		t.Error("expected assets to be marked as unchanged on second call")
	}

	// Change assets
	renderSvc.SetAssets(map[string]string{
		"style.css": "hash2",
	})
	changed = b.Assets.CheckChanged(ctx, assetsReady)
	if !changed {
		t.Error("expected assets to be marked as changed after modification")
	}
}

func TestShouldSkipSiteWideRendering(t *testing.T) {
	logger := InitLogger()
	cfg := &config.Config{
		IsDev: true,
	}
	b := NewEngineFromManual(cfg, nil, nil, nil, nil, nil, logger, nil, nil, nil, nil)

	cb := &post.MetadataContext{
		AnyPostChanged: false,
	}

	// In dev mode, not clean build, no changes -> should skip
	if !b.shouldSkipSiteWideRendering(cb, false) {
		t.Error("expected to skip site-wide rendering")
	}

	// Any post changed -> should not skip
	cb.AnyPostChanged = true
	if b.shouldSkipSiteWideRendering(cb, false) {
		t.Error("expected not to skip when posts changed")
	}
	cb.AnyPostChanged = false

	// Assets changed -> should not skip
	if b.shouldSkipSiteWideRendering(cb, true) {
		t.Error("expected not to skip when assets changed")
	}

	// Clean build -> should not skip
	b.State.IsCleanBuild = true
	if b.shouldSkipSiteWideRendering(cb, false) {
		t.Error("expected not to skip on clean build")
	}
}

func TestFinalizePhase(t *testing.T) {
	_ = afero.NewMemMapFs()
	cfg := &config.Config{
		PathConfig: config.PathConfig{
			OutputDir: "public",
		},
	}

	wasmSvc := &mocks.MockWasmService{}
	renderSvc := mocks.NewMockRenderService()
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")
	m := metrics.NewBuildMetrics()
	logger := InitLogger()

	b := NewEngineFromManual(cfg, renderSvc, nil, nil, nil, wasmSvc, logger, m, nil, nil, nil)
	b.Sink = sink
	b.Tx = tx

	var wasmWg sync.WaitGroup
	ctx := context.Background()

	err := b.finalizePhase(ctx, &wasmWg)
	if err != nil {
		t.Fatalf("finalizePhase failed: %v", err)
	}

	// Check if .nojekyll was written
	if _, ok := sink.Files["public/.nojekyll"]; !ok {
		t.Error(".nojekyll was not written")
	}

	// Check if transaction was committed
	if !tx.Committed {
		t.Error("transaction was not committed")
	}
}
