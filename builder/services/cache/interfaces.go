package cache

import (
"errors"
"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	cachepkg "github.com/Kush-Singh-26/kosh/builder/cache"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"

"github.com/Kush-Singh-26/kosh/builder/models"
)

// IsCacheMiss returns true if the error is a cache miss (sentinel ErrNoContent).
func IsCacheMiss(err error) bool {
	return errors.Is(err, core.ErrNoContent)
}

// Service provides cache operations for all services.
type Service interface {
	models.ContentCache
	models.SearchCache
	models.SocialCardCache
	models.BuildArtifactCache
	models.FragmentCache

	GetGraphHash() (string, error)
	SetGraphHash(hash string) error
	GetWasmHash() (string, error)
	SetWasmHash(hash string) error
	GetSearchHash() (string, error)
	SetSearchHash(hash string) error

Stats() (*core.CacheStats, error)
	IncrementBuildCount() (uint32, error)
	RunGC(config gc.Config) (*gc.Result, error)
	Close() error
}

// Dependencies holds all dependencies for CacheService.
type Dependencies struct {
	Ctx *buildctx.BuildContext
	Manager *cachepkg.Manager
	Logger *slog.Logger
}

