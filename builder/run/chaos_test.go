package run

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/Kush-Singh-26/kosh/builder/utils"
	"github.com/spf13/afero"
)

func TestBuild_DiskFullGracefulFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	// Enable test mode to skip absolute path resolution
	utils.TestingMode = true
	defer func() { utils.TestingMode = false }()

	cfg := config.LoadFs(fs, []string{})
	cfg.OutputDir = "public"

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := &sync.Map{}
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	// Initial successful build
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := services.NewRenderService(rnd, logger)
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	postSvc := services.NewPostService(cfg, nil, renderSvc, logger, buildMetrics, mdPool, nativeRenderer, fs, nil, nil)
	metadataScanner := services.NewMetadataScanner()

	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := NewBuilderFromManual(cfg, renderSvc, assetSvc, postSvc, metadataScanner, logger, buildMetrics, fs, mdPool, nativeRenderer)
	b.Sink = sink
	b.Tx = tx

	if err := b.Build(context.Background()); err != nil {
		t.Fatalf("Initial build failed: %v", err)
	}

	// Now simulate disk full during build
	// We use a clean build to ensure it hits the sink
	cfg.ForceRebuild = true
	failingSink := &testutil.FailingSink{Err: errors.New("no space left on device")}
	b.Sink = failingSink

	err := b.Build(context.Background())
	if err == nil {
		t.Error("Build should have failed due to disk full")
	} else {
		t.Logf("Caught expected error: %v", err)
	}
}
