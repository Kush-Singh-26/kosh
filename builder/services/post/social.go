package post

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/navigation"
)

func (s *postService) queueSocialCard(opts SocialCardOptions) {
	relPath := opts.RelPath
	res := opts.Result
	htmlRelPath := opts.HtmlRelPath
	force := opts.ForceSocialRebuild
	pool := opts.CardPool

	cardRelPath, cardDestPath, _ := navigation.CardPaths(s.cfg.BaseURL, s.cfg.OutputDir, htmlRelPath)

	cacheDir := s.cfg.CacheDir
	if !filepath.IsAbs(cacheDir) {
		abs, err := filepath.Abs(cacheDir)
		if err == nil {
			cacheDir = abs
		}
	}
	cachedCardPath := filepath.Join(cacheDir, "social-cards", res.FrontmatterHash+".webp")

	if _, err := os.Stat(cachedCardPath); err == nil && !force && res.FrontmatterHash != "" {
		s.copyCachedSocialCard(res.FrontmatterHash, cardDestPath)
	} else {
		pool.Submit(socialCardTask{
			path: relPath, relPath: cardRelPath,
			cardDestPath: cardDestPath, metadata: res.Metadata, frontmatterHash: res.FrontmatterHash,
		})
	}
}

func (s *postService) copyCachedSocialCard(cardHash, cardDestPath string) {
	if cardHash == "" {
		s.sink.Register(cardDestPath)
		return
	}
	cachedCardPath := filepath.Join(s.cfg.CacheDir, "social-cards", cardHash+".webp")
	cachedFile, err := os.Open(cachedCardPath)
	if err != nil {
		s.logger.Warn("Failed to open cached social card", "path", cachedCardPath, "error", err)
		s.sink.Register(cardDestPath)
		return
	}
	defer func() {
		if err := cachedFile.Close(); err != nil {
			s.logger.Warn("Failed to close cached social card", "path", cachedCardPath, "error", err)
		}
	}()

	if err := s.sink.MkdirAll(filepath.Dir(cardDestPath)); err != nil {
		s.logger.Warn("Failed to create social card directory", "path", filepath.Dir(cardDestPath), "error", err)
	}

	err = s.sink.WriteStream(cardDestPath, func(w io.Writer) error {
		_, err := io.Copy(w, cachedFile)
		return err
	})
	if err != nil {
		s.logger.Warn("Failed to copy cached social card", "path", cardDestPath, "error", err)
		return
	}
	s.renderer.RegisterFile(cardDestPath)
}
