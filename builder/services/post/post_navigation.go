package post

import (
	"time"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

type navInfo struct {
	allPosts   []models.PostMetadata
	postPos    map[string]int
	taxonomies map[string]models.TaxonomyData
}

func (service *postService) prepareNavigationInfo(files []models.ScannedResource) navInfo {
	var allPosts []models.PostMetadata
	taxonomyMap := make(map[string]map[string][]models.PostMetadata)

	for _, file := range files {
		if file.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}

		post := service.createPostMetadata(file)
		allPosts = append(allPosts, post)
		service.aggregateTaxonomies(taxonomyMap, post)
	}

	timeutil.SortPosts(allPosts)
	postPos := buildPostPositionMap(allPosts)
	taxonomies := service.buildTaxonomyData(taxonomyMap)

	return navInfo{allPosts: allPosts, postPos: postPos, taxonomies: taxonomies}
}

func (service *postService) createPostMetadata(file models.ScannedResource) models.PostMetadata {
	date, _ := time.Parse("2006-01-02", file.Date)
	if date.IsZero() && file.Info != nil {
		date = file.Info.ModTime()
	}

	post := models.PostMetadata{
		Title: file.Title, Link: file.Link, Weight: file.Weight,
		IsPinned: file.IsPinned, IsDraft: file.IsDraft,
		DateObj: date, Description: file.Description,
		ReadingTime: file.ReadingTime,
		Taxonomies:  make(map[string][]string),
	}

	if file.PreParsedMeta != nil {
		for taxKey := range service.cfg.Taxonomies {
			if val, ok := file.PreParsedMeta[taxKey]; ok {
				post.Taxonomies[taxKey] = extractTerms(val)
			}
		}
	}

	if len(post.Taxonomies["tags"]) == 0 && len(file.Taxonomies["tags"]) > 0 {
		post.Taxonomies["tags"] = file.Taxonomies["tags"]
	}

	return post
}

func extractTerms(val any) []string {
	switch v := val.(type) {
	case string:
		return []string{v}
	case []any:
		var terms []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				terms = append(terms, s)
			}
		}
		return terms
	case []string:
		return v
	default:
		return nil
	}
}

func (service *postService) aggregateTaxonomies(taxonomyMap map[string]map[string][]models.PostMetadata, post models.PostMetadata) {
	for taxKey, terms := range post.Taxonomies {
		if taxonomyMap[taxKey] == nil {
			taxonomyMap[taxKey] = make(map[string][]models.PostMetadata)
		}
		for _, term := range terms {
			taxonomyMap[taxKey][term] = append(taxonomyMap[taxKey][term], post)
		}
	}
}

func buildPostPositionMap(posts []models.PostMetadata) map[string]int {
	postPos := make(map[string]int, len(posts))
	for idx, post := range posts {
		postPos[post.Link] = idx
	}
	return postPos
}

func (service *postService) buildTaxonomyData(taxonomyMap map[string]map[string][]models.PostMetadata) map[string]models.TaxonomyData {
	taxonomies := make(map[string]models.TaxonomyData)
	for taxKey, plural := range service.cfg.Taxonomies {
		if termMap, ok := taxonomyMap[taxKey]; ok {
			taxonomies[taxKey] = models.TaxonomyData{
				Name:   taxKey,
				Plural: plural,
				Terms:  generators.BuildTaxonomyData(service.cfg.ContentPrefix, plural, termMap),
			}
		}
	}
	return taxonomies
}
