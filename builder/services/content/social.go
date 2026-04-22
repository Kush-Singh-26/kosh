package content

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/navigation"
)

func (service *contentService) queueSocialCard(options SocialCardOptions) {
	relativePath := options.RelativePath
	result := options.Result
	htmlRelativePath := options.HTMLRelativePath
	shouldForce := options.ForceSocialRebuild
	cardPool := options.CardPool

	seoTitle, _, _, currentHash := service.resolveSocialCardData(result)
	cardRelativePath, cardDestinationPath, _ := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, htmlRelativePath, currentHash)
	service.cleanupObsoleteSocialCard(relativePath, htmlRelativePath, currentHash)

	cacheDir := service.cfg.CacheDir
	if !filepath.IsAbs(cacheDir) {
		absoluteCacheDir, err := filepath.Abs(cacheDir)
		if err == nil {
			cacheDir = absoluteCacheDir
		}
	}
	cachedCardPath := filepath.Join(cacheDir, "social-cards", currentHash+".webp")

	if _, err := os.Stat(cachedCardPath); err == nil && !shouldForce && currentHash != "" {
		service.copyCachedSocialCard(currentHash, cardDestinationPath)
	} else {
		cardPool.Submit(socialCardTask{
			path: relativePath, relPath: cardRelativePath,
			cardDestPath: cardDestinationPath,
			seoTitle:     seoTitle,
			metadata:     result.Metadata, frontmatterHash: currentHash,
		})
	}
}

func (service *contentService) cleanupObsoleteSocialCard(contentRelPath, htmlRelPath, currentHash string) {
	if service.cache == nil || currentHash == "" {
		return
	}

	previousHash, err := service.cache.GetSocialCardHash(contentRelPath)
	if err != nil || previousHash == "" || previousHash == currentHash {
		return
	}

	cardRelPath, cardDestPath, _ := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, htmlRelPath, previousHash)
	service.removeOutputSocialCardFile(cardDestPath, cardRelPath)
	service.removeCachedSocialCardFile(previousHash)
}

func (service *contentService) removeOutputSocialCardFile(cardDestPath, cardRelPath string) {
	candidates := []string{cardDestPath}

	if sinkOutputDir := strings.TrimSpace(service.sink.GetOutputDir()); sinkOutputDir != "" {
		candidates = append(candidates, filepath.Join(
			sinkOutputDir,
			"static",
			"images",
			"cards",
			filepath.FromSlash(cardRelPath),
		))
	}

	for _, candidate := range candidates {
		service.removeIfWithinRoots(candidate, []string{service.cfg.OutputDir, service.sink.GetOutputDir()})
	}
}

func (service *contentService) removeCachedSocialCardFile(hash string) {
	cacheRoot := strings.TrimSpace(service.cfg.CacheDir)
	if hash == "" || cacheRoot == "" {
		return
	}

	cacheCardPath := filepath.Join(cacheRoot, "social-cards", hash+".webp")
	service.removeIfWithinRoots(cacheCardPath, []string{cacheRoot})
}

func (service *contentService) removeIfWithinRoots(target string, roots []string) {
	absTarget, err := filepath.Abs(filepath.FromSlash(target))
	if err != nil {
		return
	}

	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}

		absRoot, err := filepath.Abs(filepath.FromSlash(root))
		if err != nil {
			continue
		}
		if !isPathWithinRoot(absTarget, absRoot) {
			continue
		}

		if err := os.Remove(absTarget); err != nil && !errors.Is(err, os.ErrNotExist) {
			service.logger.Debug("Failed to remove stale social card file", "path", absTarget, "error", err)
		}
		return
	}
}

func isPathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if strings.HasPrefix(rel, "..") {
		return false
	}
	return !filepath.IsAbs(rel)
}

func (service *contentService) copyCachedSocialCard(cardHash, cardDestinationPath string) {
	if cardHash == "" {
		service.sink.Register(cardDestinationPath)
		return
	}
	cachedCardPath := filepath.Join(service.cfg.CacheDir, "social-cards", cardHash+".webp")
	cachedFile, err := os.Open(cachedCardPath)
	if err != nil {
		service.logger.Warn("Failed to open cached social card", "path", cachedCardPath, "error", err)
		service.sink.Register(cardDestinationPath)
		return
	}
	defer func() {
		if err := cachedFile.Close(); err != nil {
			service.logger.Warn("Failed to close cached social card", "path", cachedCardPath, "error", err)
		}
	}()

	if err := service.sink.MkdirAll(filepath.Dir(cardDestinationPath)); err != nil {
		service.logger.Warn("Failed to create social card directory", "path", filepath.Dir(cardDestinationPath), "error", err)
	}

	err = service.sink.WriteStream(cardDestinationPath, func(writer io.Writer) error {
		_, err := io.Copy(writer, cachedFile)
		return err
	})
	if err != nil {
		service.logger.Warn("Failed to copy cached social card", "path", cardDestinationPath, "error", err)
		return
	}
	service.renderer.RegisterFile(cardDestinationPath)
}
