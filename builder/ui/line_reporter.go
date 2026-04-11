package ui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/fs"
)

// ANSI color helpers — no external dependencies.
const (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[91m"
	green   = "\033[92m"
	yellow  = "\033[93m"
	cyan    = "\033[96m"
	gray    = "\033[90m"
	magenta = "\033[95m"
)

// Text-based log level labels
const (
	symCheck = "OK "
	symCross = "ERR"
	symWarn  = "WRN"
	symInfo  = "INF"
	symArrow = ">>>"
	symReady = "RDY"
	symDot   = " - "
)

var spinnerFrames = []string{"-", "\\", "|", "/"}

const (
	spinnerInterval        = 100 * time.Millisecond
	percentScale           = 100
	spinnerFrameIntervalMs = 100
	detailMaxLen           = 40
	detailTailLen          = 37
	secondsPerMinute       = 60
	bytesPerKiB            = 1024
)

type lineReporter struct {
	mode      string
	mu        sync.Mutex // protects phases, phaseOrder, status, isFinished
	isVerbose bool
	isTTY     bool

	// Active spinner state
	phases     map[Phase]*linePhaseState
	phaseOrder []Phase
	ticker     *time.Ticker
	done       chan struct{}
	status     string
	isFinished bool
}

type linePhaseState struct {
	current   int
	total     int
	detail    string
	startTime time.Time
	duration  time.Duration
	finished  bool
}

// NewReporter returns a Reporter that renders a single-line UI.
func NewReporter(verbose bool) Reporter {
	return &lineReporter{
		phases:    make(map[Phase]*linePhaseState),
		isVerbose: verbose,
		isTTY:     detectTTY() && !verbose,
		done:      make(chan struct{}),
	}
}

func detectTTY() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// Check for known terminal emulators on Windows
	if runtime.GOOS == "windows" {
		if os.Getenv("WT_SESSION") != "" || os.Getenv("ANSICON") != "" ||
			os.Getenv("ConEmuANSI") == "ON" || os.Getenv("TERM_PROGRAM") != "" {
			return true
		}
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "xterm") || strings.Contains(term, "ansi") ||
		strings.Contains(term, "color") || strings.Contains(term, "cygwin") ||
		strings.Contains(term, "msys") {
		return true
	}
	// Fallback: check if stdout is a terminal
	if fi, err := os.Stdout.Stat(); err == nil {
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

func (rep *lineReporter) color(code, text string) string {
	if !rep.isTTY {
		return text
	}
	return code + text + reset
}

// ts returns the current time formatted for log line prefixes.
func (rep *lineReporter) ts() string {
	return rep.color(gray, time.Now().Format("15:04:05"))
}

// Start initializes the reporter with the given mode.
func (rep *lineReporter) Start(mode string) {
	rep.mode = mode
	if !rep.isTTY {
		return
	}

	// Start a background goroutine to animate the spinner at 100ms intervals
	rep.ticker = time.NewTicker(spinnerInterval)
	async.FireAndForget(context.Background(), slog.Default(), "line reporter spinner", func() error {
		for {
			select {
			case <-rep.ticker.C:
				rep.renderSpinner()
			case <-rep.done:
				return nil
			}
		}
	})
}

// StartPhase marks a build phase as started.
func (rep *lineReporter) StartPhase(phase Phase) {
	rep.mu.Lock()
	defer rep.mu.Unlock()

	if _, ok := rep.phases[phase]; !ok {
		rep.phaseOrder = append(rep.phaseOrder, phase)
	}
	rep.phases[phase] = &linePhaseState{
		startTime: time.Now(),
	}

	if !rep.isTTY {
		fmt.Printf("%s %s %s\n", rep.ts(), rep.color(cyan, symArrow), phase.String())
	}
}

// UpdateProgress updates progress numbers for a phase.
func (rep *lineReporter) UpdateProgress(phase Phase, current, total int, detail string) {
	rep.mu.Lock()
	defer rep.mu.Unlock()

	if ps, ok := rep.phases[phase]; ok {
		ps.current = current
		ps.total = total
		ps.detail = detail
	}
}

// EndPhase marks a phase as completed.
func (rep *lineReporter) EndPhase(phase Phase, duration time.Duration) {
	rep.mu.Lock()
	ps, ok := rep.phases[phase]
	if !ok {
		rep.mu.Unlock()
		return
	}

	if duration == 0 {
		duration = time.Since(ps.startTime).Round(time.Millisecond)
	}
	ps.duration = duration
	ps.finished = true
	rep.mu.Unlock()

	durationStr := formatDuration(duration)
	line := fmt.Sprintf("%s %s %-22s %s\n",
		rep.ts(),
		rep.color(green, symCheck),
		phase.String(),
		rep.color(gray, durationStr))

	rep.printLine(line)
}

// Info logs an informational message.
func (rep *lineReporter) Info(msg string, args ...any) {
	content := fmt.Sprintf(msg, args...)
	if rep.shouldSkip(content) {
		return
	}
	content = rep.shortenPaths(content)
	line := fmt.Sprintf("%s %s %s\n", rep.ts(), rep.color(cyan, symInfo), content)

	rep.printLine(line)
}

// Warn logs a warning message.
func (rep *lineReporter) Warn(msg string, args ...any) {
	content := fmt.Sprintf(msg, args...)
	content = rep.shortenPaths(content)
	line := fmt.Sprintf("%s %s %s\n", rep.ts(), rep.color(yellow, symWarn), content)

	rep.printLine(line)
}

// Error logs an error message with an optional error value.
func (rep *lineReporter) Error(msg string, err error, args ...any) {
	content := fmt.Sprintf(msg, args...)
	if err != nil {
		content = fmt.Sprintf("%s: %v", content, err)
	}
	content = rep.shortenPaths(content)
	line := fmt.Sprintf("%s %s %s\n", rep.ts(), rep.color(red, symCross), content)

	rep.printLine(line)
}

// Success logs a success message.
func (rep *lineReporter) Success(msg string) {
	content := rep.shortenPaths(msg)
	line := fmt.Sprintf("%s %s %s\n", rep.ts(), rep.color(green, symCheck), content)

	rep.printLine(line)
}

// Status prints a status line.
func (rep *lineReporter) Status(msg string) {
	rep.mu.Lock()
	rep.status = msg
	rep.mu.Unlock()

	line := fmt.Sprintf("\n%s %s %s\n", rep.ts(), rep.color(cyan, symReady), rep.color(bold, msg))
	rep.printLine(line)
}

// printLine outputs a line, clearing the spinner first if active.
func (rep *lineReporter) printLine(line string) {
	rep.mu.Lock()
	finished := rep.isFinished
	isTTY := rep.isTTY
	rep.mu.Unlock()

	if isTTY && !finished {
		fmt.Fprint(os.Stdout, "\r\033[K"+line)
	} else {
		fmt.Fprint(os.Stdout, line)
	}
}

// Finish renders the final build summary.
func (rep *lineReporter) Finish(stats BuildStats) {
	rep.mu.Lock()
	rep.isFinished = true

	if rep.ticker != nil {
		rep.ticker.Stop()
	}
	select {
	case <-rep.done:
	default:
		close(rep.done)
	}

	if rep.isTTY {
		rep.clearLine()
	}
	rep.mu.Unlock()

	// Build summary
	fmt.Println()
	durationStr := formatDuration(stats.Duration)
	fmt.Printf("%s %s %s in %s\n",
		rep.ts(),
		rep.color(green+bold, symCheck),
		rep.color(green+bold, "Build Complete"),
		rep.color(bold, durationStr))
	fmt.Println()

	// Stats table
	cacheStr := fmt.Sprintf("%.0f%%", stats.HitRate*percentScale)
	statLines := []struct {
		label, value string
	}{
		{"Posts", fmt.Sprintf("%d rendered", stats.Posts)},
		{"Assets", fmt.Sprintf("%d processed", stats.Assets)},
		{"Cache", fmt.Sprintf("%s hit rate", cacheStr)},
	}
	if stats.Optimized > 0 {
		statLines = append(statLines, struct{ label, value string }{
			"Images", fmt.Sprintf("%d optimized (saved %s)", stats.Optimized, formatBytes(stats.SavedBytes)),
		})
	}

	for _, l := range statLines {
		fmt.Printf("         %s %-8s %s\n",
			rep.color(gray, symDot),
			rep.color(cyan, l.label),
			l.value)
	}
	fmt.Println()
}

// renderSpinner draws the current active phase on a single line using \r.
// Must be called with rep.mu held.
func (rep *lineReporter) renderSpinner() {
	rep.mu.Lock()
	finished := rep.isFinished

	// Find the topmost active (unfinished) phase
	var activePhase Phase
	var activeState *linePhaseState
	for _, p := range rep.phaseOrder {
		ps := rep.phases[p]
		if ps != nil && !ps.finished {
			activePhase = p
			activeState = ps
			break
		}
	}

	if activeState == nil || finished {
		rep.mu.Unlock()
		return
	}

	// Calculate spinner frame
	elapsed := time.Since(activeState.startTime)
	frameIdx := (elapsed.Milliseconds() / spinnerFrameIntervalMs) % int64(len(spinnerFrames))
	frame := spinnerFrames[frameIdx]

	ts := rep.color(gray, time.Now().Format("15:04:05"))
	var line string
	if activeState.total > 0 {
		line = fmt.Sprintf("\r%s %s %-22s [%d/%d]",
			ts,
			rep.color(cyan, frame),
			activePhase.String(),
			activeState.current,
			activeState.total)
		if activeState.detail != "" {
			detail := rep.shortenPaths(activeState.detail)
			if len(detail) > detailMaxLen {
				detail = "..." + detail[len(detail)-detailTailLen:]
			}
			line += " " + rep.color(gray, detail)
		}
	} else {
		line = fmt.Sprintf("\r%s %s %s",
			ts,
			rep.color(cyan, frame),
			activePhase.String())
		if activeState.detail != "" {
			detail := rep.shortenPaths(activeState.detail)
			if len(detail) > detailMaxLen {
				detail = "..." + detail[len(detail)-detailTailLen:]
			}
			line += " " + rep.color(gray, detail)
		}
	}
	rep.mu.Unlock()

	// Use erase-to-end-of-line to clear any leftover characters
	fmt.Fprint(os.Stdout, line+"\033[K")
}

// clearLine clears the current spinner line. Must be called with rep.mu held.
func (rep *lineReporter) clearLine() {
	fmt.Fprint(os.Stdout, "\r\033[K")
}

func (rep *lineReporter) shouldSkip(msg string) bool {
	// In verbose mode, don't skip anything
	if rep.isVerbose {
		return false
	}

	lower := strings.ToLower(msg)
	skipPatterns := []string{
		"building assets",
		"rendering",
		"scanning",
		"processing posts",
		"publishing output",
		"build complete",
		"saved caches",
		"metadata scan",
		"site-wide",
		"pagination",
		"rendering graph",
		"sitemap",
		"pwa",
		"rss",
		"atom",
		"search index",
		"social cards",
		"phase completed",
		"generating",
		"syncing",
		"initializing",
		"bytecode",
		"auto-reload",
		"serving on",
		"build health",
		"graph data unchanged",
		"renderer pool",
		"cache garbage collection",
		"triggering scheduled",
		"nativerenderer",
		"js renderer",
		"graph page rendered",
		"using cached",
		"cached search",
		"search wasm",
		"publishing",
		"build insights",
		"flushing diagram",
		"wasm deployed",
		"build health",
		"build complete",
	}
	for _, p := range skipPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func (rep *lineReporter) shortenPaths(content string) string {
	root := fs.RepoRoot()
	if root != "." {
		content = strings.ReplaceAll(content, root, ".")
	}
	if wd, err := os.Getwd(); err == nil && wd != root {
		content = strings.ReplaceAll(content, wd, ".")
	}
	return content
}

func formatDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return fmt.Sprintf("%dµs", duration.Microseconds())
	}
	if duration < time.Second {
		return fmt.Sprintf("%dms", duration.Milliseconds())
	}
	if duration < time.Minute {
		return fmt.Sprintf("%.1fs", duration.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(duration.Minutes()), int(duration.Seconds())%secondsPerMinute)
}

func formatBytes(bytes int64) string {
	const unit = bytesPerKiB
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
