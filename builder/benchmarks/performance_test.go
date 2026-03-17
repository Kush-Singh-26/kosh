package benchmarks

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/metrics"
	"github.com/Kush-Singh-26/kosh/builder/mocks/services"
	mdParser "github.com/Kush-Singh-26/kosh/builder/parser"
	"github.com/Kush-Singh-26/kosh/builder/renderer"
	"github.com/Kush-Singh-26/kosh/builder/renderer/native"
	"github.com/Kush-Singh-26/kosh/builder/run"
	"github.com/Kush-Singh-26/kosh/builder/services"
	"github.com/Kush-Singh-26/kosh/builder/testutil"
	"github.com/spf13/afero"
)

func BenchmarkMarkdownParsing(b *testing.B) {
	cfg := &config.Config{}
	r := native.New()
	defer func() { _ = r.Close() }()
	diagramCache := &sync.Map{}
	d2Group := r.GetD2Singleflight()
	parser := mdParser.New(cfg, r, diagramCache, d2Group)

	// Create a large markdown content (approx 10,000 words)
	word := "word "
	content := strings.Repeat(word, 10000)
	markdown := []byte("# Large Post\n\n" + content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		if err := parser.Convert(markdown, &buf); err != nil {
			b.Fatal(err)
		}
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
	cfg.Features.Generators.Search = true
	cfg.Features.Generators.RSS = true
	cfg.Features.Generators.Sitemap = true

	logger := run.InitLogger()
	buildMetrics := metrics.NewBuildMetrics()
	nativeRenderer := native.New()
	defer func() { _ = nativeRenderer.Close() }()
	diagramCache := &sync.Map{}
	d2Group := nativeRenderer.GetD2Singleflight()
	mdPool := &sync.Pool{
		New: func() any {
			return mdParser.New(cfg, nativeRenderer, diagramCache, d2Group)
		},
	}

	sink := testutil.NewMemSink()
	tx := testutil.NewMockTransaction("public")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// We need fresh services for each run or at least reset them
		rnd := renderer.NewWithFs(fs, false, sink, cfg.TemplateDir, true, logger)
		renderSvc := services.NewRenderServiceWith(rnd, logger)
		assetSvc := &mocks.MockAssetService{}
		assetSvc.SetMetrics(buildMetrics)
		wasmSvc := &mocks.MockWasmService{}
		postSvc := services.NewPostService(services.PostServiceDependencies{
			Cfg:            cfg,
			Renderer:       renderSvc,
			Logger:         logger,
			Metrics:        buildMetrics,
			MdPool:         mdPool,
			NativeRenderer: nativeRenderer,
			SourceFs:       fs,
		})
		metadataScanner := services.NewMetadataScanner()

		builder := run.NewBuilderFromManual(cfg, renderSvc, assetSvc, postSvc, metadataScanner, wasmSvc, logger, buildMetrics, fs, mdPool, nativeRenderer)

		builder.Sink = sink
		builder.Tx = tx

		if err := builder.Build(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
