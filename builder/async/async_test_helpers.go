package async

import (
	"io"
	"log/slog"
	"testing"
)

// helperLogger creates a test logger.
func helperLogger(t *testing.T) *slog.Logger {
	t.Helper()
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
