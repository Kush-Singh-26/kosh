package generators

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

const (
	homeDescMaxLen      = 100
	homeDescEllipsis    = "..."
	defaultItemsPerPage = 10
	firstPageIndex      = 1
	secondPageIndex     = 2
)

// PaginationOptions holds dependencies for pagination rendering
type PaginationOptions struct {
	Ctx          context.Context
	Cfg          *config.Config
	Sink         models.ArtifactSink
	Render       models.RenderService
	Cache        models.SocialCardCache
	SourceFs     afero.Fs
	AllPosts     []models.ContentMetadata
	PinnedItems  []models.ContentMetadata
	Force        bool
	Logger       *slog.Logger
	LogoPath     string
	Taxonomies   map[string]models.TaxonomyData
	ShowcaseMath template.HTML
	ShowcaseD2   template.HTML
}

func ensureHomeSocialCard(opts PaginationOptions) {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	desc := cfg.Description
	if len(desc) > homeDescMaxLen {
		desc = desc[:homeDescMaxLen-len(homeDescEllipsis)] + homeDescEllipsis
	}
	homeHash := SocialCardHash(cfg.Title, desc, &cfg.SocialCards)
	homeCached := filepath.Join(cfg.CacheDir, "social-cards", homeHash+".webp")
	homeCardPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("static/images/cards/home.%s.webp", homeHash))

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

func resolveItemsPerPage(cfg *config.Config) int {
	if cfg.ItemsPerPage > 0 {
		return cfg.ItemsPerPage
	}
	return defaultItemsPerPage
}

func resolveTotalPages(totalItems, itemsPerPage int) int {
	totalPages := int(math.Ceil(float64(totalItems) / float64(itemsPerPage)))
	if totalPages == 0 {
		return 1
	}
	return totalPages
}

func pageWindow(pageIdx, itemsPerPage, totalItems int) (int, int) {
	start, end := (pageIdx-1)*itemsPerPage, pageIdx*itemsPerPage
	if end > totalItems {
		end = totalItems
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
		buildctx.IgnoreError(sink.MkdirAll(filepath.Dir(destPath)), "create pagination page directory")
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

	desc := cfg.Description
	if len(desc) > homeDescMaxLen {
		desc = desc[:homeDescMaxLen-len(homeDescEllipsis)] + homeDescEllipsis
	}
	homeHash := SocialCardHash(cfg.Title, desc, &cfg.SocialCards)
	homeImageURL := fmt.Sprintf("%s/static/images/cards/home.%s.webp", cfg.BaseURL, homeHash)

	allItems := opts.AllPosts

	// Filter out pinned items from the main listing — they are rendered
	// separately via PinnedItems in the bento grid on the first page.
	nonPinned := make([]models.ContentMetadata, 0, len(allItems))
	for _, item := range allItems {
		if !item.IsPinned {
			nonPinned = append(nonPinned, item)
		}
	}
	allItems = nonPinned

	itemsPerPage := resolveItemsPerPage(cfg)
	totalPages := resolveTotalPages(len(allItems), itemsPerPage)

	g, _ := errgroup.WithContext(opts.Ctx)
	g.SetLimit(runtime.NumCPU())

	for i := firstPageIndex; i <= totalPages; i++ {
		pageIdx := i
		g.Go(func() error {
			start, end := pageWindow(pageIdx, itemsPerPage, len(allItems))
			pageItems := allItems[start:end]
			destPath, permalink, relPath := pagePaths(cfg, sink, pageIdx)
			paginator := buildPaginator(cfg, pageIdx, totalPages)

			// Skip root pagination if root _index.md exists - it's rendered as a single page
			if pageIdx == firstPageIndex && (relPath == "" || relPath == "index.html" || relPath == "./") {
				rootIndexPath := filepath.Join(cfg.ContentDir, "_index.md")
				exists, _ := afero.Exists(opts.SourceFs, rootIndexPath)
				if exists {
					return nil
				}
			}

			var curPinned []models.ContentMetadata
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
				Title: cfg.Title, Items: pageItems, PinnedItems: curPinned,
				BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion, TabTitle: cfg.Title,
				Description: cfg.Description, Permalink: permalink, Image: homeImageURL,
				SocialHash: homeHash,
				Paginator:  paginator, Config: cfg, Context: context,
				RelativePrefix: relPrefix, ContentPrefix: cfg.ContentPrefix,
				SectionIndexURL: sectionIndexURL,
				Taxonomies:      opts.Taxonomies,
				SiteData:        cfg.SiteData,
				ShowcaseMath:    opts.ShowcaseMath,
				ShowcaseD2:      opts.ShowcaseD2,
			}); err != nil {
				return fmt.Errorf("failed to render index page %d: %w", pageIdx, err)
			}
			return nil
		})
	}
	return g.Wait()
}
