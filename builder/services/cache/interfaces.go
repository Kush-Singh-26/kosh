package cache

import (
	"errors"
	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/cache/core"
	"github.com/Kush-Singh-26/kosh/builder/cache/gc"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/models"
)

// IsCacheMiss returns true if the error is a cache miss (sentinel ErrNoContent).
func IsCacheMiss(err error) bool {
	return errors.Is(err, core.ErrNoContent)
}

// Service provides cache operations for all services.
type Service interface {
	models.PostCache
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

	Stats() (*cache.Stats, error)
	IncrementBuildCount() (uint32, error)
	RunGC(config gc.Config) (*gc.Result, error)
	Close() error
}

// Dependencies holds all dependencies for CacheService.
type Dependencies struct {
	Ctx     *buildctx.BuildContext
	Manager *cache.Manager
	Logger  *slog.Logger
}
