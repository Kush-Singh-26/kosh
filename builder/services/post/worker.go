package post

import (
	"path/filepath"
	"strings"

	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
)

func (s *postService) parseWorkerTaskLocal(f models.ScannedFile, wCtx WorkerContext, local *workerLocalState) {
	path := f.Path
	relPath := f.RelPath

	htmlRelPath, _, destPath := navigation.ComputePathVars(s.cfg.OutputDir, relPath)

	// 1. Check Cache
	cachedMeta, useCache := s.checkCache(relPath, f, wCtx.ShouldForce)

	var parseRes *ParsedMarkdownResult
	var htmlContent string
	var finalSSRHashes []string

	if useCache {
		parseRes, htmlContent, useCache = s.loadFromCache(cachedMeta, htmlRelPath)
		if useCache {
			finalSSRHashes = cachedMeta.SSRInputHashes
			if s.metrics != nil {
				s.metrics.IncrementCacheHit()
			}
		}
	}

	// 2. Math processing from cache (only if we have cached HTML content)
	if useCache && htmlContent != "" && len(parseRes.MathExpressions) > 0 {
		var mathOk bool
		htmlContent, mathOk = s.processCachedMath(htmlContent, parseRes.MathExpressions)
		useCache = useCache && mathOk
	}

	// 3. Full Parse if needed
	var sourceBytes []byte
	var err error

	if !useCache || s.cfg.Features.RawMarkdown {
		if f.SourceLoader != nil {
			sourceBytes, err = f.SourceLoader()
			if err != nil {
				s.logger.Error("Failed to load source", "path", path, "error", err)
				local.errs = append(local.errs, err)
				return
			}
		}
	}

	if !useCache {
		if s.metrics != nil {
			s.metrics.IncrementCacheMiss()
		}

		readingTime := f.ReadingTime
		if cachedMeta != nil && cachedMeta.BodyHash == f.BodyHash && cachedMeta.ReadingTime > 0 {
			readingTime = cachedMeta.ReadingTime
		}

		parseRes, err = ParseMarkdown(ParseOptions{
			Path:                 path,
			RelPath:              relPath,
			Source:               sourceBytes,
			Info:                 f.Info,
			Renderer:             s.renderer,
			NativeRenderer:       s.nativeRenderer,
			MdPool:               s.mdPool,
			DiagramAdapter:       s.diagramAdapter,
			Metrics:              s.metrics,
			Cfg:                  s.cfg,
			CleanHtmlRelPath:     htmlRelPath,
			HtmlRelPath:          htmlRelPath,
			KnownFrontmatterHash: f.FrontmatterHash,
			KnownReadingTime:     readingTime,
			BodyOffset:           f.BodyOffset,
			PreParsedMeta:        f.PreParsedMeta,
		})
		if err != nil {
			s.logger.Error("Failed to parse markdown", "path", path, "error", err)
			local.errs = append(local.errs, err)
			return
		}
		local.anyChanged = true

		if parseRes.Post.Title == "" {
			parseRes.Post.Title = f.Title
		}

		htmlContent = s.renderMath(wCtx.Ctx, path, parseRes)
		finalSSRHashes = parseRes.SSRHashes
	}

	post := parseRes.Post
	if post.Draft && !s.cfg.IncludeDrafts {
		return
	}

	// 4. Social Card
	s.queueSocialCard(SocialCardOptions{
		RelPath:            relPath,
		Result:             parseRes,
		HtmlRelPath:        htmlRelPath,
		ForceSocialRebuild: wCtx.ForceSocialRebuild,
		CardPool:           wCtx.CardPool,
	})

	// 5. Aggregate and stream
	s.aggregateLocal(AggregateContext{
		ScannedFile: f,
		Res:         parseRes,
		Post:        post,
		HtmlContent: htmlContent,
		DestPath:    destPath,
		RelPath:     relPath,
		HtmlRelPath: htmlRelPath,
		SSRHashes:   finalSSRHashes,
		UseCache:    useCache,
		WCtx:        wCtx,
		Local:       local,
		SourceBytes: sourceBytes,
	})
	if s.metrics != nil {
		s.metrics.IncrementPostsProcessed()
	}
}

func (s *postService) aggregateLocal(ac AggregateContext) {
	f := ac.ScannedFile
	res := ac.Res
	post := ac.Post
	htmlContent := ac.HtmlContent
	destPath := ac.DestPath
	relPath := ac.RelPath
	htmlRelPath := ac.HtmlRelPath
	ssrHashes := ac.SSRHashes
	useCache := ac.UseCache
	wCtx := ac.WCtx
	local := ac.Local
	sourceBytes := ac.SourceBytes

	searchRecord := res.SearchRecord
	searchRecord.ID = xxh3.HashString(searchRecord.Link)

	localIdx := len(local.indexedPosts)
	local.indexedPosts = append(local.indexedPosts, models.IndexedPost{
		Record: searchRecord, WordFreqs: res.WordFreqs, DocLen: res.DocLen,
		StemMap: res.StemMap, PositionalIndex: res.PositionalIndex, ByteOffsets: res.ByteOffsets,
	})

	if !useCache && s.cache != nil {
		newSearch := &models.SearchRecord{
			Title: post.Title, NormalizedTitle: res.SearchRecord.NormalizedTitle,
			Content: res.SearchRecord.Content, NormalizedTags: res.SearchRecord.NormalizedTags,
		}
		postID := cache.GeneratePostID("", relPath)
		local.newSearchRecs[postID] = newSearch

		if s.cfg.Features.Generators.Search {
			local.searchTasks = append(local.searchTasks, deferredSearchTask{
				record: searchRecord, plainText: res.PlainText, localIdx: localIdx, cached: newSearch,
			})
		}
	}

	local.allPosts = append(local.allPosts, post)
	if post.Pinned {
		local.pinnedPosts = append(local.pinnedPosts, post)
	}
	for _, tag := range post.Tags {
		local.tagEntries = append(local.tagEntries, tagEntry{
			tag: strings.ToLower(strings.TrimSpace(tag)), post: post,
		})
	}

	// Stream to renderer // No locks needed for channel write
	wCtx.RenderChan <- renderTask{
		parseRes:    res,
		f:           f,
		htmlContent: htmlContent,
		destPath:    destPath,
		relPath:     relPath,
		htmlRelPath: htmlRelPath,
		source:      sourceBytes,
	}

	if !useCache && s.cache != nil {
		postID := cache.GeneratePostID("", relPath)
		newMeta := &models.PostMeta{
			PostID: postID, Path: relPath, ModTime: f.Info.ModTime().UnixNano(),
			ContentHash: res.FrontmatterHash, BodyHash: f.BodyHash, Title: post.Title, Date: post.DateObj,
			WordCount: int(f.Info.Size()), // Use WordCount as size for quick comparison
			Tags:      post.Tags, ReadingTime: post.ReadingTime, Description: post.Description,
			Link: post.Link, Pinned: post.Pinned, Weight: post.Weight, Draft: post.Draft,
			Meta: res.Metadata, TOC: res.TOC, SSRInputHashes: ssrHashes,
			CardHash: res.FrontmatterHash, HasImages: res.HasImages, MathExpressions: res.MathExpressions,
		}
		if err := s.cache.StoreHTMLForPost(newMeta, []byte(htmlContent)); err != nil {
			s.logger.Warn("Failed to store HTML for post", "path", relPath, "error", err)
		}
		local.newPostsMeta = append(local.newPostsMeta, newMeta)
		local.newDeps[postID] = &models.Dependencies{Tags: post.Tags}
	}
}

func (s *postService) mergeWorkerStates(locals []*workerLocalState, wCtx WorkerContext) {
	pc := wCtx.PC
	for _, local := range locals {
		if local.anyChanged {
			pc.anyPostChanged.Store(true)
		}
		baseIdx := len(pc.indexedPosts)
		pc.allPosts = append(pc.allPosts, local.allPosts...)
		pc.pinnedPosts = append(pc.pinnedPosts, local.pinnedPosts...)
		pc.indexedPosts = append(pc.indexedPosts, local.indexedPosts...)
		pc.newPostsMeta = append(pc.newPostsMeta, local.newPostsMeta...)
		for k, v := range local.newSearchRecs {
			pc.newSearchRecords[k] = v
		}
		for k, v := range local.newDeps {
			pc.newDeps[k] = v
		}
		for _, te := range local.tagEntries {
			pc.tagMap[te.tag] = append(pc.tagMap[te.tag], te.post)
		}
		pc.errs = append(pc.errs, local.errs...)

		for _, st := range local.searchTasks {
			globalIdx := baseIdx + st.localIdx
			wCtx.SearchPool.Submit(searchTask{
				record:    st.record,
				plainText: st.plainText,
				indexed:   &pc.indexedPosts[globalIdx],
				cached:    st.cached,
			})
		}
	}
}
