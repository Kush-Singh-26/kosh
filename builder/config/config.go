// handles command-line flags
package config

import (
	"flag"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string
	CompressImages bool
	ForceRebuild   bool
	BuildVersion   int64
	PostsPerPage   int
}

func Load(args []string) *Config {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	baseUrlFlag := fs.String("baseurl", "", "Base URL")
	compressFlag := fs.Bool("compress", false, "Enable image compression")

	// Parse the provided arguments instead of os.Args
	fs.Parse(args)

	return &Config{
		BaseURL:        strings.TrimSuffix(*baseUrlFlag, "/"),
		CompressImages: *compressFlag,
		ForceRebuild:   false,
		BuildVersion:   time.Now().Unix(),
		PostsPerPage:   10,
	}
}
