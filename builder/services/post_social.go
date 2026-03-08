package services

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (s *postServiceImpl) generateSocialCard(t socialCardTask) {
	cachedCardPath := filepath.Join(s.cfg.CacheDir, "social-cards", t.frontmatterHash+".webp")
	if err := os.MkdirAll(filepath.Dir(cachedCardPath), 0755); err != nil {
		s.logger.Warn("Failed to create social card cache directory", "path", filepath.Dir(cachedCardPath), "error", err)
	}

	cachedFile, err := os.Open(cachedCardPath)
	if err == nil && t.frontmatterHash != "" {
		defer func() {
			if cerr := cachedFile.Close(); cerr != nil {
				s.logger.Warn("Failed to close cached file", "path", cachedCardPath, "error", cerr)
			}
		}()
		errWrite := s.sink.WriteStream(t.cardDestPath, func(w io.Writer) error {
			_, err := io.Copy(w, cachedFile)
			return err
		})
		if errWrite == nil {
			if s.cache != nil {
				s.cardHashes.Store(t.path, t.frontmatterHash)
			}
			s.renderer.RegisterFile(t.cardDestPath)
			s.logger.Debug("Social card copied from cache", "path", t.cardDestPath)
			return
		} else {
			s.logger.Warn("Failed to copy cached social card", "path", t.cardDestPath, "error", errWrite)
		}
	}

	logoPath := ""
	if s.cfg.Logo != "" {
		logoPath = s.cfg.Logo
	} else {
		logoPath = filepath.Join(s.cfg.ThemeDir, s.cfg.Theme, "static", "images", "favicon.png")
	}

	if logoPath != "" {
		if _, err := s.sourceFs.Stat(logoPath); err != nil {
			s.logger.Warn("Logo/favicon not found, social card may not render correctly", "path", logoPath, "error", err)
			logoPath = ""
		}
	}

	err = generators.GenerateSocialCardToDisk(s.sourceFs, &s.cfg.SocialCards, s.cfg.Title,
		utils.GetString(t.metaData, "title"), utils.GetString(t.metaData, "description"),
		utils.GetString(t.metaData, "date"), cachedCardPath, logoPath)

	if err == nil {
		cardDir := filepath.ToSlash(filepath.Dir(t.cardDestPath))
		if err := s.sink.MkdirAll(cardDir); err != nil {
			s.logger.Error("Failed to create social card directory", "path", cardDir, "error", err)
		}
		// Read the generated card once and write to Sink
		data, err := os.ReadFile(cachedCardPath)
		if err != nil {
			s.logger.Error("Failed to read generated social card from cache", "path", cachedCardPath, "error", err)
		} else if err := s.sink.WriteFile(t.cardDestPath, data); err != nil {
			s.logger.Error("Failed to write social card", "path", t.cardDestPath, "error", err)
		} else {
			s.logger.Debug("Social card generated successfully", "path", t.cardDestPath)
			s.renderer.RegisterFile(t.cardDestPath)
		}

		if s.cache != nil && t.frontmatterHash != "" {
			s.cardHashes.Store(t.path, t.frontmatterHash)
		}
	} else {
		s.logger.Error("Failed to generate social card to disk", "path", cachedCardPath, "error", err)
		if err := generators.GenerateSocialCard(s.sink, s.sourceFs, &s.cfg.SocialCards, s.cfg.Title,
			utils.GetString(t.metaData, "title"), utils.GetString(t.metaData, "description"),
			utils.GetString(t.metaData, "date"), t.cardDestPath, logoPath); err != nil {
			s.logger.Error("Failed to generate social card (fallback)", "path", t.cardDestPath, "error", err)
		} else {
			s.renderer.RegisterFile(t.cardDestPath)
			s.logger.Debug("Social card generated successfully (fallback)", "path", t.cardDestPath)
		}
	}
}
