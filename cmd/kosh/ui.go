package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/config"
)

// Plasma gradient: Deep orange -> Bright Orange -> Yellow Core -> Deep orange
var koshGradient = []int{
	130, 166, 202, 208, 214, 220, 226, 220, 214, 208, 202, 166,
}

func supportsANSI() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "xterm") || strings.Contains(term, "ansi") || strings.Contains(term, "color") || strings.Contains(term, "cygwin") || strings.Contains(term, "msys") {
		return true
	}
	if runtime.GOOS == "windows" {
		if os.Getenv("WT_SESSION") != "" || os.Getenv("ANSICON") != "" || os.Getenv("ConEmuANSI") == "ON" || os.Getenv("TERM_PROGRAM") != "" {
			return true
		}
	}
	return false
}

func colorize(code, text string) string {
	if !supportsANSI() {
		return text
	}
	return code + text + "\x1b[0m"
}

func gradient(text string, colors []int) string {
	if !supportsANSI() {
		return text
	}
	runes := []rune(text)
	n := len(runes)
	if n == 0 {
		return ""
	}
	if n == 1 {
		return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", colors[0], text)
	}

	var b strings.Builder
	for i, r := range runes {
		pos := float64(i) / float64(n-1)
		idx := int(pos * float64(len(colors)-1))
		color := colors[idx]
		fmt.Fprintf(&b, "\x1b[38;5;%dm%c", color, r)
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

// Color definitions
func dim(text string) string     { return colorize("\x1b[38;5;244m", text) }
func accent(text string) string  { return colorize("\x1b[38;5;214m", text) }
func bright(text string) string  { return colorize("\x1b[38;5;221;1m", text) }
func warmRed(text string) string { return colorize("\x1b[38;5;209m", text) }

func statusDot() string {
	return colorize("\x1b[38;5;196m", "●")
}

func modeColor(mode string) func(string) string {
	upper := strings.ToUpper(mode)
	switch {
	case strings.Contains(upper, "DEV"):
		return bright
	case strings.Contains(upper, "CLEAN"):
		return warmRed
	case strings.Contains(upper, "HELP"):
		return accent
	default:
		return bright
	}
}

// Helper for aligned blockquote rows
func infoRow(label, value string) string {
	if value == "" || value == "-" {
		return ""
	}
	// Pad the label to 10 characters for perfect vertical alignment
	padLen := 10 - len(label)
	if padLen < 0 {
		padLen = 1
	}
	padding := strings.Repeat(" ", padLen)
	return fmt.Sprintf("     %s %s%s %s", dim("│"), accent(label), padding, value)
}

func shortenPath(path string) string {
	if path == "" {
		return "-"
	}
	cwd, err := os.Getwd()
	if err == nil {
		if rel, err := filepath.Rel(cwd, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func printStartupBanner(mode string, cfg *config.Config) {
	wide := true
	if width := os.Getenv("COLUMNS"); width != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(width)); err == nil && n > 0 && n <= 70 {
			wide = false
		}
	}

	fmt.Println()
	logo := []string{
		"██╗  ██╗ ██████╗ ███████╗██╗  ██╗",
		"██║ ██╔╝██╔═══██╗██╔════╝██║  ██║",
		"█████╔╝ ██║   ██║███████╗███████║",
		"██╔═██╗ ██║   ██║╚════██║██╔══██║",
		"██║  ██╗╚██████╔╝███████║██║  ██║",
		"╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═╝",
	}

	if wide {
		for i, line := range logo {
			// Using a heavy block character for the left anchor
			colorIdx := int((float64(i) / float64(len(logo)-1)) * float64(len(koshGradient)-1))
			dec := fmt.Sprintf("\x1b[38;5;%dm█\x1b[0m", koshGradient[colorIdx])

			if !supportsANSI() {
				dec = "█"
			}

			fmt.Printf("  %s  %s\n", dec, gradient(line, koshGradient))
		}
	} else {
		fmt.Println("  " + gradient("Kosh "+cliVersion, koshGradient))
	}

	fmt.Println()

	// Tagline with system footprint
	modeStyler := modeColor(mode)
	sysBadge := dim(fmt.Sprintf("(%s/%s)", runtime.GOOS, runtime.GOARCH))
	fmt.Printf("     %s  %s %s   %s\n", gradient("Kosh "+cliVersion, koshGradient), modeStyler(mode), statusDot(), sysBadge)
	fmt.Println()

	// Empty config state
	if cfg == nil {
		fmt.Println("     " + dim("│ ") + dim(`Use "kosh [command] --help" for command details.`))
		fmt.Println("     " + dim("╰"+strings.Repeat("─", 30)))
		fmt.Println()
		return
	}

	// Populated config state - Blockquote UI
	if cfg.BaseURL == "" && cfg.Theme == "" && cfg.OutputDir == "" {
		fmt.Println("     " + dim("│ ") + dim("Configuration will be loaded from kosh.yaml"))
	} else {
		if cfg.BaseURL != "" {
			fmt.Println(infoRow("Base URL", cfg.BaseURL))
		}
		if cfg.Theme != "" {
			fmt.Println(infoRow("Theme", cfg.Theme))
		}
		if cfg.OutputDir != "" {
			fmt.Println(infoRow("Output", shortenPath(cfg.OutputDir)))
		}
	}

	// Cap off the blockquote UI beautifully
	fmt.Println("     " + dim("╰"+strings.Repeat("─", 30)))
	fmt.Println()
}
