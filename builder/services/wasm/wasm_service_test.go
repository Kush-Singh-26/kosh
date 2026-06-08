package wasm

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
)

func TestWasmService_SkipInTestMode(t *testing.T) {
	fs := afero.NewMemMapFs()
	cfg := &config.Config{
		KoshSourceRoot: "/tmp/kosh",
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	ctx := buildctx.NewBuildContext(buildctx.ContextOptions{
		IsTesting:    true,
		IsDev:        true,
		IsCleanBuild: true,
		Scheduler:    scheduler.NewBuildScheduler(),
		Logger:       logger,
	})

	svc := NewService(Dependencies{
		Ctx:      ctx,
		Cfg:      cfg,
		Logger:   logger,
		SourceFs: fs,
	})

	updated, err := svc.CheckAndUpdate(context.Background())
	if err != nil {
		t.Errorf("CheckAndUpdate returned error in test mode: %v", err)
	}
	if updated {
		t.Error("CheckAndUpdate should not return updated=true in test mode")
	}

	err = svc.Deploy(context.Background(), nil)
	if err != nil {
		t.Errorf("Deploy returned error in test mode: %v", err)
	}
}
