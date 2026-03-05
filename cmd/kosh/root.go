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
	Short: "Kosh - A high-performance Static Site Generator",
	Long: `Kosh is a high-performance Static Site Generator built in Go.
It features BLAKE3 hashing, object pooling, pre-computed search indexes,
and generic cache operations for optimal performance.`,
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = false
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
