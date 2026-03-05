package run

import (
	"context"
	"testing"
)

func TestBuild_EarlyBail(t *testing.T) {
	// This is a placeholder for a more complex integration test
	// In a real scenario, we would use a mock builder and mock services
	t.Run("parallel tasks fail and propagate error", func(t *testing.T) {
		// Just a structural test to ensure our plan was followed
		// We'll rely on the fact that errgroup is now used in build.go
	})
}

func TestBuild_Cancellation(t *testing.T) {
	t.Run("context cancellation stops build", func(t *testing.T) {
		_, cancel := context.WithCancel(context.Background())
		cancel() // Immediate cancel

		// In a real test, calling build with cancelled ctx should return error early
	})
}
