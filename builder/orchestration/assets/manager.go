package assets

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services/asset"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

// ManagerDependencies groups service dependencies for explicit injection.
type ManagerDependencies struct {
	Cfg      *config.Config
	Asset    asset.Service
	Render   render.Service
	Logger   *slog.Logger
	Metrics  *metrics.BuildMetrics
	SourceFs afero.Fs
}

// Manager coordinates the asset building pipeline and change detection.
type Manager struct {
	deps ManagerDependencies

	// Internal state
	lastAssetHash uint64
	mu            sync.RWMutex // protects lastAssetHash
}

// NewManager initializes a new asset orchestration manager.
func NewManager(deps ManagerDependencies) *Manager {
	return &Manager{deps: deps}
}

// Reconfigure updates the manager with fresh build-time artifacts.
func (managerInstance *Manager) Reconfigure(sink fspkg.ArtifactSink, sourceFs afero.Fs) {
	if managerInstance.deps.Asset != nil {
		managerInstance.deps.Asset.ReconfigureForBuild(sink, sourceFs)
	}
	managerInstance.deps.SourceFs = sourceFs
}

// ReconfigureWithLogger updates the manager logger for the current build pass.
func (managerInstance *Manager) ReconfigureWithLogger(logger *slog.Logger) {
	managerInstance.deps.Logger = logger
}

// SetupBuilding starts the asset building process in a separate goroutine.
// Returns a signal channel for full readiness, discovery signal, wait group, and error channel.
func (managerInstance *Manager) SetupBuilding(ctx context.Context, contentAssetsChan chan []models.ScannedAsset, force bool) (<-chan struct{}, <-chan struct{}, *sync.WaitGroup, <-chan error) {
	managerInstance.deps.Logger.Info("Building assets...")
	timer := timeutil.StartPhase("Asset building")

	assets.ResetConvertedImages()
	assets.ResetWasmExecForBuild()
	managerInstance.deps.Render.SetAssets(map[string]string{})

	skipImages := managerInstance.determineSkipImages(force)
	managerInstance.configureAssetService(contentAssetsChan)

	assetsReady := make(chan struct{})
	managerInstance.deps.Asset.SetAssetsReadySignal(assetsReady)
	managerInstance.deps.Render.SetAssetsGate(assetsReady)

	discoveryCh := make(chan struct{})
	managerInstance.deps.Asset.SetDiscoveryReady(discoveryCh)

	errChan := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)

	managerInstance.runAssetBuild(ctx, skipImages, &wg, errChan, timer)

	return assetsReady, managerInstance.deps.Asset.DiscoveryReady(), &wg, errChan
}

func (managerInstance *Manager) determineSkipImages(force bool) bool {
	if force || managerInstance.deps.Cfg.CacheDir == "" {
		return false
	}
	managerInstance.deps.Logger.Debug("Checking static fingerprint", "force", force)
	current, err := asset.ComputeStaticFingerprint(managerInstance.deps.SourceFs, asset.GetStaticDirs(managerInstance.deps.Cfg))
	if err != nil {
		managerInstance.deps.Logger.Debug("Failed to compute static fingerprint", "error", err)
		return false
	}
	cached, err := asset.LoadStaticFingerprint(managerInstance.deps.Cfg.CacheDir)
	if err == nil && current == cached {
		managerInstance.deps.Logger.Debug("Static fingerprint matches, will skip image processing")
		return true
	}
	managerInstance.deps.Logger.Debug("Static fingerprint mismatch or not cached", "current", current, "cached", cached, "loadErr", err)
	_ = asset.SaveStaticFingerprint(managerInstance.deps.Cfg.CacheDir, current)
	return false
}

func (managerInstance *Manager) configureAssetService(contentAssetsChan chan []models.ScannedAsset) {
	if setter, ok := managerInstance.deps.Asset.(interface {
		SetContentAssetsChannel(<-chan []models.ScannedAsset)
	}); ok {
		setter.SetContentAssetsChannel(contentAssetsChan)
	}
}

func (managerInstance *Manager) runAssetBuild(ctx context.Context, skipImages bool, wg *sync.WaitGroup, errChan chan error, timer *timeutil.PhaseTimer) {
	async.FireAndForgetWithCleanup(async.FireAndForgetCleanupOptions{
		Ctx:       ctx,
		Logger:    managerInstance.deps.Logger,
		Operation: "asset build",
		Fn: func() error {
			if err := managerInstance.deps.Asset.BuildWithOptions(ctx, skipImages); err != nil {
				errChan <- err
			}
			return nil
		},
		Cleanup: func() {
			timer.Stop()
			wg.Done()
		},
	})
}

// CheckChanged computes a hash of the current asset map to detect changes since last site-wide render.
func (managerInstance *Manager) CheckChanged(ctx context.Context, assetsReady <-chan struct{}) bool {
	managerInstance.WaitForAvailability(ctx, assetsReady)
	assetMap := managerInstance.deps.Render.GetAssets()
	if len(assetMap) == 0 {
		return false
	}

	// Compute a simple stable hash of the asset map
	assetKeys := make([]string, 0, len(assetMap))
	for assetKey := range assetMap {
		assetKeys = append(assetKeys, assetKey)
	}
	sort.Strings(assetKeys)

	hasher := xxh3.New()
	for _, assetKey := range assetKeys {
		_, _ = hasher.WriteString(assetKey)
		_, _ = hasher.WriteString(assetMap[assetKey])
	}
	currentAssetHash := hasher.Sum64()

	managerInstance.mu.Lock()
	defer managerInstance.mu.Unlock()

	changed := currentAssetHash != managerInstance.lastAssetHash
	if changed {
		managerInstance.lastAssetHash = currentAssetHash
	}
	return changed
}

// WaitForAvailability blocks until assets are ready or context is canceled.
func (managerInstance *Manager) WaitForAvailability(workingContext context.Context, assetsReady <-chan struct{}) {
	if len(managerInstance.deps.Render.GetAssets()) > 0 {
		return
	}
	if assetsReady == nil {
		return
	}
	select {
	case <-assetsReady:
	case <-workingContext.Done():
	}
}

// BuildAssetOnly handles incremental CSS/JS changes by rebuilding assets and re-triggering Content processing.
func (managerInstance *Manager) BuildAssetOnly(ctx context.Context, buildPass func(ctx context.Context) error) error {
	return managerInstance.BuildAssetOnlyWithOptions(ctx, buildPass, true)
}

// BuildAssetOnlyWithOptions handles incremental CSS/JS changes with options.
// forceImages: if true, always process images; if false, skip image processing (for CSS/JS-only changes)
func (managerInstance *Manager) BuildAssetOnlyWithOptions(ctx context.Context, buildPass func(ctx context.Context) error, forceImages bool) error {
	if managerInstance.deps.Metrics != nil {
		managerInstance.deps.Metrics.Reset()
	}

	managerInstance.deps.Logger.Info("Building assets...")
	assetTimer := timeutil.StartPhase("Asset building")

	newAssets, buildError := managerInstance.deps.Asset.BuildForAssetChangeWithOptions(ctx, forceImages)
	assetTimer.Stop()
	if buildError != nil {
		return fmt.Errorf("failed to build assets: %w", buildError)
	}

	// Merge with existing assets to preserve image mappings
	currentAssets := managerInstance.deps.Render.GetAssets()
	if currentAssets == nil {
		currentAssets = make(map[string]string)
	}
	for assetKey, assetValue := range newAssets {
		currentAssets[assetKey] = assetValue
	}

	managerInstance.deps.Render.SetAssets(currentAssets)
	managerInstance.deps.Render.ClearRenderedFiles()
	managerInstance.deps.Render.SetAssetsGate(nil)

	// Delegate the rest of the build pass (scan, process posts, commit) back to Engine
	return buildPass(ctx)
}
