package orchestration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/fs/tx"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/incremental"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/Kush-Singh-26/kosh/builder/ui"
)

type configReloadBuilder struct {
	*Engine
	buildCalls int
	lockHeld   bool
}

func (b *configReloadBuilder) BuildLocked(context.Context) error {
	b.buildCalls++
	b.lockHeld = !b.State.BuildMu.TryLock()
	if !b.lockHeld {
		b.State.BuildMu.Unlock()
	}
	return nil
}

func TestConfigWatchReloadDoesNotDeadlock(t *testing.T) {
	for _, path := range []string{"kosh.yaml", filepath.Join(t.TempDir(), "kosh.yaml")} {
		t.Run(path, func(t *testing.T) {
			sourceFs := afero.NewMemMapFs()
			if err := afero.WriteFile(sourceFs, "kosh.yaml", []byte("title: Reloaded\n"), 0644); err != nil {
				t.Fatal(err)
			}
			engine := NewEngine(WithDeps(EngineDependencies{
				Config:   &config.Config{OutputDir: t.TempDir()},
				SourceFs: sourceFs,
				Content:  &mockPostService{},
				Asset:    &mocks.MockAssetService{},
				Render:   mocks.NewMockRenderService(),
			}))
			t.Cleanup(engine.Close)
			engine.artifactSink = testutil.NewMemSink()
			engine.buildTransaction = testutil.NewMockTransaction(engine.Cfg.OutputDir)
			builder := &configReloadBuilder{Engine: engine}
			engine.Incremental = incremental.NewManager(incremental.ManagerDependencies{
				Cfg: engine.Cfg, Logger: engine.Deps.Logger, Builder: builder,
			})
			done := make(chan struct{})
			go func() {
				engine.handleWatchChange(watch.ChangeEvent{Path: path, Op: fsnotify.Write})
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("config reload deadlocked")
			}
			if builder.buildCalls != 1 || !builder.lockHeld {
				t.Fatalf("build calls = %d, lock held = %v", builder.buildCalls, builder.lockHeld)
			}
			if engine.Cfg.Title != "Reloaded" || !engine.State.IsCleanBuild || !engine.State.ForceRerender.Load() || !engine.State.ForceGenerators.Load() {
				t.Fatal("reload did not update config and invalidate build state")
			}
			if !engine.State.BuildMu.TryLock() {
				t.Fatal("build lock was not released")
			}
			engine.State.BuildMu.Unlock()
		})
	}
}

// Release the asset failure only after the scanner/discovery error check has returned.
type lateAssetReporter struct {
	ui.Reporter
	release chan struct{}
}

func (r *lateAssetReporter) EndPhase(phase ui.Phase, _ time.Duration) {
	if phase == ui.PhaseScan {
		close(r.release)
	}
}

type lateFailAssetService struct {
	mocks.MockAssetService
	discovery chan struct{}
	ready     chan struct{}
	release   <-chan struct{}
	err       error
}

func (s *lateFailAssetService) SetDiscoveryReady(ch chan struct{})    { s.discovery = ch }
func (s *lateFailAssetService) SetAssetsReadySignal(ch chan struct{}) { s.ready = ch }
func (s *lateFailAssetService) BuildWithOptions(ctx context.Context, _ bool) error {
	close(s.discovery)
	defer close(s.ready)
	select {
	case <-s.release:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestBuildRejectsLateAssetFailureBeforePublish(t *testing.T) {
	outputDir := filepath.Join(t.TempDir(), "public")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outputDir, "previous.html")
	if err := os.WriteFile(marker, []byte("previous build"), 0644); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	assetErr := errors.New("late image encoding failure")
	assetSvc := &lateFailAssetService{release: release, err: assetErr}
	engine := NewEngine(WithDeps(EngineDependencies{
		Config:   &config.Config{OutputDir: outputDir},
		SourceFs: afero.NewMemMapFs(),
		Asset:    assetSvc,
		Content:  &mockPostService{},
		Render:   mocks.NewMockRenderService(),
		Scanner:  &mocks.MockScanner{Result: &models.MetadataScannerResult{}},
		Wasm:     &mocks.MockWasmService{},
		Metrics:  metrics.NewBuildMetrics(),
		Reporter: &lateAssetReporter{Reporter: ui.NewReporter(false), release: release},
	}))
	t.Cleanup(engine.Close)
	err := engine.Build(t.Context())
	if !errors.Is(err, assetErr) {
		t.Fatalf("Build error = %v, want %v", err, assetErr)
	}
	if engine.buildTransaction.(*tx.DirectoryTx).IsCommitted() {
		t.Fatal("failed asset build was committed")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "previous build" {
		t.Fatalf("previous output was changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, ".nojekyll")); !os.IsNotExist(err) {
		t.Fatalf("new output was published: %v", err)
	}
}
