package main

import (
	"os"

	"github.com/spf13/cobra"

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
}

func runServe(cmd *cobra.Command, args []string) {
	ctx := getContext()

	var filteredArgs []string
	if serveBaseURL != "" {
		filteredArgs = append(filteredArgs, "-baseurl", serveBaseURL)
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

	if serveDev {
		cfg := config.Load(filteredArgs)
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:2604"
		}
		printStartupBanner("Live Preview", cfg)
		b := orchestration.NewEngineWithConfig(cfg)
		b.SetDevMode(true)
		if err := b.Build(ctx); err != nil {
			orchestration.DevLogError("Build failed: " + err.Error())
			os.Exit(1)
		}

		go func() {
			w, err := watch.New([]string{b.Config().ContentDir, b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
				orchestration.DevLogChange(event.Name, "watch")
				server.SetBuildActive(true)
				b.BuildChanged(ctx, event.Name, event.Op)
				server.SetBuildActive(false)
			})
			if err != nil {
				orchestration.DevLogError("Watcher failed: " + err.Error())
				return
			}
			w.Start()
		}()

		server.Run(ctx, filteredArgs, b.Config().OutputDir, b.Config().BaseURL, b.Config().Build)
	} else {
		cfg := config.Load(filteredArgs)
		printStartupBanner("Static Preview", cfg)
		server.Run(ctx, filteredArgs, cfg.OutputDir, cfg.BaseURL, cfg.Build)
	}
}
