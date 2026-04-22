package content

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
)

func (service *contentService) queueSocialCard(options SocialCardOptions) {
	relativePath := options.RelativePath
	result := options.Result
	htmlRelativePath := options.HTMLRelativePath
	shouldForce := options.ForceSocialRebuild
	cardPool := options.CardPool

	description, _ := result.Metadata["description"].(string)
	if description == "" {
		description = result.Item.Description
	}

	layoutVal := strings.ToLower(result.Item.Section)
	if l, ok := result.Metadata["layout"].(string); ok {
		layoutVal = strings.ToLower(l)
	}
	pageContext := models.ContextSection
	if layoutVal == "home" {
		pageContext = models.ContextHome
	}
	seoTitle := service.resolveSEOTitle(result.Metadata, result.Item, pageContext)

	currentHash := generators.SocialCardHash(seoTitle, description, &service.cfg.SocialCards)
	cardRelativePath, cardDestinationPath, _ := navigation.CardPaths(service.cfg.BaseURL, service.cfg.OutputDir, htmlRelativePath, currentHash)

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
