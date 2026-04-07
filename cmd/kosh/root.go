package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"charm.land/fang/v2"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/builder/ui"
	"github.com/spf13/cobra"
)

var (
	verbose  bool
	reporter ui.Reporter
)

var rootCmd = &cobra.Command{
	Use:   "kosh",
	Short: "High-performance static site generator",
	Long: `Kosh is a high-performance Static Site Generator built in Go.
It supports full builds, incremental development rebuilds, CSS/JS asset fingerprinting,
WebP image conversion for eligible local raster images, SSR for math and D2, and Go+WASM search.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Initialize standardized themed logger for all commands
		reporter = ui.NewReporter(verbose)
		slog.SetDefault(orchestration.InitLogger(reporter))
	},
	Run: func(cmd *cobra.Command, args []string) {
		printStartupBanner("CLI Help", nil)
		_ = cmd.Help()
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.CompletionOptions.DisableDefaultCmd = false
	rootCmd.Version = cliVersion
}

func getContext() context.Context {
	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 2) // Buffer for 2 signals
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		orchestration.DevLogInfo("Received signal: " + sig.String() + ". Initiating graceful shutdown...")
		cancel()

		// Second signal forces exit
		select {
		case sig2 := <-sigChan:
			orchestration.DevLogInfo("Received second signal: " + sig2.String() + ". Forcing exit.")
			os.Exit(1)
		case <-time.After(2 * time.Second):
			// After 2 seconds, the user can still force exit with another Ctrl+C
			// but we keep the listener alive just in case.
			go func() {
				sig3 := <-sigChan
				orchestration.DevLogInfo("Received forceful signal: " + sig3.String() + ". Exiting.")
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
