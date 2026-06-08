package orchestration

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
)

var (
	sharedRenderer     *native.Renderer
	sharedRendererOnce sync.Once
)

// getSharedRenderer returns a lazily-initialized shared native renderer for tests.
// This prevents multiple QuickJS runtimes from competing and deadlocking during
// concurrent/sequential test execution in the orchestration package.
func getSharedRenderer() *native.Renderer {
	sharedRendererOnce.Do(func() {
		sharedRenderer = native.New(native.WithWorkers(1))
	})
	return sharedRenderer
}

func testCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 120*time.Second)
}

func TestMain(m *testing.M) {
	// Shutdown shared renderer if initialized after tests
	defer func() {
		if sharedRenderer != nil {
			_ = sharedRenderer.Close()
		}
	}()

	goleak.VerifyTestMain(m)
}
