package main

import (
	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/internal/new"
)

var newSection string

var newCmd = &cobra.Command{
	Use:   "new <title>",
	Short: "Create a new content file",
	Long: `Create a new markdown content file with the given title.

The file is created under the configured content prefix (contentPrefix in
kosh.yaml, e.g. "blogs"), so new posts land alongside existing content. Use the
--section flag to override the destination subdirectory.`,
	Args: cobra.MinimumNArgs(1),
	Run:  runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVarP(&newSection, "section", "s", "", "content section subdirectory (overrides contentPrefix)")
}

func runNew(_ *cobra.Command, args []string) {
	new.Run(args, newSection)
}
