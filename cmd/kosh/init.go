package main

import (
	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/internal/scaffold"
)

var initCmd = &cobra.Command{
	Use:   "init [name]",
	Short: "Initialize a new Kosh site",
	Long:  `Initialize a new Kosh site with the given name. Creates the basic directory structure and configuration.`,
	Args:  cobra.MaximumNArgs(1),
	Run:   runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, args []string) {
	scaffold.Run(args)
}
