package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n🛑 Received shutdown signal...")
		cancel()
	}()

	return ctx
}

func execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
