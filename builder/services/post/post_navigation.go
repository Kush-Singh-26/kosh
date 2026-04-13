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
	postPos := make(map[string]int)
	taxonomyMap := make(map[string]map[string][]models.PostMetadata)

	for _, file := range files {
		if file.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}
		date, _ := time.Parse("2006-01-02", file.Date)
		if date.IsZero() {
			date = file.Info.ModTime()
		}
		post := models.PostMetadata{
			Title: file.Title, Link: file.Link, Weight: file.Weight,
			IsPinned: file.IsPinned, IsDraft: file.IsDraft,
			DateObj: date, Description: file.Description,
			ReadingTime: file.ReadingTime,
			Taxonomies:  make(map[string][]string),
		}

		// Pull taxonomies from metadata if available, fallback to file.Tags
		if file.PreParsedMeta != nil {
			for taxKey := range service.cfg.Taxonomies {
				if val, ok := file.PreParsedMeta[taxKey]; ok {
					switch v := val.(type) {
					case string:
						post.Taxonomies[taxKey] = []string{v}
					case []any:
						var terms []string
						for _, item := range v {
							if s, ok := item.(string); ok {
								terms = append(terms, s)
							}
						}
						post.Taxonomies[taxKey] = terms
					case []string:
						post.Taxonomies[taxKey] = v
					}
				}
			}
		}

		// Backward compatibility for file.Tags if taxonomy mapping didn't find anything
		if len(post.Taxonomies["tags"]) == 0 && len(file.Tags) > 0 {
			post.Taxonomies["tags"] = file.Tags
		}

		allPosts = append(allPosts, post)

		// Aggregate into taxonomyMap
		for taxKey, terms := range post.Taxonomies {
			if taxonomyMap[taxKey] == nil {
				taxonomyMap[taxKey] = make(map[string][]models.PostMetadata)
			}
			for _, term := range terms {
				taxonomyMap[taxKey][term] = append(taxonomyMap[taxKey][term], post)
			}
		}
	}

	timeutil.SortPosts(allPosts)
	for idx, post := range allPosts {
		postPos[post.Link] = idx
	}

	taxonomies := make(map[string]models.TaxonomyData)
	for taxKey, plural := range service.cfg.Taxonomies {
		if termMap, ok := taxonomyMap[taxKey]; ok {
			taxonomies[taxKey] = models.TaxonomyData{
				Name:   taxKey,
				Plural: plural,
				Terms:  generators.BuildTaxonomyData(plural, termMap),
			}
		}
	}

	return navInfo{allPosts: allPosts, postPos: postPos, taxonomies: taxonomies}
}
