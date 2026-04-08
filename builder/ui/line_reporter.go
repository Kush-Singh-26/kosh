package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

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

type lineReporter struct {
	mode    string
	mu      sync.Mutex
	verbose bool
	isTTY   bool

	// Active spinner state
	phases     map[Phase]*linePhaseState
	phaseOrder []Phase
	ticker     *time.Ticker
	done       chan struct{}
	status     string
	finished   bool
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
		phases:  make(map[Phase]*linePhaseState),
		verbose: verbose,
		isTTY:   detectTTY() && !verbose,
		done:    make(chan struct{}),
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

func (r *lineReporter) color(code, text string) string {
	if !r.isTTY {
		return text
	}
	return code + text + reset
}

// ts returns the current time formatted for log line prefixes.
func (r *lineReporter) ts() string {
	return r.color(gray, time.Now().Format("15:04:05"))
}

func (r *lineReporter) Start(mode string) {
	r.mode = mode
	if !r.isTTY {
		return
	}

	// Start a background goroutine to animate the spinner at 100ms intervals
	r.ticker = time.NewTicker(100 * time.Millisecond)
	go func() {
		for {
			select {
			case <-r.ticker.C:
				r.renderSpinner()
			case <-r.done:
				return
			}
		}
	}()
}

func (r *lineReporter) StartPhase(phase Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.phases[phase]; !ok {
		r.phaseOrder = append(r.phaseOrder, phase)
	}
	r.phases[phase] = &linePhaseState{
		startTime: time.Now(),
	}

	if !r.isTTY {
		fmt.Printf("%s %s %s\n", r.ts(), r.color(cyan, symArrow), phase.String())
	}
}

func (r *lineReporter) UpdateProgress(phase Phase, current, total int, detail string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ps, ok := r.phases[phase]; ok {
		ps.current = current
		ps.total = total
		ps.detail = detail
	}
}

func (r *lineReporter) EndPhase(phase Phase, duration time.Duration) {
	r.mu.Lock()
	ps, ok := r.phases[phase]
	if !ok {
		r.mu.Unlock()
		return
	}

	if duration == 0 {
		duration = time.Since(ps.startTime).Round(time.Millisecond)
	}
	ps.duration = duration
	ps.finished = true
	r.mu.Unlock()

	durationStr := formatDuration(duration)
	line := fmt.Sprintf("%s %s %-22s %s\n",
		r.ts(),
		r.color(green, symCheck),
		phase.String(),
		r.color(gray, durationStr))

	r.printLine(line)
}

func (r *lineReporter) Info(msg string, args ...any) {
	content := fmt.Sprintf(msg, args...)
	if r.shouldSkip(content) {
		return
	}
	content = r.shortenPaths(content)
	line := fmt.Sprintf("%s %s %s\n", r.ts(), r.color(cyan, symInfo), content)

	r.printLine(line)
}

func (r *lineReporter) Warn(msg string, args ...any) {
	content := fmt.Sprintf(msg, args...)
	content = r.shortenPaths(content)
	line := fmt.Sprintf("%s %s %s\n", r.ts(), r.color(yellow, symWarn), content)

	r.printLine(line)
}

func (r *lineReporter) Error(msg string, err error, args ...any) {
	content := fmt.Sprintf(msg, args...)
	if err != nil {
		content = fmt.Sprintf("%s: %v", content, err)
	}
	content = r.shortenPaths(content)
	line := fmt.Sprintf("%s %s %s\n", r.ts(), r.color(red, symCross), content)

	r.printLine(line)
}

func (r *lineReporter) Success(msg string) {
	content := r.shortenPaths(msg)
	line := fmt.Sprintf("%s %s %s\n", r.ts(), r.color(green, symCheck), content)

	r.printLine(line)
}

func (r *lineReporter) Status(msg string) {
	r.mu.Lock()
	r.status = msg
	r.mu.Unlock()

	line := fmt.Sprintf("\n%s %s %s\n", r.ts(), r.color(cyan, symReady), r.color(bold, msg))
	r.printLine(line)
}

// printLine outputs a line, clearing the spinner first if active.
func (r *lineReporter) printLine(line string) {
	r.mu.Lock()
	finished := r.finished
	isTTY := r.isTTY
	r.mu.Unlock()

	if isTTY && !finished {
		fmt.Fprint(os.Stdout, "\r\033[K"+line)
	} else {
		fmt.Fprint(os.Stdout, line)
	}
}

func (r *lineReporter) Finish(stats BuildStats) {
	r.mu.Lock()
	r.finished = true

	if r.ticker != nil {
		r.ticker.Stop()
	}
	select {
	case <-r.done:
	default:
		close(r.done)
	}

	if r.isTTY {
		r.clearLine()
	}
	r.mu.Unlock()

	// Build summary
	fmt.Println()
	durationStr := formatDuration(stats.Duration)
	fmt.Printf("%s %s %s in %s\n",
		r.ts(),
		r.color(green+bold, symCheck),
		r.color(green+bold, "Build Complete"),
		r.color(bold, durationStr))
	fmt.Println()

	// Stats table
	cacheStr := fmt.Sprintf("%.0f%%", stats.HitRate*100)
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
			r.color(gray, symDot),
			r.color(cyan, l.label),
			l.value)
	}
	fmt.Println()
}

// renderSpinner draws the current active phase on a single line using \r.
// Must be called with r.mu held.
func (r *lineReporter) renderSpinner() {
	r.mu.Lock()
	finished := r.finished

	// Find the topmost active (unfinished) phase
	var activePhase Phase
	var activeState *linePhaseState
	for _, p := range r.phaseOrder {
		ps := r.phases[p]
		if ps != nil && !ps.finished {
			activePhase = p
			activeState = ps
			break
		}
	}

	if activeState == nil || finished {
		r.mu.Unlock()
		return
	}

	// Calculate spinner frame
	elapsed := time.Since(activeState.startTime)
	frameIdx := (elapsed.Milliseconds() / 100) % int64(len(spinnerFrames))
	frame := spinnerFrames[frameIdx]

	ts := r.color(gray, time.Now().Format("15:04:05"))
	var line string
	if activeState.total > 0 {
		line = fmt.Sprintf("\r%s %s %-22s [%d/%d]",
			ts,
			r.color(cyan, frame),
			activePhase.String(),
			activeState.current,
			activeState.total)
		if activeState.detail != "" {
			detail := r.shortenPaths(activeState.detail)
			if len(detail) > 40 {
				detail = "..." + detail[len(detail)-37:]
			}
			line += " " + r.color(gray, detail)
		}
	} else {
		line = fmt.Sprintf("\r%s %s %s",
			ts,
			r.color(cyan, frame),
			activePhase.String())
		if activeState.detail != "" {
			detail := r.shortenPaths(activeState.detail)
			if len(detail) > 40 {
				detail = "..." + detail[len(detail)-37:]
			}
			line += " " + r.color(gray, detail)
		}
	}
	r.mu.Unlock()

	// Use erase-to-end-of-line to clear any leftover characters
	fmt.Fprint(os.Stdout, line+"\033[K")
}

// clearLine clears the current spinner line. Must be called with r.mu held.
func (r *lineReporter) clearLine() {
	fmt.Fprint(os.Stdout, "\r\033[K")
}

func (r *lineReporter) shouldSkip(msg string) bool {
	// In verbose mode, don't skip anything
	if r.verbose {
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
		"robots.txt",
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

func (r *lineReporter) shortenPaths(content string) string {
	root := fs.RepoRoot()
	if root != "." {
		content = strings.ReplaceAll(content, root, ".")
	}
	if wd, err := os.Getwd(); err == nil && wd != root {
		content = strings.ReplaceAll(content, wd, ".")
	}
	return content
}

func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
