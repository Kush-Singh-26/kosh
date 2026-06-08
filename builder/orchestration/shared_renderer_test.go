package orchestration

import (
	"sync"
	"testing"

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

func TestMain(m *testing.M) {
	// Shutdown shared renderer if initialized after tests
	defer func() {
		if sharedRenderer != nil {
			_ = sharedRenderer.Close()
		}
	}()

	goleak.VerifyTestMain(m)
}
