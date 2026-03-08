package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/internal/new"
)

var newCmd = &cobra.Command{
	Use:   "new <title>",
	Short: "Create a new blog post",
	Long:  `Create a new blog post with the given title. The post will be created in the content directory.`,
	Args:  cobra.MinimumNArgs(1),
	Run:   runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) {
	new.Run(args)
	fmt.Println("\nBuilding site with new post...")
	if err := run.Run([]string{}); err != nil {
		fmt.Printf("build failed: %v\n", err)
	}
}
