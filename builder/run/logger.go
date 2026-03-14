package run

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

func DevLog(message string) {
	now := time.Now().Format(devTimeFormat)
	fmt.Fprintf(os.Stdout, "%s%s %s⚡%s %s%s\n", dim, now, cyan, reset, message, reset)
}

func DevLogChange(path, changeType string) {
	now := time.Now().Format(devTimeFormat)
	var color string
	switch changeType {
	case "css", "asset":
		color = cyan
	case "content":
		color = yellow
	case "full", "new", "frontmatter":
		color = green
	case "delete":
		color = red
	default:
		color = cyan
	}
	fmt.Fprintf(os.Stdout, "%s%s %s⚡%s %s %s(%s)%s\n", dim, now, color, reset, path, dim, changeType, reset)
}

func DevLogRebuild(action string) {
	now := time.Now().Format(devTimeFormat)
	fmt.Fprintf(os.Stdout, "%s%s %s📦%s %s%s\n", dim, now, green, reset, action, reset)
}

func DevLogSuccess(message string) {
	now := time.Now().Format(devTimeFormat)
	fmt.Fprintf(os.Stdout, "%s%s %s✓%s %s%s\n", dim, now, green, reset, message, reset)
}

func DevLogSkip(message string) {
	now := time.Now().Format(devTimeFormat)
	fmt.Fprintf(os.Stdout, "%s%s %s◯%s %s%s\n", dim, now, dim, reset, message, reset)
}

func DevLogInfo(message string) {
	now := time.Now().Format(devTimeFormat)
	fmt.Fprintf(os.Stdout, "%s%s %sℹ%s  %s%s\n", dim, now, cyan, reset, message, reset)
}

func DevLogError(message string) {
	now := time.Now().Format(devTimeFormat)
	fmt.Fprintf(os.Stdout, "%s%s %s✗%s %s%s\n", dim, now, red, reset, message, reset)
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
	fmt.Fprintf(os.Stdout, "%s%s %s%s %s %s%d%s %s%dms%s\n",
		dim, now, bold, method, path, statusColor, status, reset, dim, duration.Milliseconds(), reset)
}

type consoleHandler struct {
	output     io.Writer
	mu         sync.Mutex
	timeFormat string
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

	fmt.Fprintf(h.output, "\033[90m%s\033[0m %s ", timeStr, color)
	h.writeMessage(r.Message)

	if r.NumAttrs() > 0 {
		r.Attrs(func(a slog.Attr) bool {
			h.writeAttr(a)
			return true
		})
	}

	fmt.Fprintf(h.output, "\033[0m\n")
	return nil
}

func (h *consoleHandler) writeMessage(msg string) {
	fmt.Fprintf(h.output, "%s", msg)
}

func (h *consoleHandler) writeAttr(a slog.Attr) {
	value := a.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%s", a.Key, value.String())
	case slog.KindInt64:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%d", a.Key, value.Int64())
	case slog.KindUint64:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%d", a.Key, value.Uint64())
	case slog.KindFloat64:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%.2f", a.Key, value.Float64())
	case slog.KindBool:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%v", a.Key, value.Bool())
	case slog.KindDuration:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%s", a.Key, value.Duration())
	case slog.KindTime:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%s", a.Key, value.Time().Format(time.RFC3339))
	default:
		fmt.Fprintf(h.output, " \033[36m%s\033[0m=%v", a.Key, value)
	}
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	return h
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
		return "\033[96mINF\033[0m"
	}
}

func getLevelString(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "DBG"
	case slog.LevelInfo:
		return "INF"
	case slog.LevelWarn:
		return "WRN"
	case slog.LevelError:
		return "ERR"
	default:
		return "INF"
	}
}

func InitLogger() *slog.Logger {
	logger := slog.New(NewConsoleHandler(os.Stdout))
	slog.SetDefault(logger)
	return logger
}
