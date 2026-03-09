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

func dim(text string) string     { return colorize("\x1b[38;5;244m", text) }
func accent(text string) string  { return colorize("\x1b[38;5;214m", text) }
func bright(text string) string  { return colorize("\x1b[38;5;221;1m", text) }
func warmRed(text string) string { return colorize("\x1b[38;5;209m", text) }

func separatorLine() string {
	return dim("────────────────────────────────────────")
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

func pill(label, value string) string {
	if value == "" || value == "-" {
		return ""
	}
	return fmt.Sprintf("%s %s", accent("["+label+"]"), dim(value))
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
		" _  __         _     ",
		"| |/ /___  ___| |__  ",
		"| ' // _ \\/ __| '_ \\",
		"| . \\ (_) \\__ \\ | | |",
		"|_|\\_\\___/|___/_| |_|",
	}
	if wide {
		for _, line := range logo {
			fmt.Println(accent(line))
		}
	} else {
		fmt.Println(bright("Kosh " + cliVersion))
	}

	fmt.Println(separatorLine())
	modeStyler := modeColor(mode)
	fmt.Printf("%s  %s\n", bright("Kosh "+cliVersion), modeStyler(mode))

	if cfg == nil {
		fmt.Println(dim(`Use "kosh [command] --help" for command details.`))
		fmt.Println(separatorLine())
		fmt.Println()
		return
	}

	meta := []string{}
	if cfg.BaseURL != "" {
		meta = append(meta, pill("Base URL", cfg.BaseURL))
	}
	if cfg.Theme != "" {
		meta = append(meta, pill("Theme", cfg.Theme))
	}
	if cfg.OutputDir != "" {
		meta = append(meta, pill("Output", shortenPath(cfg.OutputDir)))
	}
	if len(meta) > 0 {
		fmt.Println(strings.Join(meta, "  "))
	}
	if cfg.BaseURL == "" && cfg.Theme == "" && cfg.OutputDir == "" {
		fmt.Println(dim("Configuration will be loaded from kosh.yaml"))
	}
	fmt.Println(separatorLine())
	fmt.Println()
}
