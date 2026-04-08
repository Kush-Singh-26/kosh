package wasm

import (
	"context"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

// Service handles WASM compilation and deployment for Search.
type Service interface {
	CheckAndUpdate(ctx context.Context) (bool, error)
	Deploy(ctx context.Context, sink fspkg.ArtifactSink) error
	SetSearchSourceDirty(dirty bool)
}
