package assets

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/spf13/afero"
)

func TestManager_Reconfigure(t *testing.T) {
	mockAsset := &mocks.MockAssetService{}
	fs := afero.NewMemMapFs()
	m := NewManager(ManagerDependencies{
		Asset:    mockAsset,
		SourceFs: fs,
	})

	newFs := afero.NewMemMapFs()
	m.Reconfigure(nil, newFs)

	if m.deps.SourceFs != newFs {
		t.Error("Reconfigure failed to update SourceFs")
	}
}

func TestManager_CheckChanged(t *testing.T) {
	mockRender := &mocks.MockRenderService{}
	m := NewManager(ManagerDependencies{
		Render: mockRender,
		Logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	})

	// Initial check with no assets
	if m.CheckChanged(context.Background(), nil) {
		t.Error("CheckChanged should return false when no assets exist")
	}

	// Set some assets
	mockRender.SetAssets(map[string]string{"main.css": "hash1"})
	if !m.CheckChanged(context.Background(), nil) {
		t.Error("CheckChanged should return true when assets are first detected")
	}

	// Same assets again
	if m.CheckChanged(context.Background(), nil) {
		t.Error("CheckChanged should return false when assets haven't changed")
	}

	// Change assets
	mockRender.SetAssets(map[string]string{"main.css": "hash2"})
	if !m.CheckChanged(context.Background(), nil) {
		t.Error("CheckChanged should return true when assets change")
	}
}

func TestManager_SetupBuilding(t *testing.T) {
	mockAsset := &mocks.MockAssetService{}
	mockRender := &mocks.MockRenderService{}
	cfg := &config.Config{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	m := NewManager(ManagerDependencies{
		Asset:  mockAsset,
		Render: mockRender,
		Cfg:    cfg,
		Logger: logger,
	})

	assetsReady, discoveryReady, wg, errChan := m.SetupBuilding(context.Background(), nil, true)

	if assetsReady == nil || discoveryReady == nil || wg == nil || errChan == nil {
		t.Error("SetupBuilding returned nil channels/wg")
	}

	wg.Wait()
}
