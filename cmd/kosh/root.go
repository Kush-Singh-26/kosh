package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "kosh",
	Short: "High-performance static site generator",
	Long: `Kosh is a high-performance Static Site Generator built in Go.
It supports full builds, incremental development rebuilds, CSS/JS asset fingerprinting,
WebP image conversion for eligible local raster images, SSR for math and D2, and Go+WASM search.`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = false
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			printStartupBanner("CLI Help", nil)
		}
		defaultHelp(cmd, args)
	})
}

func getContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 2) // Buffer for 2 signals
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\n🛑 Received signal: %s. Initiating graceful shutdown...\n", sig)
		cancel()

		// Second signal forces exit
		select {
		case sig2 := <-sigChan:
			fmt.Printf("\n🛑 Received second signal: %s. Forcing exit.\n", sig2)
			os.Exit(1)
		case <-time.After(2 * time.Second):
			// After 2 seconds, the user can still force exit with another Ctrl+C
			// but we keep the listener alive just in case.
			go func() {
				sig3 := <-sigChan
				fmt.Printf("\n🛑 Received forceful Signal: %s. Exiting.\n", sig3)
				os.Exit(1)
			}()
		}
	}()

	return ctx
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
