package generators

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildCtx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// TagSocialCardTask holds data for tag social card generation
type TagSocialCardTask struct {
	Slug  string
	Title string
	Count int
}

const maxTagSocialCardWorkers = 4

// BoundedTagSocialCardWorkers returns the number of workers for tag social card generation
func BoundedTagSocialCardWorkers() int {
	workers := max(runtime.NumCPU(), 1)
	if workers > maxTagSocialCardWorkers {
		workers = maxTagSocialCardWorkers
	}
	return workers
}

// TagOptions holds dependencies for tags rendering
type TagOptions struct {
	Ctx                context.Context
	Cfg                *config.Config
	Sink               fspkg.ArtifactSink
	Render             models.RenderService
	Cache              models.SocialCardCache
	SourceFs           afero.Fs
	TagMap             map[string][]models.PostMetadata
	ForceSocialRebuild bool
	LogoPath           string
}

// RenderTags orchestrates the generation of tag index and individual tag pages.
func RenderTags(opts TagOptions) error {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	var allTags []models.TagData
	for t, posts := range opts.TagMap {
		slug := timeutil.Slugify(t)
		allTags = append(allTags, models.TagData{Name: t, Count: len(posts), Link: fmt.Sprintf("/tags/%s.html", slug)})
	}
	sort.Slice(allTags, func(i, j int) bool { return allTags[i].Name < allTags[j].Name })

	workers := BoundedTagSocialCardWorkers()
	tagCardsTimer := timeutil.StartPhase("Tags social cards")
	tagCardPool := async.NewWorkerPool(opts.Ctx, workers, func(task TagSocialCardTask) error {
		tagCard := filepath.Join(cfg.OutputDir, fmt.Sprintf("static/images/cards/tags/%s.webp", task.Slug))

		ProvideSocialCard(ProvideSocialCardOptions{
			Sink:        sink,
			Cache:       opts.Cache,
			SourceFs:    opts.SourceFs,
			OutputDir:   cfg.OutputDir,
			CacheDir:    cfg.CacheDir,
			Title:       cfg.Title,
			DestPath:    tagCard,
			CacheKey:    "tags/" + task.Slug,
			CardTitle:   "#" + task.Title,
			Description: fmt.Sprintf("%d posts about %s", task.Count, task.Title),
			Badge:       "Topic",
			Force:       opts.ForceSocialRebuild,
			SocialCfg:   &cfg.SocialCards,
			Render:      render,
			LogoPath:    opts.LogoPath,
		})
		return nil
	})
	tagCardPool.Start()

	tagsDesc := fmt.Sprintf("Browse all %d topics", len(opts.TagMap))
	tagsIndexHash := SocialCardHash("All Topics", tagsDesc)
	tagsIndexCache := filepath.Join(cfg.CacheDir, "social-cards", tagsIndexHash+".webp")
	tagsIndexCard := filepath.Join(cfg.OutputDir, "static/images/cards/tags/index.webp")

	if ShouldGenerateSocialCard(opts.Cache, "tags/index", tagsIndexHash, tagsIndexCache, opts.ForceSocialRebuild) {
		ProvideSocialCard(ProvideSocialCardOptions{
			Sink:        sink,
			Cache:       opts.Cache,
			SourceFs:    opts.SourceFs,
			OutputDir:   cfg.OutputDir,
			CacheDir:    cfg.CacheDir,
			Title:       cfg.Title,
			DestPath:    tagsIndexCard,
			CacheKey:    "tags/index",
			CardTitle:   "All Topics",
			Description: tagsDesc,
			Badge:       "Topics",
			Force:       opts.ForceSocialRebuild,
			SocialCfg:   &cfg.SocialCards,
			Render:      render,
			LogoPath:    opts.LogoPath,
		})
	} else {
		// Use file reading helper if needed, or just standard os.ReadFile
		if data, err := afero.ReadFile(afero.NewOsFs(), tagsIndexCache); err == nil {
			buildCtx.IgnoreError(sink.MkdirAll(filepath.Dir(tagsIndexCard)), "create tags index card dir")
			buildCtx.IgnoreError(sink.WriteFile(tagsIndexCard, data), "write cached tags index card")
			render.RegisterFile(tagsIndexCard)
		}
	}

	// Generate Tags Index
	tagRenderTimer := timeutil.StartPhase("Tags HTML rendering")
	if err := render.RenderPage(filepath.Join(cfg.OutputDir, "tags/index.html"), models.PageData{
		Title: "All Tags", IsTagsIndex: true, AllTags: allTags,
		BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion,
		Permalink: cfg.BaseURL + "/tags/index.html",
		Image:     cfg.BaseURL + "/static/images/cards/tags/index.webp",
		TabTitle:  "All Topics | " + cfg.Title, Config: cfg,
		Weight:         0,
		RelativePrefix: "../",
	}); err != nil {
		return fmt.Errorf("failed to render tags index: %w", err)
	}

	g, _ := errgroup.WithContext(opts.Ctx)
	g.SetLimit(runtime.NumCPU())

	for t, posts := range opts.TagMap {
		tagName := t
		tagPosts := make([]models.PostMetadata, len(posts))
		copy(tagPosts, posts)
		slug := timeutil.Slugify(tagName)
		tagDesc := fmt.Sprintf("%d posts about %s", len(tagPosts), tagName)
		hash := SocialCardHash("#"+tagName, tagDesc)
		cached := filepath.Join(cfg.CacheDir, "social-cards", hash+".webp")

		if ShouldGenerateSocialCard(opts.Cache, "tags/"+slug, hash, cached, opts.ForceSocialRebuild) {
			tagCardPool.Submit(TagSocialCardTask{Slug: slug, Title: tagName, Count: len(tagPosts)})
		} else if data, err := afero.ReadFile(afero.NewOsFs(), cached); err == nil {
			tagCard := filepath.Join(cfg.OutputDir, fmt.Sprintf("static/images/cards/tags/%s.webp", slug))
			buildCtx.IgnoreError(sink.MkdirAll(filepath.Dir(tagCard)), "create tag card dir")
			buildCtx.IgnoreError(sink.WriteFile(tagCard, data), "write cached tag card")
			render.RegisterFile(tagCard)
		}

		g.Go(func() error {
			timeutil.SortPosts(tagPosts)
			if err := render.RenderPage(filepath.Join(cfg.OutputDir, fmt.Sprintf("tags/%s.html", slug)), models.PageData{
				Title: "#" + tagName, IsIndex: true, Posts: tagPosts,
				BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion,
				Permalink: fmt.Sprintf("%s/tags/%s.html", cfg.BaseURL, slug),
				Image:     fmt.Sprintf("%s/static/images/cards/tags/%s.webp", cfg.BaseURL, slug),
				TabTitle:  "#" + tagName + " | " + cfg.Title, Config: cfg,
				Weight:         0,
				RelativePrefix: "../",
			}); err != nil {
				return fmt.Errorf("failed to render tag page %s: %w", slug, err)
			}
			return nil
		})
	}
	err := g.Wait()
	tagRenderTimer.Stop()

	func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Debug("Tag card pool stop recovered", "panic", r)
			}
		}()
		buildCtx.IgnoreError(tagCardPool.Stop(), "stop tag card pool")
		tagCardsTimer.Stop()
	}()

	return err
}
