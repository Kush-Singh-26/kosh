package asset

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/config"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

const fingerprintFileMode = 0644

// ComputeStaticFingerprint hashes static asset metadata to detect changes.
func ComputeStaticFingerprint(sourceFs afero.Fs, dirs []string) (string, error) {
	hasher := xxh3.New()
	var fileCount int

	for _, dir := range dirs {
		dir = fspkg.NormalizePath(dir)
		err := afero.Walk(sourceFs, dir, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			relPath, err := filepath.Rel(dir, path)
			if err != nil {
				return nil
			}
			relPath = filepath.ToSlash(relPath)
			if _, err := fmt.Fprintf(hasher, "%s:%d:%d;", relPath, info.Size(), info.ModTime().UnixNano()); err != nil {
				return err
			}
			fileCount++
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("failed to walk directory %s: %w", dir, err)
		}
	}

	hash := hasher.Sum128()
	hashBytes := hash.Bytes()
	return hex.EncodeToString(hashBytes[:]), nil
}

// GetStaticDirs returns existing static directories based on config.
func GetStaticDirs(config *config.Config) []string {
	var dirs []string

	themeStatic := filepath.Join(config.ThemeDir, config.Theme, "static")
	if config.StaticDir != "" {
		themeStatic = config.StaticDir
	}
	if _, err := os.Stat(themeStatic); err == nil {
		dirs = append(dirs, themeStatic)
	}

	siteStatic := "static"
	if config.SiteRoot != "" {
		siteStatic = filepath.Join(config.SiteRoot, "static")
	}
	if _, err := os.Stat(siteStatic); err == nil {
		dirs = append(dirs, siteStatic)
	}

	assetsDir := "assets"
	if config.SiteRoot != "" {
		assetsDir = filepath.Join(config.SiteRoot, "assets")
	}
	if _, err := os.Stat(assetsDir); err == nil {
		dirs = append(dirs, assetsDir)
	}

	if config.Server.RootDirectory != "" {
		parentAssets := filepath.Join(config.Server.RootDirectory, "assets")
		if _, err := os.Stat(parentAssets); err == nil {
			dirs = append(dirs, parentAssets)
		}
	}

	return dirs
}

var errCacheDirNotSet = errors.New("cache directory not set")

// LoadStaticFingerprint reads the static fingerprint from cache.
func LoadStaticFingerprint(cacheDir string) (string, error) {
	if cacheDir == "" {
		return "", errCacheDirNotSet
	}
	fingerprintPath := filepath.Join(cacheDir, "static-fingerprint")
	data, err := os.ReadFile(fingerprintPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// SaveStaticFingerprint writes the static fingerprint to cache.
func SaveStaticFingerprint(cacheDir, fingerprint string) error {
	if cacheDir == "" {
		return errCacheDirNotSet
	}
	fingerprintPath := filepath.Join(cacheDir, "static-fingerprint")
	return os.WriteFile(fingerprintPath, []byte(fingerprint), fingerprintFileMode)
}
