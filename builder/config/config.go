// handles command-line flags
package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// Global flag to track if we're in development mode
var isDevMode = atomic.Bool{}

type MenuEntry struct {
	Name   string `yaml:"name"`
	URL    string `yaml:"url,omitempty"`
	Target string `yaml:"target,omitempty"`
	ID     string `yaml:"id,omitempty"`
	Class  string `yaml:"class,omitempty"`
}

type AuthorConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type Config struct {
	Title          string       `yaml:"title"`
	Description    string       `yaml:"description"`
	BaseURL        string       `yaml:"baseURL"`
	Language       string       `yaml:"language"`
	Author         AuthorConfig `yaml:"author"`
	Menu           []MenuEntry  `yaml:"menu"`
	PostsPerPage   int          `yaml:"postsPerPage"`
	CompressImages bool         `yaml:"compressImages"`
	// Internal / Runtime fields
	ForceRebuild  bool  `yaml:"-"`
	IncludeDrafts bool  `yaml:"-"`
	BuildVersion  int64 `yaml:"-"`
	IsDev         bool  `yaml:"-"`
}

func Load(args []string) *Config {
	// 1. Default Configuration
	cfg := &Config{
		Title:        "Kosh Blog",
		BaseURL:      "",
		PostsPerPage: 10,
		BuildVersion: time.Now().Unix(),
	}

	// 2. Load from YAML file if exists
	if data, err := os.ReadFile("kosh.yaml"); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			fmt.Printf("⚠️ Failed to parse kosh.yaml: %v\n", err)
		}
	} else {
		// Try fallback to config.yaml
		if data, err := os.ReadFile("config.yaml"); err == nil {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				fmt.Printf("⚠️ Failed to parse config.yaml: %v\n", err)
			}
		}
	}

	// 3. Override with CLI Flags
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	baseUrlFlag := fs.String("baseurl", "", "Base URL (overrides config file)")
	compressFlag := fs.Bool("compress", false, "Enable image compression (overrides config file)")
	draftsFlag := fs.Bool("drafts", false, "Include draft posts in the build")

	_ = fs.Parse(args)

	if *baseUrlFlag != "" {

		cfg.BaseURL = strings.TrimSuffix(*baseUrlFlag, "/")
	}
	if *compressFlag {
		cfg.CompressImages = true
	}
	if *draftsFlag {
		cfg.IncludeDrafts = true
	}

	return cfg
}

// SetDevMode is a helper to set development mode on a config pointer
func SetDevMode(cfg *Config, isDev bool) {
	cfg.IsDev = isDev
	isDevMode.Store(isDev)
}

// IsDevMode returns true if currently in development mode
func IsDevMode() bool {
	return isDevMode.Load()
}
