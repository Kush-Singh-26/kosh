package orchestration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

const (
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	dim    = "\033[90m"
	reset  = "\033[0m"
	bold   = "\033[1m"
)

var devTimeFormat = "15:04:05"

var rebuildLevel = slog.Level(slog.LevelWarn + 1)

func DevLogChange(path, changeType string) {
	slog.Log(context.Background(), rebuildLevel, "file change", "path", path, "type", changeType)
}

func DevLogRebuild(action string) {
	slog.Log(context.Background(), rebuildLevel, action)
}

func DevLogSuccess(message string) {
	slog.Log(context.Background(), slog.LevelInfo, message)
}

func DevLogSkip(message string) {
	slog.Log(context.Background(), rebuildLevel, message, "skipped", true)
}

func DevLogInfo(message string) {
	slog.Log(context.Background(), slog.LevelInfo, message)
}

func DevLogError(message string) {
	slog.Log(context.Background(), slog.LevelError, message)
}

func HTTPLog(method, path string, status int, duration time.Duration) {
	now := time.Now().Format(devTimeFormat)
	var statusColor string
	switch {
	case status >= 500:
		statusColor = red
	case status >= 400:
		statusColor = yellow
	case status >= 300:
		statusColor = cyan
	default:
		statusColor = green
	}
	_, _ = fmt.Fprintf(os.Stdout, "%s%s %s%s %s %s%d%s %s%dms%s\n",
		dim, now, bold, method, path, statusColor, status, reset, dim, duration.Milliseconds(), reset)
}

type consoleHandler struct {
	output     io.Writer
	mu         sync.Mutex
	timeFormat string
	attrs      []slog.Attr
	group      string
}

func NewConsoleHandler(output io.Writer) *consoleHandler {
	return &consoleHandler{
		output:     output,
		timeFormat: "15:04:05",
	}
}

func (h *consoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *consoleHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	color := getLevelColor(r.Level)

	timeStr := r.Time.Format(h.timeFormat)

	_, _ = fmt.Fprintf(h.output, "\033[90m%s\033[0m %s ", timeStr, color)
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

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &consoleHandler{
		output:     h.output,
		timeFormat: h.timeFormat,
		attrs:      newAttrs,
		group:      h.group,
	}
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	return &consoleHandler{
		output:     h.output,
		timeFormat: h.timeFormat,
		attrs:      h.attrs,
		group:      name,
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

func InitLogger() *slog.Logger {
	return slog.New(NewConsoleHandler(os.Stdout))
}
