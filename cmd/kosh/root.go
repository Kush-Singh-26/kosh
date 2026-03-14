package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/spf13/cobra"
	"charm.land/fang/v2"
)

var rootCmd = &cobra.Command{
	Use:   "kosh",
	Short: "High-performance static site generator",
	Long: `Kosh is a high-performance Static Site Generator built in Go.
It supports full builds, incremental development rebuilds, CSS/JS asset fingerprinting,
WebP image conversion for eligible local raster images, SSR for math and D2, and Go+WASM search.`,
	Run: func(cmd *cobra.Command, args []string) {
		printStartupBanner("CLI Help", nil)
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = false
	rootCmd.Version = cliVersion
}

func getContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 2) // Buffer for 2 signals
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		run.DevLogInfo("Received signal: " + sig.String() + ". Initiating graceful shutdown...")
		cancel()

		// Second signal forces exit
		select {
		case sig2 := <-sigChan:
			run.DevLogInfo("Received second signal: " + sig2.String() + ". Forcing exit.")
			os.Exit(1)
		case <-time.After(2 * time.Second):
			// After 2 seconds, the user can still force exit with another Ctrl+C
			// but we keep the listener alive just in case.
			go func() {
				sig3 := <-sigChan
				run.DevLogInfo("Received forceful signal: " + sig3.String() + ". Exiting.")
				os.Exit(1)
			}()
		}
	}()

	return ctx
}

func execute() {
	ctx := getContext()
	if err := fang.Execute(ctx, rootCmd); err != nil {
		os.Exit(1)
	}
}
