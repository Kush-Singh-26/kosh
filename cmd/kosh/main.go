package main

import (
	"fmt"
	"os"

	"my-ssg/builder/run"
	"my-ssg/internal/build"
	"my-ssg/internal/clean"
	"my-ssg/internal/new"
	"my-ssg/internal/server"
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
		server.Run(args)
	case "build":
		// 1. Check/Build WASM first
		build.CheckWASM()
		// 2. Run Site Builder
		run.Run(args)
	case "help":
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
}
