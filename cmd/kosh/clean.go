package main

import (
	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/internal/clean"
)

var (
	cleanCache     bool
	cleanNoStaging bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean output and rebuild",
	Long: `Clean the output directory and immediately rebuild the site.

Use --cache to also remove .kosh-cache and force a true cold rebuild.`,
	Run: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)

	cleanCmd.Flags().BoolVar(&cleanCache, "cache", false, "Also clean .kosh-cache directory")
	cleanCmd.Flags().BoolVar(&cleanNoStaging, "no-staging", false, "Disable atomic staging (overwrites output in place)")
}

func runClean(_ *cobra.Command, _ []string) {
	mode := "Warm Rebuild"
	if cleanCache {
		mode = "Cold Rebuild"
	}
	var filteredArgs []string
	if cleanNoStaging {
		filteredArgs = append(filteredArgs, "-no-staging")
	}
	if debug {
		filteredArgs = append(filteredArgs, "-debug")
	}

	cfg := config.Load(filteredArgs)
	printStartupBanner(mode, cfg)

	if err := clean.RunWithConfig(cfg, cleanCache); err != nil {
		orchestration.DevLogError("Clean failed: " + err.Error())
	}

	orchestration.DevLogInfo("Rebuilding site...")
	if err := orchestration.Run(filteredArgs, reporter); err != nil {
		orchestration.DevLogError("Rebuild failed: " + err.Error())
	}
}
