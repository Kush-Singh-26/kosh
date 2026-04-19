package generators

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
	"github.com/spf13/afero"
	"golang.org/x/sync/errgroup"
)

// TaxonomySocialCardTask holds data for taxonomy term social card generation
type TaxonomySocialCardTask struct {
	Taxonomy string
	Plural   string
	Slug     string
	Title    string
	Count    int
}

const maxTaxonomySocialCardWorkers = 4

// BoundedTaxonomySocialCardWorkers returns the number of workers for taxonomy social card generation
func BoundedTaxonomySocialCardWorkers() int {
	workers := max(runtime.NumCPU(), 1)
	if workers > maxTaxonomySocialCardWorkers {
		workers = maxTaxonomySocialCardWorkers
	}
	return workers
}

// TaxonomyOptions holds dependencies for taxonomies rendering
type TaxonomyOptions struct {
	Ctx                context.Context
	Cfg                *config.Config
	Sink               fspkg.ArtifactSink
	Render             models.RenderService
	Cache              models.SocialCardCache
	SourceFs           afero.Fs
	TaxonomyMap        map[string]map[string][]models.ContentMetadata
	ForceSocialRebuild bool
	LogoPath           string
}

// BuildTaxonomyData builds a list of TermData from a term map for a specific taxonomy.
func BuildTaxonomyData(prefix, plural string, termMap map[string][]models.ContentMetadata) []models.TermData {
	allTerms := make([]models.TermData, 0, len(termMap))
	prefix = strings.Trim(prefix, "/")
	cleanPlural := strings.TrimPrefix(strings.Trim(plural, "/"), prefix+"/")
	for t, posts := range termMap {
		slug := timeutil.Slugify(t)
		link := fmt.Sprintf("/%s/%s.html", cleanPlural, slug)
		if prefix != "" {
			link = fmt.Sprintf("/%s/%s/%s.html", prefix, cleanPlural, slug)
		}
		allTerms = append(allTerms, models.TermData{
			Name:  t,
			Count: len(posts),
			Link:  link,
		})
	}
	sort.Slice(allTerms, func(i, j int) bool { return allTerms[i].Name < allTerms[j].Name })
	return allTerms
}

func startTaxonomyCardPool(opts TaxonomyOptions, workers int) *async.WorkerPool[TaxonomySocialCardTask] {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	pool := async.NewWorkerPool(opts.Ctx, workers, func(task TaxonomySocialCardTask) error {
		cardPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("static/images/cards/%s/%s.webp", task.Plural, task.Slug))
		ProvideSocialCard(ProvideSocialCardOptions{
			Sink:        sink,
			Cache:       opts.Cache,
			SourceFs:    opts.SourceFs,
			OutputDir:   cfg.OutputDir,
			CacheDir:    cfg.CacheDir,
			Title:       cfg.Title,
			DestPath:    cardPath,
			CacheKey:    fmt.Sprintf("%s/%s", task.Plural, task.Slug),
			CardTitle:   task.Title,
			Description: fmt.Sprintf("%d items in %s: %s", task.Count, task.Plural, task.Title),
			Badge:       task.Taxonomy,
			Force:       opts.ForceSocialRebuild,
			SocialCfg:   &cfg.SocialCards,
			Render:      render,
			LogoPath:    opts.LogoPath,
		})
		return nil
	})
	pool.Start()
	return pool
}

func ensureTaxonomyIndexCard(opts TaxonomyOptions, _ string, plural, desc string) {
	cfg := opts.Cfg
	sink := opts.Sink
	render := opts.Render

	title := fmt.Sprintf("All %s", plural)
	indexHash := SocialCardHash(title, desc)
	indexCache := filepath.Join(cfg.CacheDir, "social-cards", indexHash+".webp")
	indexCard := filepath.Join(cfg.OutputDir, fmt.Sprintf("static/images/cards/%s/index.webp", plural))

	if ShouldGenerateSocialCard(CheckSocialCardOptions{
		Cache:          opts.Cache,
		CacheKey:       fmt.Sprintf("%s/index", plural),
		CurrentHash:    indexHash,
		CachedCardPath: indexCache,
		Force:          opts.ForceSocialRebuild,
	}) {
		ProvideSocialCard(ProvideSocialCardOptions{
			Sink:        sink,
			Cache:       opts.Cache,
			SourceFs:    opts.SourceFs,
			OutputDir:   cfg.OutputDir,
			CacheDir:    cfg.CacheDir,
			Title:       cfg.Title,
			DestPath:    indexCard,
			CacheKey:    fmt.Sprintf("%s/index", plural),
			CardTitle:   title,
			Description: desc,
			Badge:       plural,
			Force:       opts.ForceSocialRebuild,
			SocialCfg:   &cfg.SocialCards,
			Render:      render,
			LogoPath:    opts.LogoPath,
		})
		return
	}

	if data, err := afero.ReadFile(afero.NewOsFs(), indexCache); err == nil {
		buildctx.IgnoreError(sink.MkdirAll(filepath.Dir(indexCard)), "create taxonomy index card dir")
		buildctx.IgnoreError(sink.WriteFile(indexCard, data), "write cached taxonomy index card")
		render.RegisterFile(indexCard)
	}
}

func renderTaxonomyIndex(cfg *config.Config, render models.RenderService, taxonomy, plural string, terms []models.TermData) error {
	prefix := strings.Trim(cfg.ContentPrefix, "/")
	// Normalize plural to not have the prefix if we are going to prepend it
	cleanPlural := strings.TrimPrefix(strings.Trim(plural, "/"), prefix+"/")

	indexPath := fmt.Sprintf("%s/index.html", cleanPlural)
	if prefix != "" {
		indexPath = fmt.Sprintf("%s/%s/index.html", prefix, cleanPlural)
	}
	sectionIndexURL := navigation.ResolveSectionIndex(indexPath)

	return render.RenderPage(filepath.Join(cfg.OutputDir, indexPath), models.PageData{
		Title: fmt.Sprintf("All %s", plural), IsTaxonomyIndex: true, Context: models.ContextSection,
		BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion,
		Permalink: cfg.BaseURL + "/" + indexPath,
		Image:     fmt.Sprintf("%s/static/images/cards/%s/index.webp", cfg.BaseURL, plural),
		TabTitle:  fmt.Sprintf("All %s | %s", plural, cfg.Title), Config: cfg,
		Taxonomies: map[string]models.TaxonomyData{
			taxonomy: {
				Name:   taxonomy,
				Plural: plural,
				Terms:  terms,
			},
		},
		Weight:         0,
		RelativePrefix: "../../", ContentPrefix: cfg.ContentPrefix,
		SectionIndexURL: sectionIndexURL,
	})
}

func renderTermPage(cfg *config.Config, render models.RenderService, taxonomy, plural, termName, slug string, items []models.ContentMetadata) error {
	timeutil.SortItemsByTaxonomy(taxonomy, items)
	prefix := strings.Trim(cfg.ContentPrefix, "/")
	// Normalize plural to not have the prefix if we are going to prepend it
	cleanPlural := strings.TrimPrefix(strings.Trim(plural, "/"), prefix+"/")

	termPath := fmt.Sprintf("%s/%s.html", cleanPlural, slug)
	if prefix != "" {
		termPath = fmt.Sprintf("%s/%s/%s.html", prefix, cleanPlural, slug)
	}
	sectionIndexURL := navigation.ResolveSectionIndex(termPath)
	return render.RenderPage(filepath.Join(cfg.OutputDir, termPath), models.PageData{
		Title: termName, IsIndex: true, Context: models.ContextSection, Items: items,
		BaseURL: cfg.BaseURL, BuildVersion: cfg.BuildVersion,
		Permalink: fmt.Sprintf("%s/%s", cfg.BaseURL, termPath),
		Image:     fmt.Sprintf("%s/static/images/cards/%s/%s.webp", cfg.BaseURL, plural, slug),
		TabTitle:  termName + " | " + cfg.Title, Config: cfg,
		Taxonomies: map[string]models.TaxonomyData{
			taxonomy: {
				Name:   taxonomy,
				Plural: plural,
				Terms:  BuildTaxonomyData(cfg.ContentPrefix, plural, map[string][]models.ContentMetadata{termName: items}),
			},
		},
		Weight:         0,
		RelativePrefix: "../../", ContentPrefix: cfg.ContentPrefix,
		SectionIndexURL: sectionIndexURL,
	})
}

// RenderTaxonomies orchestrates the generation of all taxonomy index and term pages.
func RenderTaxonomies(opts TaxonomyOptions) error {
	cardPool := startTaxonomyCardPool(opts, BoundedTaxonomySocialCardWorkers())
	defer buildctx.IgnoreError(cardPool.Stop(), "stop taxonomy card pool")

	g, _ := errgroup.WithContext(opts.Ctx)
	g.SetLimit(runtime.NumCPU())

	for taxonomy, plural := range opts.Cfg.Taxonomies {
		taxKey, taxPlural := taxonomy, plural
		termMap := opts.TaxonomyMap[taxKey]
		if termMap == nil {
			continue
		}

		if err := renderSingleTaxonomy(opts, taxKey, taxPlural, termMap, cardPool, g); err != nil {
			return err
		}
	}

	return g.Wait()
}

func renderSingleTaxonomy(opts TaxonomyOptions, taxKey, taxPlural string, termMap map[string][]models.ContentMetadata, cardPool *async.WorkerPool[TaxonomySocialCardTask], g *errgroup.Group) error {
	cfg := opts.Cfg
	allTerms := BuildTaxonomyData(cfg.ContentPrefix, taxPlural, termMap)

	desc := fmt.Sprintf("Browse all %d %s", lenAll(termMap), taxPlural)
	ensureTaxonomyIndexCard(opts, taxKey, taxPlural, desc)

	if err := renderTaxonomyIndex(cfg, opts.Render, taxKey, taxPlural, allTerms); err != nil {
		return fmt.Errorf("failed to render taxonomy index %s: %w", taxKey, err)
	}

	for t, posts := range termMap {
		termName := t
		termPosts := slices.Clone(posts)
		slug := timeutil.Slugify(termName)

		handleTaxonomySocialCard(opts, taxKey, taxPlural, termName, slug, len(termPosts), cardPool)

		g.Go(func() error {
			if err := renderTermPage(cfg, opts.Render, taxKey, taxPlural, termName, slug, termPosts); err != nil {
				return fmt.Errorf("failed to render term page %s/%s: %w", taxPlural, slug, err)
			}
			return nil
		})
	}
	return nil
}

func handleTaxonomySocialCard(opts TaxonomyOptions, taxKey, taxPlural, termName, slug string, count int, cardPool *async.WorkerPool[TaxonomySocialCardTask]) {
	cfg := opts.Cfg
	termDesc := fmt.Sprintf("%d items in %s: %s", count, taxPlural, termName)
	hash := SocialCardHash(termName, termDesc)
	cached := filepath.Join(cfg.CacheDir, "social-cards", hash+".webp")

	if ShouldGenerateSocialCard(CheckSocialCardOptions{
		Cache:          opts.Cache,
		CacheKey:       fmt.Sprintf("%s/%s", taxPlural, slug),
		CurrentHash:    hash,
		CachedCardPath: cached,
		Force:          opts.ForceSocialRebuild,
	}) {
		cardPool.Submit(TaxonomySocialCardTask{
			Taxonomy: taxKey,
			Plural:   taxPlural,
			Slug:     slug,
			Title:    termName,
			Count:    count,
		})
	} else if data, err := afero.ReadFile(afero.NewOsFs(), cached); err == nil {
		cardPath := filepath.Join(cfg.OutputDir, fmt.Sprintf("static/images/cards/%s/%s.webp", taxPlural, slug))
		buildctx.IgnoreError(opts.Sink.MkdirAll(filepath.Dir(cardPath)), "create taxonomy card dir")
		buildctx.IgnoreError(opts.Sink.WriteFile(cardPath, data), "write cached taxonomy card")
		opts.Render.RegisterFile(cardPath)
	}
}

func lenAll(m map[string][]models.ContentMetadata) int {
	return len(m)
}
