package generators

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
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
	PinnedItems []models.PostMetadata
	Force       bool
	Logger      *slog.Logger
	LogoPath    string
	Taxonomies  map[string]models.TaxonomyData
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
			Badge:       cfg.GetHomeBadge(),
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
	// For now, pagination remains at the root or content prefix as configured.
	// In the future, every section will have its own pagination.
	prefix := strings.Trim(cfg.ContentPrefix, "/")
	if prefix != "" {
		prefix = "/" + prefix
	}

	destPath, permalink := filepath.Join(cfg.OutputDir, prefix, "index.html"), cfg.BaseURL+prefix+"/"
	relPath := filepath.Join(prefix, "index.html")

	if pageIdx > firstPageIndex {
		destPath = filepath.Join(cfg.OutputDir, prefix, fmt.Sprintf("page/%d/index.html", pageIdx))
		permalink = fmt.Sprintf("%s%s/page/%d/", cfg.BaseURL, prefix, pageIdx)
		relPath = filepath.Join(prefix, fmt.Sprintf("page/%d/index.html", pageIdx))
		_ = sink.MkdirAll(filepath.Dir(destPath))
	}
	return destPath, permalink, filepath.ToSlash(relPath)
}

func buildPaginator(cfg *config.Config, pageIdx, totalPages int) models.Paginator {
	prefix := strings.Trim(cfg.ContentPrefix, "/")
	if prefix != "" {
		prefix = "/" + prefix
	}

	paginator := models.Paginator{
		CurrentPage: pageIdx,
		TotalPages:  totalPages,
		HasPrev:     pageIdx > firstPageIndex,
		HasNext:     pageIdx < totalPages,
		FirstURL:    cfg.BaseURL + prefix + "/#latest",
		LastURL:     fmt.Sprintf("%s%s/page/%d/#latest", cfg.BaseURL, prefix, totalPages),
	}
	if pageIdx > secondPageIndex {
		paginator.PrevURL = fmt.Sprintf("%s%s/page/%d/#latest", cfg.BaseURL, prefix, pageIdx-1)
	} else if pageIdx == secondPageIndex {
		paginator.PrevURL = cfg.BaseURL + prefix + "/#latest"
	}
	if pageIdx < totalPages {
		paginator.NextURL = fmt.Sprintf("%s%s/page/%d/#latest", cfg.BaseURL, prefix, pageIdx+1)
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
				curPinned = opts.PinnedItems
			}

			context := models.ContextSection
			// If this is the main site index (root), use Home context
			if pageIdx == firstPageIndex && (relPath == "" || relPath == "index.html" || relPath == "./") {
				context = models.ContextHome
			}

			relPrefix := fspkg.GetRelativePrefix(relPath)
			sectionIndexURL := navigation.ResolveSectionIndex(relPath)
			if err := render.RenderIndex(destPath, models.PageData{
				Title: cfg.Title, Posts: pagePosts, PinnedItems: curPinned,
				BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion, TabTitle: cfg.Title,
				Description: cfg.Description, Permalink: permalink, Image: cfg.BaseURL + "/static/images/cards/home.webp",
				Paginator: paginator, Config: cfg, Context: context,
				RelativePrefix: relPrefix, ContentPrefix: cfg.ContentPrefix,
				SectionIndexURL: sectionIndexURL,
				Taxonomies:   opts.Taxonomies,
				SiteData:     cfg.SiteData,
			}); err != nil {
				return fmt.Errorf("failed to render index page %d: %w", pageIdx, err)
			}
			return nil
		})
	}
	return g.Wait()
}
