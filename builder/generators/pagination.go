package generators

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"

	"log/slog"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// PaginationOptions holds dependencies for pagination rendering
type PaginationOptions struct {
	Ctx         context.Context
	Cfg         *config.Config
	Sink        models.ArtifactSink
	Render      models.RenderService
	Cache       models.SocialCardCache
	SourceFs    afero.Fs
	AllPosts    []models.PostMetadata
	PinnedPosts []models.PostMetadata
	Force       bool
	Logger      *slog.Logger
	LogoPath    string
	AllTags     []models.TagData
}

// RenderPagination orchestrates the generation of paginated index pages.
func RenderPagination(opts PaginationOptions) error {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	homeCardPath := filepath.Join(cfg.OutputDir, "static/images/cards/home.webp")
	desc := cfg.Description
	if len(desc) > 100 {
		desc = desc[:97] + "..."
	}
	homeHash := SocialCardHash(cfg.Title, desc)
	homeCached := filepath.Join(cfg.CacheDir, "social-cards", homeHash+".webp")

	if ShouldGenerateSocialCard(opts.Cache, "home", homeHash, homeCached, opts.Force) {
		homeCardTimer := timeutil.StartPhase("Home social card")
		ProvideSocialCard(ProvideSocialCardOptions{
			Sink:        sink,
			Cache:       opts.Cache,
			SourceFs:    opts.SourceFs,
			OutputDir:   cfg.OutputDir,
			CacheDir:    cfg.CacheDir,
			Title:       cfg.Title,
			DestPath:    homeCardPath,
			CacheKey:    "home",
			CardTitle:   cfg.Title,
			Description: desc,
			Badge:       "Latest Posts",
			Force:       opts.Force,
			SocialCfg:   &cfg.SocialCards,
			Render:      render,
			LogoPath:    opts.LogoPath,
		})
		homeCardTimer.Stop()
	} else {
		if data, err := os.ReadFile(homeCached); err == nil {
			buildCtx.IgnoreError(sink.MkdirAll(filepath.Dir(homeCardPath)), "create home card dir")
			buildCtx.IgnoreError(sink.WriteFile(homeCardPath, data), "write cached home card")
			render.RegisterFile(homeCardPath)
		}
	}

	latestPosts := opts.AllPosts

	postsPerPage := cfg.PostsPerPage
	if postsPerPage <= 0 {
		postsPerPage = 10
	}
	totalPages := int(math.Ceil(float64(len(latestPosts)) / float64(postsPerPage)))
	if totalPages == 0 {
		totalPages = 1
	}

	g, _ := errgroup.WithContext(opts.Ctx)
	g.SetLimit(runtime.NumCPU())

	for i := 1; i <= totalPages; i++ {
		pageIdx := i
		g.Go(func() error {
			start, end := (pageIdx-1)*postsPerPage, pageIdx*postsPerPage
			if end > len(latestPosts) {
				end = len(latestPosts)
			}
			pagePosts := latestPosts[start:end]
			destPath, permalink := filepath.Join(cfg.OutputDir, "index.html"), cfg.BaseURL+"/"
			if pageIdx > 1 {
				destPath = filepath.Join(cfg.OutputDir, fmt.Sprintf("page/%d/index.html", pageIdx))
				permalink = fmt.Sprintf("%s/page/%d/", cfg.BaseURL, pageIdx)
				_ = sink.MkdirAll(filepath.Dir(destPath))
			}
			paginator := models.Paginator{
				CurrentPage: pageIdx,
				TotalPages:  totalPages,
				HasPrev:     pageIdx > 1,
				HasNext:     pageIdx < totalPages,
				FirstURL:    cfg.BaseURL + "/#latest",
				LastURL:     fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, totalPages),
			}
			if pageIdx > 2 {
				paginator.PrevURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, pageIdx-1)
			} else if pageIdx == 2 {
				paginator.PrevURL = cfg.BaseURL + "/#latest"
			}
			if pageIdx < totalPages {
				paginator.NextURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, pageIdx+1)
			}
			var curPinned []models.PostMetadata
			if pageIdx == 1 {
				curPinned = opts.PinnedPosts
			}

			relPath := "index.html"
			if pageIdx > 1 {
				relPath = fmt.Sprintf("page/%d/index.html", pageIdx)
			}

			if err := render.RenderIndex(destPath, models.PageData{
				Title: cfg.Title, Posts: pagePosts, PinnedPosts: curPinned,
				BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion, TabTitle: cfg.Title,
				Description: cfg.Description, Permalink: permalink, Image: cfg.BaseURL + "/static/images/cards/home.webp",
				Paginator: paginator, Config: cfg,
				RelativePrefix: fspkg.GetRelativePrefix(relPath),
				AllTags:        opts.AllTags,
			}); err != nil {
				return fmt.Errorf("failed to render index page %d: %w", pageIdx, err)
			}
			return nil
		})
	}
	return g.Wait()
}
