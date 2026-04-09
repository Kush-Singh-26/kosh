package orchestration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/ui"
)

var rebuildLevel = slog.Level(slog.LevelWarn + 1)

// DevLogChange logs a file change event in dev mode.
func DevLogChange(path, changeType string) {
	slog.Log(context.Background(), rebuildLevel, "file change", "path", path, "type", changeType)
}

// DevLogRebuild logs a rebuild action in dev mode.
func DevLogRebuild(action string) {
	slog.Log(context.Background(), rebuildLevel, action)
}

// DevLogSuccess logs a success message in dev mode.
func DevLogSuccess(message string) {
	slog.Log(context.Background(), slog.LevelInfo, message)
}

// DevLogSkip logs a skipped action in dev mode.
func DevLogSkip(message string) {
	slog.Log(context.Background(), rebuildLevel, message, "skipped", true)
}

// DevLogInfo logs an informational message in dev mode.
func DevLogInfo(message string) {
	slog.Log(context.Background(), slog.LevelInfo, message)
}

// DevLogError logs an error message in dev mode.
func DevLogError(message string) {
	slog.Log(context.Background(), slog.LevelError, message)
}

// HTTPLog logs an HTTP request with timing information.
func HTTPLog(method, path string, status int, duration time.Duration) {
	slog.Info("http request",
		"method", method,
		"path", path,
		"status", status,
		"duration", duration,
		"http", true,
	)
}

type consoleHandler struct {
	output     io.Writer
	mu         sync.Mutex // serializes writes to output and handler state
	timeFormat string
	attrs      []slog.Attr
	group      string
	reporter   ui.Reporter
}

// NewConsoleHandler constructs a console handler for slog output.
func NewConsoleHandler(output io.Writer, reporter ui.Reporter) *consoleHandler {
	return &consoleHandler{
		output:     output,
		timeFormat: "15:04:05",
		reporter:   reporter,
	}
}

// Enabled reports whether the handler handles the given level.
func (h *consoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

// Handle formats and writes a log record.
func (h *consoleHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.reporter != nil {
		isHTTP := false
		// Only handle Warn, Error, and important Info (like HTTP requests)
		if r.Level < slog.LevelWarn {
			// Special handling for HTTP logs
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "http" {
					if a.Value.Kind() == slog.KindBool && a.Value.Bool() {
						isHTTP = true
					}
					return false
				}
				return true
			})
		}

		var b strings.Builder
		if isHTTP {
			// Format HTTP log line manually for a clean look in the UI
			var method, path string
			var status int
			var durationStr string
			r.Attrs(func(a slog.Attr) bool {
				switch a.Key {
				case "method":
					if a.Value.Kind() == slog.KindString {
						method = a.Value.String()
					}
				case "path":
					if a.Value.Kind() == slog.KindString {
						path = a.Value.String()
					}
				case "status":
					if a.Value.Kind() == slog.KindInt64 {
						status = int(a.Value.Int64())
					}
				case "duration":
					if a.Value.Kind() == slog.KindDuration {
						durationStr = fmt.Sprintf("%dms", a.Value.Duration().Milliseconds())
					} else {
						durationStr = a.Value.String()
					}
				}
				return true
			})

			fmt.Fprintf(&b, "%s %s %d %s", method, path, status, durationStr)
		} else {
			// Not an HTTP log - use standard message formatting
			b.WriteString(r.Message)
			if r.NumAttrs() > 0 {
				r.Attrs(func(a slog.Attr) bool {
					fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())
					return true
				})
			}
		}
		msg := b.String()

		switch {
		case r.Level >= slog.LevelError:
			h.reporter.Error("%s", nil, msg)
		case r.Level >= slog.LevelWarn:
			h.reporter.Warn("%s", msg)
		default:
			h.reporter.Info("%s", msg)
		}
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Use fmt.Sprintf to avoid races in fmt package's internal state
	color := getLevelColor(r.Level)
	timeStr := r.Time.Format(h.timeFormat)

	line := fmt.Sprintf("\033[90m%s\033[0m %s ", timeStr, color)
	_, _ = fmt.Fprintf(h.output, "%s", line)
	h.writeMessage(r.Message)

	// Write handler attributes
	for _, a := range h.attrs {
		h.writeAttr(a)
	}

	// Write record attributes
	if r.NumAttrs() > 0 {
		r.Attrs(func(a slog.Attr) bool {
			h.writeAttr(a)
			return true
		})
	}

	_, _ = fmt.Fprintf(h.output, "\033[0m\n")
	return nil
}

func (h *consoleHandler) writeMessage(msg string) {
	if h.group != "" {
		_, _ = fmt.Fprintf(h.output, "[\033[1m%s\033[0m] %s", h.group, msg)
	} else {
		_, _ = fmt.Fprintf(h.output, "%s", msg)
	}
}

func (h *consoleHandler) writeAttr(a slog.Attr) {
	value := a.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%s", a.Key, value.String())
	case slog.KindInt64:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%d", a.Key, value.Int64())
	case slog.KindUint64:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%d", a.Key, value.Uint64())
	case slog.KindFloat64:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%.2f", a.Key, value.Float64())
	case slog.KindBool:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%v", a.Key, value.Bool())
	case slog.KindDuration:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%s", a.Key, value.Duration())
	case slog.KindTime:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%s", a.Key, value.Time().Format(time.RFC3339))
	case slog.KindGroup:
		attrs := value.Group()
		for _, ga := range attrs {
			h.writeAttr(ga)
		}
	default:
		_, _ = fmt.Fprintf(h.output, " \033[36m%s\033[0m=%v", a.Key, value)
	}
}

// WithAttrs returns a handler with additional attributes.
func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &consoleHandler{
		output:     h.output,
		timeFormat: h.timeFormat,
		attrs:      newAttrs,
		group:      h.group,
		reporter:   h.reporter,
	}
}

// WithGroup returns a handler with the given group name.
func (h *consoleHandler) WithGroup(name string) slog.Handler {
	return &consoleHandler{
		output:     h.output,
		timeFormat: h.timeFormat,
		attrs:      h.attrs,
		group:      name,
		reporter:   h.reporter,
	}
}

func getLevelColor(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "\033[90mDBG\033[0m"
	case slog.LevelInfo:
		return "\033[96mINF\033[0m"
	case slog.LevelWarn:
		return "\033[93mWRN\033[0m"
	case slog.LevelError:
		return "\033[91mERR\033[0m"
	default:
		if level == rebuildLevel {
			return "\033[92mREB\033[0m"
		}
		return "\033[96mINF\033[0m"
	}
}

// InitLogger initializes the default logger and optionally wires a reporter.
func InitLogger(reporters ...ui.Reporter) *slog.Logger {
	var r ui.Reporter
	if len(reporters) > 0 {
		r = reporters[0]
	}
	return slog.New(NewConsoleHandler(os.Stdout, r))
}
