package main

import (
	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/internal/clean"
)

var (
	cleanCache bool
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
}

func runClean(cmd *cobra.Command, args []string) {
	mode := "Warm Rebuild"
	if cleanCache {
		mode = "Cold Rebuild"
	}
	printStartupBanner(mode, config.Load([]string{}))
	clean.Run(cleanCache)

	orchestration.DevLogInfo("Rebuilding site...")
	if err := orchestration.Run([]string{}); err != nil {
		orchestration.DevLogError("Rebuild failed: " + err.Error())
	}
}
