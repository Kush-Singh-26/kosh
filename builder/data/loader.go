// Package data provides functionality for loading and parsing site configuration and data files.
package data

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// Map represents the structured site data.
type Map map[string]any

// Load reads all .yaml, .yml, and .json files from the data directory.
func Load(fs afero.Fs, dataDir string) (Map, error) {
	exists, err := afero.DirExists(fs, dataDir)
	if err != nil || !exists {
		return make(Map), nil
	}

	siteData := make(Map)

	err = afero.Walk(fs, dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || path == dataDir {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil
		}

		content, err := afero.ReadFile(fs, path)
		if err != nil {
			return fmt.Errorf("failed to read data file %s: %w", path, err)
		}

		// Use the filename stem as the key
		name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

		var decoded any
		if ext == ".json" {
			if err := json.Unmarshal(content, &decoded); err != nil {
				return fmt.Errorf("failed to parse JSON from %s: %w", path, err)
			}
		} else {
			if err := yaml.Unmarshal(content, &decoded); err != nil {
				return fmt.Errorf("failed to parse YAML from %s: %w", path, err)
			}
		}

		siteData[name] = decoded
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk data directory: %w", err)
	}

	return siteData, nil
}
