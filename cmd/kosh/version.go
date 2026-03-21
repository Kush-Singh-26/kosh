package main

import (
	"log/slog"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/internal/version"
)

var (
	versionInfo bool
)

const cliVersion = "v1.4.0"

var versionCmd = &cobra.Command{
	Use:   "version [vX.X]",
	Short: "Version management commands",
	Long: `Show current documentation version info or create a new version.

Examples:
  kosh version           Show current version info
  kosh version v4.0      Freeze current latest and start new version v4.0
  kosh version --info    Show Kosh build information`,
	Run: runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolVar(&versionInfo, "info", false, "Show Kosh build information")
}

func runVersion(cmd *cobra.Command, args []string) {
	if versionInfo {
		printVersionInfo()
		return
	}

	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		version.Run(args)
	} else {
		version.Run([]string{})
	}
}

func printVersionInfo() {
	slog.Info("Kosh Static Site Generator")
	slog.Info("Version", "version", cliVersion)
	slog.Info("Go version", "go", runtime.Version())
	slog.Info("Build date", "date", "2026-03-21")
	slog.Info("Optimized with:",
		"features", "XXH3 hashing, Incremental single-post rebuilds, "+
			"Atomic clean-build publish, CSS/JS fingerprinting, "+
			"WebP conversion, Go+WASM search")
}
