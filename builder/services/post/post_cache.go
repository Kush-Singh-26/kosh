package post

import (
	"github.com/zeebo/xxh3"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func (s *postService) checkCache(relPath string, f models.ScannedFile, shouldForce bool) (*models.PostMeta, bool) {
	if s.cache == nil || shouldForce {
		return nil, false
	}
	cachedMeta, err := s.cache.GetPostByPath(relPath)
	if err != nil || cachedMeta == nil {
		return nil, false
	}
	fastBail := cachedMeta.BodyHash == f.BodyHash && cachedMeta.ContentHash == f.FrontmatterHash
	return cachedMeta, fastBail
}

func (s *postService) loadFromCache(cachedMeta *models.PostMeta, htmlRelPath string) (*ParsedMarkdownResult, string, bool) {
	cachedHTML, err := s.cache.GetHTMLContent(cachedMeta)
	if err != nil || cachedHTML == nil {
		return nil, "", false
	}
	cachedSearch, err := s.cache.GetSearchRecord(cachedMeta.PostID)
	if err != nil || cachedSearch == nil {
		return nil, "", false
	}

	res := &ParsedMarkdownResult{
		Metadata: cachedMeta.Meta, TOC: cachedMeta.TOC,
		FrontmatterHash: cachedMeta.ContentHash, SSRHashes: cachedMeta.SSRInputHashes,
		HasImages: cachedMeta.HasImages, MathExpressions: cachedMeta.MathExpressions,
		SearchRecord: models.PostRecord{
			ID:    xxh3.HashString(cachedMeta.Link),
			Title: cachedSearch.Title, NormalizedTitle: cachedSearch.NormalizedTitle,
			Link: htmlRelPath, Content: cachedSearch.Content,
			NormalizedTags: cachedSearch.NormalizedTags,
		},
		WordFreqs: cachedSearch.BM25Data, DocLen: cachedSearch.DocLen,
		StemMap: cachedSearch.StemMap, PositionalIndex: cachedSearch.PositionalIndex,
		ByteOffsets: cachedSearch.ByteOffsets,
		Post: models.PostMetadata{
			Title: cachedMeta.Title, Link: cachedMeta.Link, Description: cachedMeta.Description,
			Tags: cachedMeta.Tags, Pinned: cachedMeta.Pinned, Weight: cachedMeta.Weight,
			ReadingTime: cachedMeta.ReadingTime, DateObj: cachedMeta.Date,
			Draft: cachedMeta.Draft,
		},
	}
	return res, string(cachedHTML), true
}
