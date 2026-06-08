//go:build !wasm

package wasm

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/assets"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

// Dependencies holds all dependencies for WasmService.
type Dependencies struct {
	Ctx      *buildctx.BuildContext
	Cfg      *config.Config
	Logger   *slog.Logger
	SourceFs afero.Fs
}

type wasmService struct {
	ctx      *buildctx.BuildContext
	cfg      *config.Config
	logger   *slog.Logger
	sourceFs afero.Fs
}

// NewService creates a new WasmService with the given dependencies.
func NewService(dependencies Dependencies) Service {
	return &wasmService{
		ctx:      dependencies.Ctx,
		cfg:      dependencies.Cfg,
		logger:   dependencies.Logger,
		sourceFs: dependencies.SourceFs,
	}
}

// CheckAndUpdate is a no-op in the simplified manual workflow.
func (service *wasmService) CheckAndUpdate(_ context.Context) (bool, error) {
	if service.cfg.KoshSourceRoot == "" {
		return false, nil
	}

	currentHash, err := assets.CalculateSearchSourceHash(service.cfg.KoshSourceRoot)
	if err != nil {
		return false, nil
	}

	// Developer reminder if embedded is stale
	if currentHash != assets.SearchSourceHash {
		service.logger.Info("Search source has changed. Run 'go run scripts/rebuild_search.go' to update the embedded binary.", "source_hash", currentHash, "embedded_hash", assets.SearchSourceHash)
	}

	return false, nil
}

// Deploy ensures the search WASM is available in the output sink.
func (service *wasmService) Deploy(_ context.Context, sink fspkg.ArtifactSink) error {
	// Skip WASM operations in test mode unless E2E is requested
	if service.ctx != nil && service.ctx.IsTesting && os.Getenv("KOSH_E2E") != "1" {
		return nil
	}

	// Always deploy the embedded WASM in the simplified workflow.
	if _, err := assets.CheckWASM(sink, service.cfg.CacheDir); err != nil {
		return err
	}

	// Deploy wasm_exec.js (Go WASM runtime stub)
	if err := assets.DeployWasmExec(sink); err != nil {
		return err
	}

	return nil
}
