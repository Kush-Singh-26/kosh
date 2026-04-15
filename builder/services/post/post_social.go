package post

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

const socialCardCacheDirMode = 0755

func (service *postService) generateSocialCard(task socialCardTask) {
	cachedCardPath := filepath.Join(service.cfg.CacheDir, "social-cards", task.frontmatterHash+".webp")
	if err := os.MkdirAll(filepath.Dir(cachedCardPath), socialCardCacheDirMode); err != nil {
		service.logger.Warn("Failed to create social card cache directory", "path", filepath.Dir(cachedCardPath), "error", err)
	}

	cachedFile, err := os.Open(cachedCardPath)
	if err == nil && task.frontmatterHash != "" {
		defer func() {
			if closeErr := cachedFile.Close(); closeErr != nil {
				service.logger.Warn("Failed to close cached file", "path", cachedCardPath, "error", closeErr)
			}
		}()
		errWrite := service.sink.WriteStream(task.cardDestPath, func(writer io.Writer) error {
			_, err := io.Copy(writer, cachedFile)
			return err
		})
		if errWrite != nil {
			service.logger.Warn("Failed to copy cached social card", "path", task.cardDestPath, "error", errWrite)
			return
		}
		if service.cache != nil {
			_ = service.cache.SetSocialCardHash(task.path, task.frontmatterHash)
		}
		service.renderer.RegisterFile(task.cardDestPath)
	}

	logoPath := service.cfg.Logo

	if logoPath != "" {
		if _, err := service.sourceFs.Stat(logoPath); err != nil {
			service.logger.Warn("Logo not found, social card may not render correctly", "path", logoPath, "error", err)
			logoPath = ""
		}
	}

	err = generators.GenerateSocialCardToDisk(generators.SocialCardOptions{
		SrcFs:       service.sourceFs,
		Cfg:         &service.cfg.SocialCards,
		SiteTitle:   service.cfg.Title,
		Title:       timeutil.ExtractStringFromMap(task.metadata, "title"),
		Description: timeutil.ExtractStringFromMap(task.metadata, "description"),
		DateStr:     timeutil.ExtractStringFromMap(task.metadata, "date"),
		DestPath:    cachedCardPath,
		LogoPath:    logoPath,
	})

	if err == nil {
		cardDir := filepath.ToSlash(filepath.Dir(task.cardDestPath))
		if err := service.sink.MkdirAll(cardDir); err != nil {
			service.logger.Error("Failed to create social card directory", "path", cardDir, "error", err)
		}
		// Read the generated card once and write to Sink
		data, err := os.ReadFile(cachedCardPath)
		if err != nil {
			service.logger.Error("Failed to read generated social card from cache", "path", cachedCardPath, "error", err)
		} else if err := service.sink.WriteFile(task.cardDestPath, data); err != nil {
			service.logger.Error("Failed to write social card", "path", task.cardDestPath, "error", err)
		} else {
			service.renderer.RegisterFile(task.cardDestPath)
		}

		if service.cache != nil && task.frontmatterHash != "" {
			_ = service.cache.SetSocialCardHash(task.path, task.frontmatterHash)
		}
	} else {
		service.logger.Error("Failed to generate social card to disk", "path", cachedCardPath, "error", err)
		if err := generators.GenerateSocialCard(generators.SocialCardOptions{
			Sink:        service.sink,
			SrcFs:       service.sourceFs,
			Cfg:         &service.cfg.SocialCards,
			SiteTitle:   service.cfg.Title,
			Title:       timeutil.ExtractStringFromMap(task.metadata, "title"),
			Description: timeutil.ExtractStringFromMap(task.metadata, "description"),
			DateStr:     timeutil.ExtractStringFromMap(task.metadata, "date"),
			DestPath:    task.cardDestPath,
			LogoPath:    logoPath,
		}); err != nil {
			service.logger.Error("Failed to generate social card (fallback)", "path", task.cardDestPath, "error", err)
		} else {
			service.renderer.RegisterFile(task.cardDestPath)
		}
	}
}
