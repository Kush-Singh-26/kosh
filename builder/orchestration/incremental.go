package orchestration

import (
	"context"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

func (b *Engine) BuildChanged(ctx context.Context, changedPath string, op fsnotify.Op) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	DevLogInfo("Change detected: " + filepath.Base(changedPath) + " " + op.String())

	if b.Watch != nil {
		b.Watch.EnqueueChange(changedPath, op)
	}
}
