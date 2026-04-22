package content

import (
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
)

func (s *contentService) checkCache(relPath string, f models.ScannedResource, shouldForce bool, forceRerender bool) (*models.ContentMeta, bool) {
	if s.cache == nil || shouldForce {
		return nil, false
	}
	cachedMeta, err := s.cache.GetItemByPath(relPath)
	if err != nil || cachedMeta == nil {
		return nil, false
	}
	// Check both mod time and size for fast bail. ModTime now uses UnixNano for precision.
	fastBail := cachedMeta.ModTime == f.Info.ModTime().UnixNano() && cachedMeta.FileSize == int(f.Info.Size())

	if forceRerender {
		return cachedMeta, fastBail
	}
	return cachedMeta, fastBail
}

func (s *contentService) loadFromCache(cachedMeta *models.ContentMeta) (*ParsedMarkdownResult, string, bool) {
	cachedHTML, err := s.cache.GetHTMLContent(cachedMeta)
	if err != nil || cachedHTML == nil {
		return nil, "", false
	}
	cachedSearch, err := s.cache.GetSearchRecord(cachedMeta.ContentID)
	if err != nil || cachedSearch == nil {
		return nil, "", false
	}

	res := &ParsedMarkdownResult{
		Metadata: cachedMeta.Meta, TOC: cachedMeta.TOC,
		FrontmatterHash: cachedMeta.ContentHash, SSRHashes: cachedMeta.SSRInputHashes,
		HasImages: cachedMeta.HasImages, MathExpressions: cachedMeta.MathExpressions,
		SearchRecord: searchpkg.ContentRecord{
			Title:          cachedSearch.Title,
			Link:           cachedMeta.Link,
			Content:        cachedSearch.Content,
			Taxonomies:     cachedSearch.Taxonomies,
			NormalizedTaxs: cachedSearch.NormalizedTaxs,
		},
		WordFreqs: cachedSearch.WordFreqs, DocLen: cachedSearch.DocLen,
		StemMap: cachedSearch.StemMap, PositionalIndex: cachedSearch.PositionalIndex,
		Item: models.ContentMetadata{
			Title: cachedMeta.Title, Link: cachedMeta.Link, Description: cachedMeta.Description,
			Taxonomies: cachedMeta.Taxonomies, IsPinned: cachedMeta.IsPinned, Weight: cachedMeta.Weight,
			ReadingTime: cachedMeta.ReadingTime, DateObj: cachedMeta.Date,
			IsDraft: cachedMeta.IsDraft,
		},
	}
	return res, string(cachedHTML), true
}
