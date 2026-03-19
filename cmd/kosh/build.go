package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"

	"github.com/Kush-Singh-26/kosh/internal/watch"
)

var (
	buildWatch            bool
	buildCPUProfile       string
	buildMemProfile       string
	buildBaseURL          string
	buildDrafts           bool
	buildTheme            string
	buildForceLock        bool
	buildPhaseTimings     bool
	buildPhaseTimingsFile string
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
	buildCmd.Flags().BoolVar(&buildForceLock, "force-lock", false, "Acquire build lock even if another build is running")
	buildCmd.Flags().BoolVar(&buildPhaseTimings, "phase-timings", false, "Print per-phase build timings")
	buildCmd.Flags().StringVar(&buildPhaseTimingsFile, "phase-timings-file", "", "Write per-phase timings to a JSON file")
}

func runBuild(cmd *cobra.Command, args []string) {
	ctx := getContext()
	timeutil.ResetPhaseTracking()
	if buildPhaseTimings || buildPhaseTimingsFile != "" {
		timeutil.EnablePhaseTracking()
		defer timeutil.DisablePhaseTracking()
	}

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
	if buildForceLock {
		filteredArgs = append(filteredArgs, "-force-lock")
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

	cfg := config.Load(filteredArgs)
	mode := "Production Build"
	if buildWatch {
		mode = "Watch Build"
	}
	printStartupBanner(mode, cfg)

	if buildWatch {
		b := orchestration.NewEngine(filteredArgs)
		if err := b.Build(ctx); err != nil {
			orchestration.DevLogError("Initial build failed: " + err.Error())
			os.Exit(1)
		}
		maybePrintPhaseTimings()
		maybeWritePhaseTimings()

		w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
			orchestration.DevLogRebuild("Change detected: " + event.Name)
			timeutil.ResetPhaseTracking()
			b.BuildChanged(ctx, event.Name, event.Op)
			maybePrintPhaseTimings()
			maybeWritePhaseTimings()
		})
		if err != nil {
			orchestration.DevLogError("Watcher failed: " + err.Error())
			os.Exit(1)
		}
		w.Start()
	} else {
		if err := orchestration.Run(filteredArgs); err != nil {
			os.Exit(1)
		}
		maybePrintPhaseTimings()
		maybeWritePhaseTimings()

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

func maybePrintPhaseTimings() {
	if !buildPhaseTimings {
		return
	}
	summary := timeutil.FormatPhaseSummary()
	if summary != "" {
		fmt.Print(summary)
	}
}

func maybeWritePhaseTimings() {
	if buildPhaseTimingsFile == "" {
		return
	}
	if err := timeutil.WritePhaseDurationsJSON(buildPhaseTimingsFile); err != nil {
		fmt.Printf("failed to write phase timings: %v\n", err)
	}
}
