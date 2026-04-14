package benchmarks

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	buildctx "github.com/Kush-Singh-26/kosh/builder/context"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	mocks "github.com/Kush-Singh-26/kosh/builder/mocks/services"
	"github.com/Kush-Singh-26/kosh/builder/orchestration"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/pools"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/scheduler"
	"github.com/Kush-Singh-26/kosh/builder/services/post"
	"github.com/Kush-Singh-26/kosh/builder/services/render"
	"github.com/Kush-Singh-26/kosh/builder/services/scanner"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

func BenchmarkMarkdownParsing(b *testing.B) {
	cfg := &config.Config{}
	r := native.New()
	defer func() { _ = r.Close() }()
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := r.GetD2Singleflight()
	parser := mdParser.New(cfg,
		mdParser.WithRenderer(r),
		mdParser.WithDiagramCache(diagramCache),
		mdParser.WithD2Group(d2Group),
	)

	// Create a large markdown content (approx 10,000 words)
	word := "word "
	content := strings.Repeat(word, 10000)
	markdown := []byte("# Large Post\n\n" + content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pools.SharedBufferPool.Get()
		if err := parser.Convert(markdown, buf); err != nil {
			pools.SharedBufferPool.Put(buf)
			b.Fatal(err)
		}
		pools.SharedBufferPool.Put(buf)
	}
}

func BenchmarkFullBuild(b *testing.B) {
	// Setup a 100-post site
	fs := afero.NewMemMapFs()
	testutil.ScaffoldTestSite(fs)

	// Add 99 more posts
	for i := 1; i < 100; i++ {
		postContent := fmt.Sprintf(`---
title: "Post %d"
date: 2026-03-06
tags: ["test"]
---
# Post %d
This is post number %d.
`, i, i, i)
		_ = afero.WriteFile(fs, fmt.Sprintf("content/posts/post%d.md", i), []byte(postContent), 0644)
	}

	cfg := config.LoadFs(fs, []string{})
	cfg.OutputDir = "public"
	cfg.Features.Generators.IsSearchEnabled = true
	cfg.Features.Generators.IsRSSEnabled = true
	cfg.Features.Generators.IsSitemapEnabled = true

	logger := orchestration.InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	defer func() { _ = nativeRenderer.Close() }()
	diagramCache := mdParser.NewMemorySSRMap()
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg,
				mdParser.WithRenderer(nativeRenderer),
				mdParser.WithDiagramCache(diagramCache),
				mdParser.WithD2Group(d2Group),
			)
		},
	}

	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// We need fresh services for each run or at least reset them
		rnd := renderer.NewWithFs(renderer.RendererOptions{
			SourceFs:    fs,
			Compress:    false,
			Sink:        sink,
			TemplateDir: cfg.TemplateDir,
			DevMode:     true,
			Logger:      logger,
		})
		renderSvc := render.NewService(render.Dependencies{
			Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
				IsTesting:    true,
				IsDev:        false,
				IsCleanBuild: false,
				Scheduler:    scheduler.NewBuildScheduler(),
				Logger:       logger,
			}),
			Renderer: rnd,
			Logger:   logger,
		})
		assetSvc := &mocks.MockAssetService{}
		assetSvc.SetMetrics(buildMetrics)
		wasmSvc := &mocks.MockWasmService{}
		postSvc := post.NewService(post.Dependencies{
			Ctx: buildctx.NewBuildContext(buildctx.ContextOptions{
				IsTesting:    true,
				IsDev:        false,
				IsCleanBuild: false,
				Scheduler:    scheduler.NewBuildScheduler(),
				Logger:       logger,
			}),
			Cfg:            cfg,
			Renderer:       renderSvc,
			Logger:         logger,
			Metrics:        buildMetrics,
			MdPool:         mdPool,
			NativeRenderer: nativeRenderer,
		Fragments:      nil,
			SourceFs:       fs,
		})
		metadataScanner := scanner.NewScanner()

		engine := orchestration.NewEngine(orchestration.WithDeps(orchestration.EngineDependencies{
			Config:         cfg,
			Render:         renderSvc,
			Asset:          assetSvc,
			Post:           postSvc,
			Scanner:        metadataScanner,
			Wasm:           wasmSvc,
			Logger:         logger,
			Metrics:        buildMetrics,
			SourceFs:       fs,
			MdPool:         mdPool,
			NativeRenderer: nativeRenderer,
		Fragments:      nil,
		}))

		engine.SetArtifactSink(sink)
		engine.SetBuildTransaction(tx)

		if err := engine.Build(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
