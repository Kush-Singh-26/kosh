package orchestration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services"
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

	b := &Engine{
		Cfg: cfg,
		Ctx: buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Deps: EngineDependencies{
			Wasm:   wasmSvc,
			Render: renderSvc,
			Asset:  assetSvc,
			Post:   postSvc,
		},
		Logger:   logger,
		Metrics:  buildMetrics,
		Sink:     sink,
		Tx:       tx,
		SourceFs: fs,
	}

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

	b := &Engine{
		Deps: EngineDependencies{
			Render: renderSvc,
			Asset:  assetSvc,
		},
		Logger: logger,
		Ctx:    buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
	}

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

	b := &Engine{
		Cfg: cfg,
		Ctx: buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Deps: EngineDependencies{
			Scanner: scanner,
		},
		SourceFs: fs,
	}

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

	b := &Engine{
		Deps: EngineDependencies{
			Render: renderSvc,
		},
		Logger: logger,
		Ctx:    buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
	}
	b.State.LastAssetHash = 0

	assetsReady := make(chan struct{})
	close(assetsReady)

	// First call - should be true as lastAssetHash is 0
	changed := b.checkAssetsChanged(assetsReady)
	if !changed {
		t.Error("expected assets to be marked as changed on first call")
	}

	// Second call - should be false as hash matches
	changed = b.checkAssetsChanged(assetsReady)
	if changed {
		t.Error("expected assets to be marked as unchanged on second call")
	}

	// Change assets
	renderSvc.SetAssets(map[string]string{
		"style.css": "hash2",
	})
	changed = b.checkAssetsChanged(assetsReady)
	if !changed {
		t.Error("expected assets to be marked as changed after modification")
	}
}

func TestShouldSkipSiteWideRendering(t *testing.T) {
	logger := InitLogger()
	b := &Engine{
		Cfg: &config.Config{
			IsDev: true,
		},
		Ctx: buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
	}

	cb := &services.MetadataContext{
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

	b := &Engine{
		Cfg: cfg,
		Ctx: buildCtx.NewBuildContext(true, false, false, scheduler.GetGlobalScheduler(), logger),
		Deps: EngineDependencies{
			Wasm:   wasmSvc,
			Render: renderSvc,
		},
		Sink:    sink,
		Tx:      tx,
		Metrics: m,
		Logger:  logger,
	}

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
