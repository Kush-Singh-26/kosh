package content

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

const socialCardCacheDirMode = 0755

func (service *contentService) resolveSEOTitle(metadata map[string]any, item models.ContentMetadata, pageContext models.PageContext) string {
	// 1. Check for explicit SEO overrides
	if seoTitle, ok := metadata["seo_title"].(string); ok && seoTitle != "" {
		return seoTitle
	}
	if metaTitle, ok := metadata["meta_title"].(string); ok && metaTitle != "" {
		return metaTitle
	}

	// 2. Special case for home page identity
	if pageContext == models.ContextHome {
		return service.cfg.Title
	}

	// 3. Fallback to page title
	if title, ok := metadata["title"].(string); ok && title != "" {
		return title
	}
	return item.Title
}

func (service *contentService) generateSocialCard(task socialCardTask) {
	cachedCardPath := filepath.Join(service.cfg.CacheDir, "social-cards", task.frontmatterHash+".webp")
	if service.loadSocialCardFromCache(task, cachedCardPath) {
		return
	}

	logoPath := service.getLogoPath()
	opts := generators.SocialCardOptions{
		SrcFs:       service.sourceFs,
		Cfg:         &service.cfg.SocialCards,
		SiteTitle:   service.cfg.Title,
		Title:       task.seoTitle,
		Description: timeutil.ExtractStringFromMap(task.metadata, "description"),
		DateStr:     timeutil.ExtractDateStringFromMap(task.metadata, "date"),
		DestPath:    cachedCardPath,
		LogoPath:    logoPath,
	}

	if err := generators.GenerateSocialCardToDisk(opts); err == nil {
		service.saveSocialCardToCache(task, cachedCardPath)
	} else {
		service.logger.Error("Failed to generate social card to disk", "path", cachedCardPath, "error", err)
		service.generateSocialCardFallback(task, opts)
	}
}

func (service *contentService) loadSocialCardFromCache(task socialCardTask, cachedCardPath string) bool {
	if task.frontmatterHash == "" {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(cachedCardPath), socialCardCacheDirMode); err != nil {
		service.logger.Warn("Failed to create social card cache directory", "path", filepath.Dir(cachedCardPath), "error", err)
	}

	cachedFile, err := os.Open(cachedCardPath)
	if err != nil {
		return false
	}
	defer func() { _ = cachedFile.Close() }()

	errWrite := service.sink.WriteStream(task.cardDestPath, func(writer io.Writer) error {
		_, err := io.Copy(writer, cachedFile)
		return err
	})
	if errWrite != nil {
		service.logger.Warn("Failed to copy cached social card", "path", task.cardDestPath, "error", errWrite)
		return false
	}

	if service.cache != nil {
		_ = service.cache.SetSocialCardHash(task.path, task.frontmatterHash)
	}
	service.renderer.RegisterFile(task.cardDestPath)
	return true
}

func (service *contentService) getLogoPath() string {
	logoPath := service.cfg.Logo
	if logoPath != "" {
		if _, err := service.sourceFs.Stat(logoPath); err != nil {
			service.logger.Warn("Logo not found, social card may not render correctly", "path", logoPath, "error", err)
			return ""
		}
	}
	return logoPath
}

func (service *contentService) saveSocialCardToCache(task socialCardTask, cachedCardPath string) {
	cardDir := filepath.ToSlash(filepath.Dir(task.cardDestPath))
	if err := service.sink.MkdirAll(cardDir); err != nil {
		service.logger.Error("Failed to create social card directory", "path", cardDir, "error", err)
	}
	data, err := os.ReadFile(cachedCardPath)
	if err != nil {
		service.logger.Error("Failed to read generated social card from cache", "path", cachedCardPath, "error", err)
		return
	}
	if err := service.sink.WriteFile(task.cardDestPath, data); err != nil {
		service.logger.Error("Failed to write social card", "path", task.cardDestPath, "error", err)
		return
	}
	service.renderer.RegisterFile(task.cardDestPath)
	if service.cache != nil && task.frontmatterHash != "" {
		_ = service.cache.SetSocialCardHash(task.path, task.frontmatterHash)
	}
}

func (service *contentService) generateSocialCardFallback(task socialCardTask, opts generators.SocialCardOptions) {
	opts.Sink = service.sink
	opts.DestPath = task.cardDestPath
	if err := generators.GenerateSocialCard(opts); err != nil {
		service.logger.Error("Failed to generate social card (fallback)", "path", task.cardDestPath, "error", err)
	} else {
		service.renderer.RegisterFile(task.cardDestPath)
	}
}
