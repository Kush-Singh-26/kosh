package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	"my-ssg/builder/run"
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
		// Check for --cache flag
		cleanCache := false
		var filteredArgs []string
		for _, arg := range args {
			if arg == "--cache" || arg == "-cache" {
				cleanCache = true
			} else {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		args = filteredArgs

		clean.Run(cleanCache)
		// Auto-rebuild after clean
		fmt.Println("\n🔄 Rebuilding site...")
		run.Run([]string{})

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
			// Check if we need to rebuild. If not, skip.
			// How to check? If 'public' exists and seems fresh.
			// But user might want to ensure it's up to date.
			// Let's rely on Builder's internal smart caching.
			// The issue is `b.Build()` always runs fully at start.

			// If we want to skip initial build if "kosh clean" just ran,
			// we can check if "public/index.html" is very recent?
			// Or pass a flag "SkipInitialBuild" if we knew.
			// But here we are running a separate command.

			// The user complains: "Why rebuilding, when it was already built".
			// Let's make the initial build conditional or smarter.
			// If we create a Builder, it loads cache.
			// `b.Build()` checks timestamps.
			// If `public/index.html` is newer than all sources, `b.Build()` should naturally skip most work.
			// BUT `b.Build()` in `run.go` has logic:
			/*
				if indexInfo, err := os.Stat("public/index.html"); err == nil {
					lastBuildTime := indexInfo.ModTime()
					...
				} else {
					shouldForce = true
				}
			*/
			// If `clean` ran, `public` exists.
			// If `clean` just finished, `public/index.html` is NEW.
			// So `lastBuildTime` is NOW.
			// `dep` mod time is OLDER than NOW.
			// So `shouldForce` stays false.

			// However, `filesToProcess` loop checks:
			/*
				destInfo, err := b.DestFs.Stat(destPath)
				if err == nil {
					if destInfo.ModTime().After(info.ModTime()) {
						skipRendering = true
					}
				}
			*/
			// In `clean`, we built to disk.
			// In `serve`, we start a NEW Builder.
			// `NewBuilder` creates `destFs := afero.NewMemMapFs()`.
			// `DestFs` is EMPTY!
			// So `b.DestFs.Stat(destPath)` returns NotExist.
			// So it rebuilds EVERYTHING.

			// FIX: We need to populate `DestFs` from `public` folder if it exists on disk!
			// Or verify against `public` folder on disk.

			// Option A: Initialize `DestFs` with content from `public` (Mirror).
			// Option B: `NewBuilder` takes `DestFs` as argument? No.
			// Option C: `NewBuilder` can use `NewOsFs` for DestFs?
			// If we use `NewOsFs` for DestFs, then we are writing to disk directly.
			// The goal of VFS was speed.
			// But in Dev mode, maybe we want persistence or read from disk to avoid rebuild?

			// If we use `MemMapFs`, it is empty at start.
			// We can "Hydrate" the VFS from `public` disk folder at startup.

			// Let's add hydration logic to `NewBuilder` or `Run`.

			b := run.NewBuilder(args)

			b.SetDevMode(true)
			b.Build()

			// 2. Start Watcher in background
			go func() {
				w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
					fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
					b.BuildChanged(event.Name)
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
		// Check for flags
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
			defer f.Close()
			if err := pprof.StartCPUProfile(f); err != nil {
				fmt.Printf("could not start CPU profile: %v\n", err)
				os.Exit(1)
			}
			defer pprof.StopCPUProfile()
		}

		if isWatch {
			b := run.NewBuilder(args)
			b.Build() // Initial build

			w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
				fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
				b.BuildChanged(event.Name)
			})
			if err != nil {
				fmt.Printf("❌ Watcher failed: %v\n", err)
				os.Exit(1)
			}
			w.Start()
		} else {
			// Standard one-off build
			run.Run(args)

			if memProfile != "" {
				f, err := os.Create(memProfile)
				if err != nil {
					fmt.Printf("could not create memory profile: %v\n", err)
					os.Exit(1)
				}
				defer f.Close()
				runtime.GC() // get up-to-date statistics
				if err := pprof.WriteHeapProfile(f); err != nil {
					fmt.Printf("could not write memory profile: %v\n", err)
					os.Exit(1)
				}
			}
		}

	case "cache":
		handleCacheCommand(args)

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
	fmt.Println("  clean          Clean public directory & rebuild")
	fmt.Println("  serve          Start the preview server")
	fmt.Println("  build          Build the static site (and WASM)")
	fmt.Println("  cache          Cache management commands")
	fmt.Println("  help           Show this help message")
	fmt.Println("\nFlags for clean:")
	fmt.Println("  --cache        Also clean .kosh-cache directory (force full re-render)")
	fmt.Println("\nFlags for build:")
	fmt.Println("  -baseurl       Base URL for the site")
	fmt.Println("  -drafts        Include draft posts in the build")
	fmt.Println("  --cpuprofile <file>  Write CPU profile to file")
	fmt.Println("  --memprofile <file>  Write memory profile to file")
	fmt.Println("\nFlags for serve:")
	fmt.Println("  --dev          Enable development mode (serve + watch)")
	fmt.Println("  -drafts        Include draft posts in development mode")
}
