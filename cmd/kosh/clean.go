package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/internal/clean"
)

var (
	cleanCache bool
	cleanAll   bool
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Clean output directory",
	Long:  `Clean output directory. Use --cache to also clean the cache, --all to clean all versions.`,
	Run:   runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)

	cleanCmd.Flags().BoolVar(&cleanCache, "cache", false, "Also clean .kosh-cache directory")
	cleanCmd.Flags().BoolVar(&cleanAll, "all", false, "Clean all versions including versioned folders")
}

func runClean(cmd *cobra.Command, args []string) {
	clean.Run(cleanCache, cleanAll)
	fmt.Println("\nRebuilding site...")
	run.Run([]string{})
}
