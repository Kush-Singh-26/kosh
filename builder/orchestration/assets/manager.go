package assets

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"
)

// ManagerDependencies groups service dependencies for explicit injection.
type ManagerDependencies struct {
	Cfg      *config.Config
	Asset    services.AssetService
	Render   services.RenderService
	Logger   *slog.Logger
	Metrics  *metrics.BuildMetrics
	SourceFs afero.Fs
}

// Manager coordinates the asset building pipeline and change detection.
type Manager struct {
	deps ManagerDependencies

	// Internal state
	lastAssetHash uint64
	mu            sync.RWMutex
}

// NewManager initializes a new asset orchestration manager.
func NewManager(deps ManagerDependencies) *Manager {
	return &Manager{deps: deps}
}

// Reconfigure updates the manager with fresh build-time artifacts.
func (m *Manager) Reconfigure(sink fspkg.ArtifactSink, sourceFs afero.Fs) {
	if m.deps.Asset != nil {
		m.deps.Asset.ReconfigureForBuild(sink, sourceFs)
	}
	m.deps.SourceFs = sourceFs
}

// SetupBuilding starts the asset building process in a separate goroutine.
// Returns a signal channel for readiness, a wait group for completion, and an error channel.
func (m *Manager) SetupBuilding(ctx context.Context, contentAssetsChan chan []models.ScannedAsset) (<-chan struct{}, *sync.WaitGroup, <-chan error) {
	m.deps.Logger.Info("Building assets...")
	assetTimer := timeutil.StartPhase("Asset building")

	// Reset rendered assets in memory before starting fresh build pass
	m.deps.Render.SetAssets(map[string]string{})

	// Link content assets from scanner to asset service
	if setter, ok := m.deps.Asset.(interface {
		SetContentAssetsChannel(<-chan []models.ScannedAsset)
	}); ok {
		setter.SetContentAssetsChannel(contentAssetsChan)
	}

	// Create and register a fresh readiness signal for this build pass
	assetsReady := make(chan struct{})
	m.deps.Asset.SetAssetsReadySignal(assetsReady)

	// Ensure RenderService waits for these assets before entering render phase
	m.deps.Render.SetAssetsGate(assetsReady)

	assetErrChan := make(chan error, 1)
	var assetWg sync.WaitGroup
	assetWg.Add(1)

	go func() {
		defer assetWg.Done()
		if err := m.deps.Asset.Build(ctx); err != nil {
			assetErrChan <- err
		}
		close(assetErrChan)
		assetTimer.Stop()
	}()

	return assetsReady, &assetWg, assetErrChan
}

// CheckChanged computes a hash of the current asset map to detect changes since last site-wide render.
func (m *Manager) CheckChanged(ctx context.Context, assetsReady <-chan struct{}) bool {
	m.WaitForAvailability(ctx, assetsReady)
	assets := m.deps.Render.GetAssets()
	if len(assets) == 0 {
		return false
	}

	// Compute a simple stable hash of the asset map
	assetKeys := make([]string, 0, len(assets))
	for k := range assets {
		assetKeys = append(assetKeys, k)
	}
	sort.Strings(assetKeys)

	hasher := xxh3.New()
	for _, k := range assetKeys {
		_, _ = hasher.WriteString(k)
		_, _ = hasher.WriteString(assets[k])
	}
	currentAssetHash := hasher.Sum64()

	m.mu.Lock()
	defer m.mu.Unlock()

	changed := currentAssetHash != m.lastAssetHash
	if changed {
		m.lastAssetHash = currentAssetHash
	}
	return changed
}

// WaitForAvailability blocks until assets are ready or context is cancelled.
func (m *Manager) WaitForAvailability(ctx context.Context, assetsReady <-chan struct{}) {
	if len(m.deps.Render.GetAssets()) > 0 {
		return
	}
	if assetsReady == nil {
		return
	}
	select {
	case <-assetsReady:
	case <-ctx.Done():
	}
}

// BuildAssetOnly handles incremental CSS/JS changes by rebuilding assets and re-triggering post processing.
// This is used for fast feedback when only styling/scripting changes.
func (m *Manager) BuildAssetOnly(ctx context.Context, buildPass func(ctx context.Context) error) error {
	if m.deps.Metrics != nil {
		m.deps.Metrics.Reset()
	}
	m.deps.Render.SetAssets(map[string]string{})

	m.deps.Logger.Info("Building assets...")
	assetTimer := timeutil.StartPhase("Asset building")

	assets, err := m.deps.Asset.BuildForAssetChange(ctx)
	assetTimer.Stop()
	if err != nil {
		return fmt.Errorf("failed to build assets: %w", err)
	}

	m.deps.Render.SetAssets(assets)
	m.deps.Render.ClearRenderedFiles()
	m.deps.Render.SetAssetsGate(nil)

	// Delegate the rest of the build pass (scan, process posts, commit) back to Engine
	return buildPass(ctx)
}
