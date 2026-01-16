package main

import (
	"fmt"
	"os"

	"my-ssg/builder/run"
	"my-ssg/internal/build"
	"my-ssg/internal/clean"
	"my-ssg/internal/new"
	"my-ssg/internal/server"
	"my-ssg/internal/watch"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "clean":
		clean.Run()
		// Auto-rebuild after clean (compressed)
		fmt.Println("\n🔄 Rebuilding site (compressed)...")
		build.CheckWASM()
		run.Run([]string{"-compress"})

	case "new":
		new.Run(args)

	case "serve":
		// Check for --dev flag
		isDev := false
		var filteredArgs []string
		for _, arg := range args {
			if arg == "--dev" || arg == "-dev" {
				isDev = true
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		args = filteredArgs

		if isDev {
			fmt.Println("🚀 Starting Kosh in Development Mode...")
			// 1. Initial Build
			build.CheckWASM()
			b := run.NewBuilder(args)
			b.Build()

			// 2. Start Watcher in background
			go func() {
				w, err := watch.New([]string{"content", "templates", "static", "kosh.yaml"}, func(event watch.Event) {
					fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
					b.Build()
				})
				if err != nil {
					fmt.Printf("❌ Watcher failed: %v\n", err)
					return
				}
				w.Start()
			}()

			// 3. Start Server (blocking)
			server.Run(args)
		} else {
			server.Run(args)
		}

	case "build":
		// Check for --watch flag
		isWatch := false
		var filteredArgs []string
		for _, arg := range args {
			if arg == "--watch" || arg == "-watch" {
				isWatch = true
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		args = filteredArgs

		// 1. Check/Build WASM first
		build.CheckWASM()

		if isWatch {
			b := run.NewBuilder(args)
			b.Build() // Initial build

			w, err := watch.New([]string{"content", "templates", "static", "kosh.yaml"}, func(event watch.Event) {
				fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
				b.Build()
			})
			if err != nil {
				fmt.Printf("❌ Watcher failed: %v\n", err)
				os.Exit(1)
			}
			w.Start()
		} else {
			// Standard one-off build
			run.Run(args)
		}

	case "help", "-help", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: kosh <command> [arguments]")
	fmt.Println("\nCommands:")
	fmt.Println("  new <title>    Create a new blog post")
	fmt.Println("  clean          Clean public directory & rebuild compressed")
	fmt.Println("  serve          Start the preview server")
	fmt.Println("  build          Build the static site (and WASM)")
	fmt.Println("  help           Show this help message")
	fmt.Println("\nFlags for build:")
	fmt.Println("  -compress      Enable minification")
	fmt.Println("  --watch        Enable watch mode (continuous rebuild)")
	fmt.Println("\nFlags for serve:")
	fmt.Println("  --dev          Enable development mode (serve + watch)")
}
