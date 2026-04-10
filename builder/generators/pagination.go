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
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

const (
	homeDescMaxLen      = 100
	homeDescEllipsis    = "..."
	defaultPostsPerPage = 10
	firstPageIndex      = 1
	secondPageIndex     = 2
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

func ensureHomeSocialCard(opts PaginationOptions) {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	homeCardPath := filepath.Join(cfg.OutputDir, "static/images/cards/home.webp")
	desc := cfg.Description
	if len(desc) > homeDescMaxLen {
		desc = desc[:homeDescMaxLen-len(homeDescEllipsis)] + homeDescEllipsis
	}
	homeHash := SocialCardHash(cfg.Title, desc)
	homeCached := filepath.Join(cfg.CacheDir, "social-cards", homeHash+".webp")

	if ShouldGenerateSocialCard(CheckSocialCardOptions{
		Cache:          opts.Cache,
		CacheKey:       "home",
		CurrentHash:    homeHash,
		CachedCardPath: homeCached,
		Force:          opts.Force,
	}) {
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
		return
	}

	if data, err := os.ReadFile(homeCached); err == nil {
		buildctx.IgnoreError(sink.MkdirAll(filepath.Dir(homeCardPath)), "create home card dir")
		buildctx.IgnoreError(sink.WriteFile(homeCardPath, data), "write cached home card")
		render.RegisterFile(homeCardPath)
	}
}

func resolvePostsPerPage(cfg *config.Config) int {
	if cfg.PostsPerPage <= 0 {
		return defaultPostsPerPage
	}
	return cfg.PostsPerPage
}

func resolveTotalPages(totalPosts, postsPerPage int) int {
	totalPages := int(math.Ceil(float64(totalPosts) / float64(postsPerPage)))
	if totalPages == 0 {
		return 1
	}
	return totalPages
}

func pageWindow(pageIdx, postsPerPage, totalPosts int) (int, int) {
	start, end := (pageIdx-1)*postsPerPage, pageIdx*postsPerPage
	if end > totalPosts {
		end = totalPosts
	}
	return start, end
}

func pagePaths(cfg *config.Config, sink models.ArtifactSink, pageIdx int) (string, string, string) {
	destPath, permalink := filepath.Join(cfg.OutputDir, "index.html"), cfg.BaseURL+"/"
	relPath := "index.html"
	if pageIdx > firstPageIndex {
		destPath = filepath.Join(cfg.OutputDir, fmt.Sprintf("page/%d/index.html", pageIdx))
		permalink = fmt.Sprintf("%s/page/%d/", cfg.BaseURL, pageIdx)
		relPath = fmt.Sprintf("page/%d/index.html", pageIdx)
		_ = sink.MkdirAll(filepath.Dir(destPath))
	}
	return destPath, permalink, relPath
}

func buildPaginator(cfg *config.Config, pageIdx, totalPages int) models.Paginator {
	paginator := models.Paginator{
		CurrentPage: pageIdx,
		TotalPages:  totalPages,
		HasPrev:     pageIdx > firstPageIndex,
		HasNext:     pageIdx < totalPages,
		FirstURL:    cfg.BaseURL + "/#latest",
		LastURL:     fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, totalPages),
	}
	if pageIdx > secondPageIndex {
		paginator.PrevURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, pageIdx-1)
	} else if pageIdx == secondPageIndex {
		paginator.PrevURL = cfg.BaseURL + "/#latest"
	}
	if pageIdx < totalPages {
		paginator.NextURL = fmt.Sprintf("%s/page/%d/#latest", cfg.BaseURL, pageIdx+1)
	}
	return paginator
}

// RenderPagination orchestrates the generation of paginated index pages.
func RenderPagination(opts PaginationOptions) error {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	ensureHomeSocialCard(opts)

	latestPosts := opts.AllPosts

	postsPerPage := resolvePostsPerPage(cfg)
	totalPages := resolveTotalPages(len(latestPosts), postsPerPage)

	g, _ := errgroup.WithContext(opts.Ctx)
	g.SetLimit(runtime.NumCPU())

	for i := firstPageIndex; i <= totalPages; i++ {
		pageIdx := i
		g.Go(func() error {
			start, end := pageWindow(pageIdx, postsPerPage, len(latestPosts))
			pagePosts := latestPosts[start:end]
			destPath, permalink, relPath := pagePaths(cfg, sink, pageIdx)
			paginator := buildPaginator(cfg, pageIdx, totalPages)
			var curPinned []models.PostMetadata
			if pageIdx == firstPageIndex {
				curPinned = opts.PinnedPosts
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
