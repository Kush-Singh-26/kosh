package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/internal/version"
)

var (
	versionInfo bool
)

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
	fmt.Println("Kosh Static Site Generator")
	fmt.Println("Version: v1.3.9")
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Println("Build Date: 2026-03-05")
	fmt.Println("\nOptimized with:")
	fmt.Println("  - BLAKE3 hashing (replaced MD5)")
	fmt.Println("  - Object pooling for memory management")
	fmt.Println("  - Pre-computed search indexes")
	fmt.Println("  - Generic cache operations")
	fmt.Println("  - Content-addressed template cache")
	fmt.Println("  - HTML content deduplication with reference counting")
}
