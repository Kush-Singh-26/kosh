package run

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/services/mocks"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
)

type diskFullFs struct {
	afero.Fs
	full bool
}

func (f *diskFullFs) Create(name string) (afero.File, error) {
	if f.full {
		return nil, errors.New("disk full")
	}
	return f.Fs.Create(name)
}

func (f *diskFullFs) MkdirAll(path string, perm os.FileMode) error {
	if f.full {
		return errors.New("disk full")
	}
	return f.Fs.MkdirAll(path, perm)
}

func TestBuild_DiskFullGracefulFailure(t *testing.T) {
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	cfg := config.LoadFs(fs, []string{})
	cfg.OutputDir = "public"

	logger := InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	diagramCache := &sync.Map{}
	mdPool := &sync.Pool{
		New: func() interface{} {
			return mdParser.New(cfg, nativeRenderer, diagramCache)
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
