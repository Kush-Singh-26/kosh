// Package main implements the kosh build command.
package main

import (
	"context"
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
	buildNoStaging        bool
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
	buildCmd.Flags().BoolVar(&buildNoStaging, "no-staging", false, "Disable atomic staging (overwrites output in place)")
}

func runBuild(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()
	initPhaseTracking()

	filteredArgs := collectBuildArgs()

	if buildCPUProfile != "" {
		cleanup := setupCPUProfiling()
		defer cleanup()
	}

	cfg := config.Load(filteredArgs)
	mode := "Production Build"
	if buildWatch {
		mode = "Watch Build"
	}
	printStartupBanner(mode, cfg)

	if buildWatch {
		runWatchBuild(ctx, filteredArgs)
	} else {
		runSingleBuild(filteredArgs)
	}
}

func initPhaseTracking() {
	timeutil.ResetPhaseTracking()
	if buildPhaseTimings || buildPhaseTimingsFile != "" {
		timeutil.EnablePhaseTracking()
	}
}

func collectBuildArgs() []string {
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
	if debug {
		filteredArgs = append(filteredArgs, "-debug")
	}
	if buildNoStaging {
		filteredArgs = append(filteredArgs, "-no-staging")
	}
	return filteredArgs
}

func setupCPUProfiling() func() {
	profileFile, err := os.Create(buildCPUProfile)
	if err != nil {
		fmt.Printf("could not create CPU profile: %v\n", err)
		os.Exit(1)
	}
	if err := pprof.StartCPUProfile(profileFile); err != nil {
		fmt.Printf("could not start CPU profile: %v\n", err)
		_ = profileFile.Close()
		os.Exit(1)
	}
	return func() {
		pprof.StopCPUProfile()
		_ = profileFile.Close()
	}
}

func runWatchBuild(ctx context.Context, filteredArgs []string) {
	engine := orchestration.NewEngine(orchestration.WithArgs(filteredArgs), orchestration.WithReporter(reporter))
	if reporter != nil {
		reporter.Start("Watch Build")
	}
	if err := engine.Build(ctx); err != nil {
		orchestration.DevLogError("Initial build failed: " + err.Error())
		timeutil.DisablePhaseTracking()
		os.Exit(1)
	}
	maybePrintPhaseTimings()
	maybeWritePhaseTimings()

	watchDirs := []string{"content", engine.Cfg.TemplateDir, engine.Cfg.LayoutsDir, engine.Cfg.StaticDir, "kosh.yaml"}
	watcher, err := watch.New(watchDirs, func(event watch.Event) {
		orchestration.DevLogRebuild("Change detected: " + event.Name)
		timeutil.ResetPhaseTracking()
		engine.BuildChanged(ctx, event.Name, event.Op)
		maybePrintPhaseTimings()
		maybeWritePhaseTimings()
	})
	if err != nil {
		orchestration.DevLogError("Watcher failed: " + err.Error())
		timeutil.DisablePhaseTracking()
		os.Exit(1)
	}
	watcher.Start()
}

func runSingleBuild(filteredArgs []string) {
	if err := orchestration.Run(filteredArgs, reporter); err != nil {
		timeutil.DisablePhaseTracking()
		os.Exit(1)
	}
	maybePrintPhaseTimings()
	maybeWritePhaseTimings()

	if buildMemProfile != "" {
		writeMemProfile()
	}
}

func writeMemProfile() {
	profileFile, err := os.Create(buildMemProfile)
	if err != nil {
		fmt.Printf("could not create memory profile: %v\n", err)
		os.Exit(1)
	}
	runtime.GC()
	if err := pprof.WriteHeapProfile(profileFile); err != nil {
		fmt.Printf("could not write memory profile: %v\n", err)
		timeutil.DisablePhaseTracking()
		_ = profileFile.Close()
		os.Exit(1)
	}
	_ = profileFile.Close()
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
