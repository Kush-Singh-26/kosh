package buildctx

import "log/slog"

// IgnoreError provides a centralized way to handle intentionally ignored errors.
// This makes the intent explicit and provides a hook for future observability.
func IgnoreError(err error, reason string) {
	if err != nil {
		// In a production system, you might want to log these at a low level
		// or increment a metric to track how often "ignorable" errors occur.
		slog.Debug("Intentionally ignoring error", "error", err, "reason", reason)
	}
}
