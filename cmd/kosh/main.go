package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/internal/clean"
	"github.com/Kush-Singh-26/kosh/internal/new"
	"github.com/Kush-Singh-26/kosh/internal/scaffold"
	"github.com/Kush-Singh-26/kosh/internal/server"
	"github.com/Kush-Singh-26/kosh/internal/version"
	"github.com/Kush-Singh-26/kosh/internal/watch"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n🛑 Received shutdown signal...")
		cancel()
	}()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "clean":
		cleanCache := false
		cleanAll := false
		for _, arg := range args {
			if arg == "--cache" || arg == "-cache" {
				cleanCache = true
			}
			if arg == "--all" || arg == "-all" {
				cleanAll = true
			}
		}

		clean.Run(cleanCache, cleanAll)
		fmt.Println("\n🔄 Rebuilding site...")
		run.Run([]string{})

	case "new":
		new.Run(args)
		fmt.Println("\n🔄 Building site with new post...")
		run.Run([]string{})

	case "init":
		scaffold.Run(args)

	case "serve":
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
			b := run.NewBuilder(args)
			b.SetDevMode(true)
			if err := b.Build(ctx); err != nil {
				fmt.Printf("❌ Build failed: %v\n", err)
				os.Exit(1)
			}

			go func() {
				w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
					fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
					b.BuildChanged(ctx, event.Name)
				})
				if err != nil {
					fmt.Printf("❌ Watcher failed: %v\n", err)
					return
				}
				w.Start()
			}()

			server.Run(ctx, args, b.Config().OutputDir)
		} else {
			cfg := config.Load(args)
			server.Run(ctx, args, cfg.OutputDir)
		}

	case "build":
		isWatch := false
		cpuProfile := ""
		memProfile := ""
		var filteredArgs []string
		for i := 0; i < len(args); i++ {
			arg := args[i]
			if arg == "--watch" || arg == "-watch" {
				isWatch = true
			} else if arg == "--cpuprofile" && i+1 < len(args) {
				cpuProfile = args[i+1]
				i++
			} else if arg == "--memprofile" && i+1 < len(args) {
				memProfile = args[i+1]
				i++
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		args = filteredArgs

		if cpuProfile != "" {
			f, err := os.Create(cpuProfile)
			if err != nil {
				fmt.Printf("could not create CPU profile: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = f.Close() }()
			if err := pprof.StartCPUProfile(f); err != nil {
				fmt.Printf("could not start CPU profile: %v\n", err)
				os.Exit(1)
			}
			defer pprof.StopCPUProfile()
		}

		if isWatch {
			b := run.NewBuilder(args)
			if err := b.Build(ctx); err != nil {
				fmt.Printf("❌ Initial build failed: %v\n", err)
				os.Exit(1)
			}

			w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
				fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
				b.BuildChanged(ctx, event.Name)
			})
			if err != nil {
				fmt.Printf("❌ Watcher failed: %v\n", err)
				os.Exit(1)
			}
			w.Start()
		} else {
			run.Run(args)

			if memProfile != "" {
				f, err := os.Create(memProfile)
				if err != nil {
					fmt.Printf("could not create memory profile: %v\n", err)
					os.Exit(1)
				}
				defer func() { _ = f.Close() }()
				runtime.GC()
				if err := pprof.WriteHeapProfile(f); err != nil {
					fmt.Printf("could not write memory profile: %v\n", err)
					os.Exit(1)
				}
			}
		}

	case "cache":
		handleCacheCommand(args)

	case "version":
		if len(args) > 0 && (args[0] == "-info" || args[0] == "--info") {
			printVersion()
		} else if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
			version.Run(args)
		} else {
			version.Run([]string{})
		}

	case "-version", "--version":
		printVersion()
		os.Exit(0)

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
	fmt.Println("  init [name]    Initialize a new Kosh site")
	fmt.Println("  new <title>    Create a new blog post")
	fmt.Println("  build          Build the static site")
	fmt.Println("  serve          Start the preview server")
	fmt.Println("  clean          Clean output directory")
	fmt.Println("  cache          Cache management commands")
	fmt.Println("  version        Version management commands")
	fmt.Println("  help           Show this help message")
	fmt.Println("\nBuild Flags:")
	fmt.Println("  --watch              Watch for changes and rebuild")
	fmt.Println("  --cpuprofile <file>  Write CPU profile to file")
	fmt.Println("  --memprofile <file>  Write memory profile to file")
	fmt.Println("  -baseurl <url>       Override base URL from config")
	fmt.Println("  -drafts              Include draft posts in build")
	fmt.Println("  -theme <name>        Override theme from config")
	fmt.Println("\nServe Flags:")
	fmt.Println("  --dev                Enable development mode (build + watch + serve)")
	fmt.Println("  --host <host>        Host/IP to bind to (default: localhost)")
	fmt.Println("  --port <port>        Port to listen on (default: 2604)")
	fmt.Println("  -drafts              Include draft posts in development mode")
	fmt.Println("  -baseurl <url>       Override base URL from config")
	fmt.Println("\nClean Flags:")
	fmt.Println("  --cache              Also clean .kosh-cache directory")
	fmt.Println("  --all                Clean all versions including versioned folders")
	fmt.Println("\nCache Commands:")
	fmt.Println("  cache stats          Show cache statistics")
	fmt.Println("  cache gc             Run garbage collection on cache")
	fmt.Println("  cache verify         Check cache integrity")
	fmt.Println("  cache rebuild        Clear cache for full rebuild")
	fmt.Println("  cache clear          Delete all cache data")
	fmt.Println("  cache inspect <path> Show cache entry for a file")
	fmt.Println("\nCache GC Flags:")
	fmt.Println("  --dry-run, -n        Show what would be deleted without deleting")
	fmt.Println("\nVersion Commands:")
	fmt.Println("  version              Show current documentation version info")
	fmt.Println("  version <vX.X>       Freeze current latest and start new version")
	fmt.Println("  version --info       Show Kosh build information")
}

func printVersion() {
	fmt.Println("Kosh Static Site Generator")
	fmt.Println("Version: v1.0.0")
	fmt.Printf("Go Version: %s\n", runtime.Version())
	fmt.Println("Build Date: 2026-02-12")
	fmt.Println("\nOptimized with:")
	fmt.Println("  - BLAKE3 hashing (replaced MD5)")
	fmt.Println("  - Object pooling for memory management")
	fmt.Println("  - Pre-computed search indexes")
	fmt.Println("  - Generic cache operations")
}
