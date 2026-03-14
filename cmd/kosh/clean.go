package main

import (
	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/internal/clean"
)

var (
	cleanCache bool
	cleanAll   bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean output and rebuild",
	Long: `Clean the output directory and immediately rebuild the site.

Use --cache to also remove .kosh-cache and force a true cold rebuild.
Use --all to clean all versioned output folders instead of preserving configured versions.`,
	Run: runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)

	cleanCmd.Flags().BoolVar(&cleanCache, "cache", false, "Also clean .kosh-cache directory")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "Clean all versions including versioned folders")
}

func runClean(cmd *cobra.Command, args []string) {
	mode := "Warm Rebuild"
	if cleanCache {
		mode = "Cold Rebuild"
	}
	printStartupBanner(mode, config.Load([]string{}))
	clean.Run(cleanCache, cleanAll)

	run.DevLogInfo("Rebuilding site...")
	if err := run.Run([]string{}); err != nil {
		run.DevLogError("Rebuild failed: " + err.Error())
	}
}
