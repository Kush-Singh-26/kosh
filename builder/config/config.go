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
}

func Load() *Config {
	baseUrlFlag := flag.String("baseurl", "", "Base URL")
	compressFlag := flag.Bool("compress", false, "Enable image compression")
	flag.Parse()

	return &Config{
		BaseURL:        strings.TrimSuffix(*baseUrlFlag, "/"),
		CompressImages: *compressFlag,
		ForceRebuild:   false, 
		BuildVersion:   time.Now().Unix(),
	}
}