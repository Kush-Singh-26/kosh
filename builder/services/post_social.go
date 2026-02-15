package services

import (
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (s *postServiceImpl) generateSocialCard(t socialCardTask) {
	cachedCardPath := filepath.Join(s.cfg.CacheDir, "social-cards", t.frontmatterHash+".webp")

	cachedFile, err := os.Open(cachedCardPath)
	if err == nil && t.frontmatterHash != "" {
		defer func() {
			if cerr := cachedFile.Close(); cerr != nil {
				s.logger.Warn("Failed to close cached file", "path", cachedCardPath, "error", cerr)
			}
		}()
		out, err := s.destFs.Create(t.cardDestPath)
		if err == nil {
			defer func() {
				if cerr := out.Close(); cerr != nil {
					s.logger.Warn("Failed to close output file", "path", t.cardDestPath, "error", cerr)
				}
			}()
			if _, err := io.Copy(out, cachedFile); err == nil {
				if s.cache != nil {
					if err := s.cache.SetSocialCardHash(t.path, t.frontmatterHash); err != nil {
						s.logger.Warn("Failed to set social card hash in cache", "path", t.path, "error", err)
					}
				}
				s.renderer.RegisterFile(t.cardDestPath)
				s.logger.Debug("Social card copied from cache", "path", t.cardDestPath)
				return
			} else {
				s.logger.Warn("Failed to copy cached social card", "path", t.cardDestPath, "error", err)
			}
		} else {
			s.logger.Warn("Failed to create social card destination", "path", t.cardDestPath, "error", err)
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
		if data, err := os.ReadFile(cachedCardPath); err == nil {
			cardDir := filepath.ToSlash(filepath.Dir(t.cardDestPath))
			if err := s.destFs.MkdirAll(cardDir, 0755); err != nil {
				s.logger.Error("Failed to create social card directory", "path", cardDir, "error", err)
			}
			if err := afero.WriteFile(s.destFs, t.cardDestPath, data, 0644); err != nil {
				s.logger.Error("Failed to write social card", "path", t.cardDestPath, "error", err)
			} else {
				s.logger.Debug("Social card generated successfully", "path", t.cardDestPath)
				if _, err := s.destFs.Stat(t.cardDestPath); err != nil {
					s.logger.Error("Social card file not found in VFS after write", "path", t.cardDestPath, "error", err)
				} else {
					s.logger.Debug("Social card verified in VFS", "path", t.cardDestPath)
				}
				s.renderer.RegisterFile(t.cardDestPath)
			}
		} else {
			s.logger.Error("Failed to read generated social card from cache", "path", cachedCardPath, "error", err)
		}

		if s.cache != nil && t.frontmatterHash != "" {
			if err := s.cache.SetSocialCardHash(t.path, t.frontmatterHash); err != nil {
				s.logger.Warn("Failed to set social card hash in cache", "path", t.path, "error", err)
			}
		}
	} else {
		s.logger.Error("Failed to generate social card to disk", "path", cachedCardPath, "error", err)
		if err := generators.GenerateSocialCard(s.destFs, s.sourceFs, &s.cfg.SocialCards, s.cfg.Title,
			utils.GetString(t.metaData, "title"), utils.GetString(t.metaData, "description"),
			utils.GetString(t.metaData, "date"), t.cardDestPath, logoPath); err != nil {
			s.logger.Error("Failed to generate social card (fallback)", "path", t.cardDestPath, "error", err)
		} else {
			s.renderer.RegisterFile(t.cardDestPath)
			s.logger.Debug("Social card generated successfully (fallback)", "path", t.cardDestPath)
		}
	}
}
