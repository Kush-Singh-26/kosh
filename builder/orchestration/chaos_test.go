package orchestration

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

func TestBuild_DiskFullGracefulFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	// Enable test mode to skip absolute path resolution
	cfg := config.LoadFs(fs, []string{})
	cfg.OutputDir = "public"

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	t.Cleanup(func() { _ = nativeRenderer.Close() })
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	// Initial successful build
	rnd := renderer.NewWithFs(fs, false, nil, cfg.TemplateDir, true, logger)
	renderSvc := render.NewService(render.Dependencies{
		Ctx:      buildCtx.NewBuildContext(true, false, false, scheduler.NewBuildScheduler(), logger),
		Renderer: rnd,
		Logger:   logger,
	})
	assetSvc := &mocks.MockAssetService{}
	assetSvc.SetMetrics(buildMetrics)
	wasmSvc := &mocks.MockWasmService{}
	postSvc := post.NewService(post.Dependencies{
		Ctx:            buildCtx.NewBuildContext(true, false, false, scheduler.NewBuildScheduler(), logger),
		Cfg:            cfg,
		Renderer:       renderSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
		SourceFs:       fs,
	})
	metadataScanner := scanner.NewScanner()
	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b := NewEngineFromManual(EngineDependencies{
		Config:         cfg,
		Render:         renderSvc,
		Asset:          assetSvc,
		Post:           postSvc,
		Scanner:        metadataScanner,
		Wasm:           wasmSvc,
		Logger:         logger,
		Metrics:        buildMetrics,
		SourceFs:       fs,
		MdPool:         mdPool,
		NativeRenderer: nativeRenderer,
	})
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
