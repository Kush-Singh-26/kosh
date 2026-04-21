package main

import (
	"log/slog"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	versionInfo bool
)

const cliVersion = "v2.0.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show Kosh version information",
	Long:  `Show current Kosh version and build information.`,
	Run:   runVersion,
}

func init() {
	rootCmd.AddCommand(versionCmd)

	versionCmd.Flags().BoolVar(&versionInfo, "info", false, "Show Kosh build information")
}

func runVersion(_ *cobra.Command, _ []string) {
	printVersionInfo()
}

func printVersionInfo() {
	slog.Info("Kosh Static Site Generator")
	slog.Info("Version", "version", cliVersion)
	slog.Info("Go version", "go", runtime.Version())
	slog.Info("Build date", "date", "2026-04-21")
	slog.Info("Optimized with:",
		"features", "XXH3 hashing, Incremental single-post rebuilds, "+
			"Atomic clean-build publish, CSS/JS fingerprinting, "+
			"WebP conversion, Go+WASM search")
}
