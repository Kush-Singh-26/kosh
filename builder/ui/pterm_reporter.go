package ui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/pterm/pterm"
)

type phaseState struct {
	current   int
	total     int
	detail    string
	startTime time.Time
	duration  time.Duration
	finished  bool
}

type ptermReporter struct {
	mode         string
	phases       map[Phase]*phaseState
	phaseOrder   []Phase
	status       string
	area         *pterm.AreaPrinter
	lastProgress string
	mu           sync.Mutex
	verbose      bool
	isTTY        bool
	ticker       *time.Ticker
	done         chan struct{}
}

func NewPtermReporter(verbose bool) Reporter {
	return &ptermReporter{
		phases:  make(map[Phase]*phaseState),
		verbose: verbose,
		isTTY:   pterm.PrintColor && !verbose,
		done:    make(chan struct{}),
	}
}

func (r *ptermReporter) Start(mode string) {
	r.mode = mode
	if !r.isTTY {
		return
	}

	area := pterm.DefaultArea
	r.area, _ = area.Start()

	// 100ms refresh for smooth spinners, diffing handles efficiency
	r.ticker = time.NewTicker(100 * time.Millisecond)
	go func() {
		for {
			select {
			case <-r.ticker.C:
				r.mu.Lock()
				r.renderProgress()
				r.mu.Unlock()
			case <-r.done:
				return
			}
		}
	}()
}

func (r *ptermReporter) StartPhase(phase Phase) {
	if !r.isTTY {
		pterm.Info.Printf("[%s] Starting...\n", phase)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.phases[phase]; !ok {
		r.phaseOrder = append(r.phaseOrder, phase)
	}
	r.phases[phase] = &phaseState{
		startTime: time.Now(),
	}
	r.renderProgress()
}

func (r *ptermReporter) UpdateProgress(phase Phase, current, total int, detail string) {
	if !r.isTTY {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if ps, ok := r.phases[phase]; ok {
		ps.current = current
		ps.total = total
		ps.detail = detail
	}
}

func (r *ptermReporter) EndPhase(phase Phase, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ps, ok := r.phases[phase]
	if !ok {
		return
	}

	if duration == 0 {
		duration = time.Since(ps.startTime).Round(time.Millisecond * 100)
	}
	ps.duration = duration
	ps.finished = true

	if !r.isTTY {
		pterm.Success.Printf("[%s] Completed in %s\n", phase, duration)
		return
	}

	// When a phase ends, we "print" it as a static success line above the area
	r.printAbove(pterm.Success.Sprintf("%-20s [%s]", phase, duration.Round(time.Millisecond*100)))
}

func (r *ptermReporter) Info(msg string, args ...any) {
	content := fmt.Sprintf(msg, args...)
	if r.shouldSkip(content) {
		return
	}
	content = r.shortenPaths(content)
	if !r.isTTY {
		pterm.Info.Println(content)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.printAbove(pterm.Info.Sprint(content))
}

func (r *ptermReporter) shouldSkip(msg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// If we aren't in progress-bar mode (e.g. after Finish() or in non-TTY), don't skip anything
	if r.area == nil {
		return false
	}

	msg = strings.ToLower(msg)
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
	}
	for _, p := range skipPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

func (r *ptermReporter) Warn(msg string, args ...any) {
	content := fmt.Sprintf(msg, args...)
	content = r.shortenPaths(content)
	if !r.isTTY {
		pterm.Warning.Println(content)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.printAbove(pterm.Warning.Sprint(content))
}

func (r *ptermReporter) Error(msg string, err error, args ...any) {
	content := fmt.Sprintf(msg, args...)
	if err != nil {
		content = fmt.Sprintf("%s: %v", content, err)
	}
	content = r.shortenPaths(content)
	if !r.isTTY {
		pterm.Error.Println(content)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.printAbove(pterm.Error.Sprint(content))
}

func (r *ptermReporter) Success(msg string) {
	content := r.shortenPaths(msg)
	if !r.isTTY {
		pterm.Success.Println(content)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.printAbove(pterm.Success.Sprint(content))
}

func (r *ptermReporter) Status(msg string) {
	if !r.isTTY {
		pterm.Info.WithPrefix(pterm.Prefix{Text: "READY", Style: pterm.NewStyle(pterm.FgCyan)}).Println(msg)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = msg
	r.renderProgress()
}

func (r *ptermReporter) printAbove(content string) {
	if r.area == nil {
		fmt.Println(content)
		return
	}
	// Clear area, print log, redraw area immediately to sync line count
	r.area.Clear()
	fmt.Println(content)
	r.lastProgress = "" // Force redraw on next frame
	r.renderProgress()
}

func (r *ptermReporter) renderProgress() {
	if r.area == nil {
		return
	}

	var b strings.Builder
	width := pterm.GetTerminalWidth()
	if width <= 0 {
		width = 80
	}

	activeCount := 0
	for _, p := range r.phaseOrder {
		ps := r.phases[p]
		if ps == nil || ps.finished {
			continue
		}
		activeCount++

		if ps.total > 0 {
			percentage := float64(ps.current) / float64(ps.total) * 100
			barWidth := 20
			if width > 60 {
				barWidth = 30
			}
			filled := int(float64(barWidth) * percentage / 100)
			if filled > barWidth {
				filled = barWidth
			}
			bar := strings.Repeat("█", filled) + strings.Repeat(" ", barWidth-filled)
			line := fmt.Sprintf("%-20s [%03d/%03d] %s %3d%%",
				p, ps.current, ps.total, pterm.Cyan(bar), int(percentage))

			if ps.detail != "" {
				detail := r.shortenPaths(ps.detail)
				// Reserve space for the line and some padding
				// Strip ANSI codes to calculate visible length
				visibleLen := len(stripANSI(line))
				maxDetailLen := width - visibleLen - 5
				if maxDetailLen > 10 {
					if len(detail) > maxDetailLen {
						detail = "..." + detail[len(detail)-maxDetailLen+3:]
					}
					line += fmt.Sprintf(" %s", pterm.Gray(detail))
				}
			}
			b.WriteString(line + "\n")
		} else {
			// Simple text for phases without progress
			frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
			frame := frames[(time.Since(ps.startTime).Milliseconds()/100)%int64(len(frames))]
			line := fmt.Sprintf("%s %-20s", pterm.Cyan(frame), p)
			if ps.detail != "" {
				detail := r.shortenPaths(ps.detail)
				visibleLen := len(stripANSI(line))
				maxDetailLen := width - visibleLen - 5
				if maxDetailLen > 10 {
					if len(detail) > maxDetailLen {
						detail = "..." + detail[len(detail)-maxDetailLen+3:]
					}
					line += fmt.Sprintf(" %s", pterm.Gray(detail))
				}
			}
			b.WriteString(line + "\n")
		}
	}

	if r.status != "" {
		if activeCount > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(pterm.Info.WithPrefix(pterm.Prefix{Text: "READY", Style: pterm.NewStyle(pterm.FgCyan)}).Sprint(r.status))
		b.WriteByte('\n')
	}

	newContent := b.String()
	if newContent != r.lastProgress {
		r.area.Update(newContent)
		r.lastProgress = newContent
	}
}

func stripANSI(str string) string {
	const ansi = "[\u001B\u009B][[()#;?]*(?:[0-9]{1,4}(?:;[0-9]{0,4})*)?[0-9A-ORZcf-nqry=><]"
	re := regexp.MustCompile(ansi)
	return re.ReplaceAllString(str, "")
}

func (r *ptermReporter) shortenPaths(content string) string {
	root := fs.RepoRoot()
	if root != "." {
		content = strings.ReplaceAll(content, root, ".")
	}
	if wd, err := os.Getwd(); err == nil && wd != root {
		content = strings.ReplaceAll(content, wd, ".")
	}
	return content
}

func (r *ptermReporter) Finish(duration time.Duration, hitRate float64, posts, assets, optimized int, savedBytes int64) {
	r.mu.Lock()
	if r.ticker != nil {
		r.ticker.Stop()
	}
	select {
	case <-r.done:
	default:
		close(r.done)
	}

	if r.isTTY && r.area != nil {
		r.area.Stop()
		r.area = nil
	}
	r.mu.Unlock()

	fmt.Println()
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgLightBlue)).Println("Build Complete")

	pterm.DefaultSection.Println("Statistics")
	pterm.DefaultBulletList.WithItems([]pterm.BulletListItem{
		{Level: 0, Text: fmt.Sprintf("Duration: %s", duration)},
		{Level: 0, Text: fmt.Sprintf("Cache Hits: %.1f%%", hitRate*100)},
		{Level: 0, Text: fmt.Sprintf("Posts Rendered: %d", posts)},
		{Level: 0, Text: fmt.Sprintf("Assets Processed: %d", assets)},
		{Level: 0, Text: fmt.Sprintf("Images Optimized: %d (saved %s)", optimized, formatBytes(savedBytes))},
	}).Render()
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
