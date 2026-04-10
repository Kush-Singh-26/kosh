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
func (handler *consoleHandler) Enabled(workingContext context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

// Handle formats and writes a log record.
func (handler *consoleHandler) Handle(workingContext context.Context, record slog.Record) error {
	if handler.reporter != nil {
		isHTTP := false
		// Only handle Warn, Error, and important Info (like HTTP requests)
		if record.Level < slog.LevelWarn {
			// Special handling for HTTP logs
			record.Attrs(func(attribute slog.Attr) bool {
				if attribute.Key == "http" {
					if attribute.Value.Kind() == slog.KindBool && attribute.Value.Bool() {
						isHTTP = true
					}
					return false
				}
				return true
			})
		}

		var logBuilder strings.Builder
		if isHTTP {
			// Format HTTP log line manually for a clean look in the UI
			var method, path string
			var status int
			var durationStr string
			record.Attrs(func(attribute slog.Attr) bool {
				switch attribute.Key {
				case "method":
					if attribute.Value.Kind() == slog.KindString {
						method = attribute.Value.String()
					}
				case "path":
					if attribute.Value.Kind() == slog.KindString {
						path = attribute.Value.String()
					}
				case "status":
					if attribute.Value.Kind() == slog.KindInt64 {
						status = int(attribute.Value.Int64())
					}
				case "duration":
					if attribute.Value.Kind() == slog.KindDuration {
						durationStr = fmt.Sprintf("%dms", attribute.Value.Duration().Milliseconds())
					} else {
						durationStr = attribute.Value.String()
					}
				}
				return true
			})

			fmt.Fprintf(&logBuilder, "%s %s %d %s", method, path, status, durationStr)
		} else {
			// Not an HTTP log - use standard message formatting
			logBuilder.WriteString(record.Message)
			if record.NumAttrs() > 0 {
				record.Attrs(func(attribute slog.Attr) bool {
					fmt.Fprintf(&logBuilder, " %s=%v", attribute.Key, attribute.Value.Any())
					return true
				})
			}
		}
		message := logBuilder.String()

		switch {
		case record.Level >= slog.LevelError:
			handler.reporter.Error("%s", nil, message)
		case record.Level >= slog.LevelWarn:
			handler.reporter.Warn("%s", message)
		default:
			handler.reporter.Info("%s", message)
		}
		return nil
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()

	// Use fmt.Sprintf to avoid races in fmt package's internal state
	color := getLevelColor(record.Level)
	timeString := record.Time.Format(handler.timeFormat)

	line := fmt.Sprintf("\033[90m%s\033[0m %s ", timeString, color)
	_, _ = fmt.Fprintf(handler.output, "%s", line)
	handler.writeMessage(record.Message)

	// Write handler attributes
	for _, attribute := range handler.attrs {
		handler.writeAttr(attribute)
	}

	// Write record attributes
	if record.NumAttrs() > 0 {
		record.Attrs(func(attribute slog.Attr) bool {
			handler.writeAttr(attribute)
			return true
		})
	}

	_, _ = fmt.Fprintf(handler.output, "\033[0m\n")
	return nil
}

func (handler *consoleHandler) writeMessage(message string) {
	if handler.group != "" {
		_, _ = fmt.Fprintf(handler.output, "[\033[1m%s\033[0m] %s", handler.group, message)
	} else {
		_, _ = fmt.Fprintf(handler.output, "%s", message)
	}
}

func (handler *consoleHandler) writeAttr(attribute slog.Attr) {
	value := attribute.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%s", attribute.Key, value.String())
	case slog.KindInt64:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%d", attribute.Key, value.Int64())
	case slog.KindUint64:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%d", attribute.Key, value.Uint64())
	case slog.KindFloat64:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%.2f", attribute.Key, value.Float64())
	case slog.KindBool:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%v", attribute.Key, value.Bool())
	case slog.KindDuration:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%s", attribute.Key, value.Duration())
	case slog.KindTime:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%s", attribute.Key, value.Time().Format(time.RFC3339))
	case slog.KindGroup:
		attrs := value.Group()
		for _, ga := range attrs {
			handler.writeAttr(ga)
		}
	default:
		_, _ = fmt.Fprintf(handler.output, " \033[36m%s\033[0m=%v", attribute.Key, value)
	}
}

// WithAttrs returns a handler with additional attributes.
func (handler *consoleHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	newAttributes := make([]slog.Attr, len(handler.attrs)+len(attributes))
	copy(newAttributes, handler.attrs)
	copy(newAttributes[len(handler.attrs):], attributes)
	return &consoleHandler{
		output:     handler.output,
		timeFormat: handler.timeFormat,
		attrs:      newAttributes,
		group:      handler.group,
		reporter:   handler.reporter,
	}
}

// WithGroup returns a handler with the given group name.
func (handler *consoleHandler) WithGroup(name string) slog.Handler {
	return &consoleHandler{
		output:     handler.output,
		timeFormat: handler.timeFormat,
		attrs:      handler.attrs,
		group:      name,
		reporter:   handler.reporter,
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
	var reporter ui.Reporter
	if len(reporters) > 0 {
		reporter = reporters[0]
	}
	return slog.New(NewConsoleHandler(os.Stdout, reporter))
}
