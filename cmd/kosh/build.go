package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/internal/watch"
)

var (
	buildWatch      bool
	buildCPUProfile string
	buildMemProfile string
	buildBaseURL    string
	buildDrafts     bool
	buildTheme      string
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the static site",
	Long:  `Build the static site from markdown content. Supports watching for changes and profiling.`,
	Run:   runBuild,
}

func init() {
	rootCmd.AddCommand(buildCmd)

	buildCmd.Flags().BoolVarP(&buildWatch, "watch", "w", false, "Watch for changes and rebuild")
	buildCmd.Flags().StringVar(&buildCPUProfile, "cpuprofile", "", "Write CPU profile to file")
	buildCmd.Flags().StringVar(&buildMemProfile, "memprofile", "", "Write memory profile to file")
	buildCmd.Flags().StringVarP(&buildBaseURL, "baseurl", "", "", "Override base URL from config")
	buildCmd.Flags().BoolVarP(&buildDrafts, "drafts", "", false, "Include draft posts in build")
	buildCmd.Flags().StringVarP(&buildTheme, "theme", "", "", "Override theme from config")
}

func runBuild(cmd *cobra.Command, args []string) {
	ctx := getContext()

	var filteredArgs []string
	if buildBaseURL != "" {
		filteredArgs = append(filteredArgs, "-baseurl", buildBaseURL)
	}
	if buildDrafts {
		filteredArgs = append(filteredArgs, "-drafts")
	}
	if buildTheme != "" {
		filteredArgs = append(filteredArgs, "-theme", buildTheme)
	}

	if buildCPUProfile != "" {
		f, err := os.Create(buildCPUProfile)
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

	if buildWatch {
		b := run.NewBuilder(filteredArgs)
		if err := b.Build(ctx); err != nil {
			fmt.Printf("❌ Initial build failed: %v\n", err)
			os.Exit(1)
		}

		w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
			fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
			b.BuildChanged(ctx, event.Name, event.Op)
		})
		if err != nil {
			fmt.Printf("❌ Watcher failed: %v\n", err)
			os.Exit(1)
		}
		w.Start()
	} else {
		run.Run(filteredArgs)

		if buildMemProfile != "" {
			f, err := os.Create(buildMemProfile)
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
}
