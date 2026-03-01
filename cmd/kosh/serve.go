package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/internal/server"
	"github.com/Kush-Singh-26/kosh/internal/watch"
)

var (
	serveDev     bool
	serveHost    string
	servePort    string
	serveDrafts  bool
	serveBaseURL string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the preview server",
	Long:  `Start the preview server. Use --dev for development mode with live reload.`,
	Run:   runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	serveCmd.Flags().BoolVarP(&serveDev, "dev", "", false, "Enable development mode (build + watch + serve)")
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "Host/IP to bind to")
	serveCmd.Flags().StringVar(&servePort, "port", "2604", "Port to listen on")
	serveCmd.Flags().BoolVarP(&serveDrafts, "drafts", "", false, "Include draft posts in development mode")
	serveCmd.Flags().StringVarP(&serveBaseURL, "baseurl", "", "", "Override base URL from config")
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

	if serveDev {
		fmt.Println("🚀 Starting Kosh in Development Mode...")
		cfg := config.Load(filteredArgs)
		if cfg.BaseURL == "" {
			cfg.BaseURL = "http://localhost:2604"
			fmt.Println("   📝 Auto-detected baseURL: http://localhost:2604")
		}
		b := run.NewBuilderWithConfig(cfg)
		b.SetDevMode(true)
		if err := b.Build(ctx); err != nil {
			fmt.Printf("❌ Build failed: %v\n", err)
			os.Exit(1)
		}

		go func() {
			w, err := watch.New([]string{"content", b.Config().TemplateDir, b.Config().StaticDir, "kosh.yaml"}, func(event watch.Event) {
				fmt.Printf("\n⚡ Change detected: %s | Rebuilding...\n", event.Name)
				b.BuildChanged(ctx, event.Name, event.Op)
			})
			if err != nil {
				fmt.Printf("❌ Watcher failed: %v\n", err)
				return
			}
			w.Start()
		}()

		server.Run(ctx, filteredArgs, b.Config().OutputDir, b.Config().Build)
	} else {
		cfg := config.Load(filteredArgs)
		server.Run(ctx, filteredArgs, cfg.OutputDir, cfg.Build)
	}
}
