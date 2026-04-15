package main

import (
	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/internal/new"
)

var newCmd = &cobra.Command{
	Use:   "new <title>",
	Short: "Create a new content file",
	Long:  `Create a new markdown content file with the given title. The file will be created in the configured content directory.`,
	Args:  cobra.MinimumNArgs(1),
	Run:   runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(_ *cobra.Command, args []string) {
	new.Run(args)
}
