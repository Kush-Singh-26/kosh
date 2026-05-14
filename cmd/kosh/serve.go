package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	"github.com/Kush-Singh-26/kosh/internal/server"
	"github.com/Kush-Singh-26/kosh/internal/watch"
)

var (
	serveDev       bool
	serveHost      string
	servePort      string
	serveDrafts    bool
	serveBaseURL   string
	serveForceLock bool
	serveNoStaging bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the preview server",
	Long: `Start the preview server.

Use --dev for build + watch + serve with live reload and incremental rebuilds.
Without --dev, serve only hosts the current output directory.`,
	Run: runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().BoolVarP(&serveDev, "dev", "", false, "Enable development mode (build + watch + serve)")
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "Host/IP to bind to")
	serveCmd.Flags().StringVar(&servePort, "port", "2604", "Port to listen on")
	serveCmd.Flags().BoolVarP(&serveDrafts, "drafts", "", false, "Include draft posts in development mode")
	serveCmd.Flags().StringVarP(&serveBaseURL, "baseurl", "", "", "Override base URL from config")
	serveCmd.Flags().BoolVar(&serveForceLock, "force-lock", false, "Acquire build lock even if another build is running")
	serveCmd.Flags().BoolVar(&serveNoStaging, "no-staging", false, "Disable atomic staging (overwrites output in place)")
}

func runServe(cmd *cobra.Command, _ []string) {
	ctx := cmd.Context()
	filteredArgs := collectServeArgs()

	if serveDev {
		runDevServe(ctx, filteredArgs)
	} else {
		runStaticServe(ctx, filteredArgs)
	}
}

func collectServeArgs() []string {
	var filteredArgs []string
	if serveBaseURL != "" {
		filteredArgs = append(filteredArgs, "-baseurl", serveBaseURL)
	}
	if serveNoStaging {
		filteredArgs = append(filteredArgs, "-no-staging")
	}
	if serveDrafts {
		filteredArgs = append(filteredArgs, "-drafts")
	}
	if serveHost != "localhost" {
		filteredArgs = append(filteredArgs, "-host", serveHost)
	}
	if servePort != "2604" {
		filteredArgs = append(filteredArgs, "-port", servePort)
	}
	if serveForceLock {
		filteredArgs = append(filteredArgs, "-force-lock")
	}
	if debug {
		filteredArgs = append(filteredArgs, "-debug")
	}
	return filteredArgs
}

func runDevServe(ctx context.Context, filteredArgs []string) {
	cfg := config.Load(filteredArgs)
	config.SetDevMode(cfg, true)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:2604"
	}
	printStartupBanner("Live Preview", cfg)
	engine := setupDevEngine(cfg)
	defer engine.Close()
	defer engine.SaveCaches()

	if err := engine.Build(ctx); err != nil {
		orchestration.DevLogError("Build failed: " + err.Error())
		os.Exit(1)
	}

	watcher := startWatcher(ctx, engine)
	if watcher != nil {
		defer watcher.Close()
	}

	server.Run(server.Options{
		Ctx:            ctx,
		Args:           filteredArgs,
		OutputDir:      engine.Cfg.OutputDir,
		RootDirectory:  engine.Cfg.Server.RootDirectory,
		SiteRoot:       engine.Cfg.SiteRoot,
		BaseURL:        engine.Cfg.BaseURL,
		BuildConfig:    engine.Cfg.Build,
		Reporter:       reporter,
		IsDev:          true,
		HealthRegistry: engine.Health,
	})
}

func setupDevEngine(cfg *config.Config) *orchestration.Engine {
	engine := orchestration.NewEngine(orchestration.WithConfig(cfg))
	if reporter != nil {
		engine.SetReporter(reporter)
		reporter.Start("Live Preview")
	}
	engine.OnBuildStart = func() { server.SetBuildActive(true) }
	engine.OnBuildDone = func() {
		server.SetBuildActive(false)
		server.BroadcastReload("site", "")
	}
	engine.OnSearchStart = func() { server.SetBuildActive(true) }
	engine.OnSearchDone = func() {
		server.SetBuildActive(false)
		server.BroadcastReload("site", "")
	}
	return engine
}

func startWatcher(ctx context.Context, engine *orchestration.Engine) *watch.Watcher {
	watchDirs := []string{engine.Cfg.ContentDir, engine.Cfg.TemplateDir, engine.Cfg.LayoutsDir, engine.Cfg.StaticDir, "kosh.yaml"}

	watcher, err := watch.New(watchDirs, func(event watch.Event) {
		orchestration.DevLogChange(event.Name, "watch")
		engine.BuildChanged(ctx, event.Name, event.Op)
	})
	if err != nil {
		orchestration.DevLogError("Watcher failed: " + err.Error())
		return nil
	}

	async.FireAndForget(ctx, slog.Default(), "watcher", func() error {
		go func() {
			<-ctx.Done()
			_ = watcher.Close()
		}()

		watcher.Start()
		return nil
	})

	return watcher
}

func runStaticServe(ctx context.Context, filteredArgs []string) {
	cfg := config.Load(filteredArgs)
	printStartupBanner("Static Preview", cfg)

	// Verify output directory exists
	if _, err := os.Stat(cfg.OutputDir); os.IsNotExist(err) {
		orchestration.DevLogError(fmt.Sprintf("Output directory '%s' not found. Run 'kosh build' first.", cfg.OutputDir))
		os.Exit(1)
	}

	server.Run(server.Options{
		Ctx:            ctx,
		Args:           filteredArgs,
		OutputDir:      cfg.OutputDir,
		RootDirectory:  cfg.Server.RootDirectory,
		SiteRoot:       cfg.SiteRoot,
		BaseURL:        cfg.BaseURL,
		BuildConfig:    cfg.Build,
		Reporter:       reporter,
		HealthRegistry: orchestration.NewBuildHealthRegistry(),
	})
}
