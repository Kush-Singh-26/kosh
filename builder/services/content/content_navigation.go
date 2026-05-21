package content

import (
	"time"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/navigation"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

type navInfo struct {
	allItems       []models.ContentMetadata
	postPos        map[string]int
	taxonomies     map[string]models.TaxonomyData
	navigationTree *models.NodeTree
}

func (service *contentService) prepareNavigationInfo(files []models.ScannedResource) navInfo {
	var allItems []models.ContentMetadata
	taxonomyMap := make(map[string]map[string][]models.ContentMetadata)

	for _, file := range files {
		if file.IsDraft && !service.cfg.ShouldIncludeDrafts {
			continue
		}

		item := service.createContentMetadata(file)
		allItems = append(allItems, item)
		service.aggregateTaxonomies(taxonomyMap, item)
	}

	timeutil.SortItems(allItems)
	navigationTree := navigation.BuildNavigationTree(allItems)

	// Derive sequential navigation from the hierarchical tree to ensure
	// Next/Prev matches the Sidebar order perfectly.
	sequentialResources := navigation.FlattenTree(navigationTree)
	sequentialItems := make([]models.ContentMetadata, len(sequentialResources))
	for i, r := range sequentialResources {
		sequentialItems[i] = models.ContentMetadata{
			Title: r.Title, Link: r.Link, Path: r.RelPath, Weight: r.Weight,
			IsPinned: r.IsPinned, DateObj: r.Date, Description: r.Description,
			ReadingTime: r.ReadingTime,
		}
	}

	postPos := buildPostPositionMap(sequentialItems)
	taxonomies := service.buildTaxonomyData(taxonomyMap)

	return navInfo{allItems: sequentialItems, postPos: postPos, taxonomies: taxonomies, navigationTree: navigationTree}
}

func (service *contentService) createContentMetadata(file models.ScannedResource) models.ContentMetadata {
	date, _ := time.Parse("2006-01-02", file.Date)
	if date.IsZero() && file.Info != nil {
		date = file.Info.ModTime()
	}

	item := models.ContentMetadata{
		Title: file.Title, Link: file.Link, Weight: file.Weight,
		IsPinned: file.IsPinned, IsDraft: file.IsDraft,
		DateObj: date, Description: file.Description,
		ReadingTime: file.ReadingTime, Path: file.RelPath,
		Taxonomies: make(map[string][]string),
	}

	if file.PreParsedMeta != nil {
		for taxKey := range service.cfg.Taxonomies {
			if val, ok := file.PreParsedMeta[taxKey]; ok {
				item.Taxonomies[taxKey] = extractTerms(val)
			}
		}
	}

	for taxKey := range service.cfg.Taxonomies {
		if len(item.Taxonomies[taxKey]) == 0 && len(file.Taxonomies[taxKey]) > 0 {
			item.Taxonomies[taxKey] = file.Taxonomies[taxKey]
		}
	}

	return item
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

func (service *contentService) aggregateTaxonomies(taxonomyMap map[string]map[string][]models.ContentMetadata, item models.ContentMetadata) {
	for taxKey, terms := range item.Taxonomies {
		if taxonomyMap[taxKey] == nil {
			taxonomyMap[taxKey] = make(map[string][]models.ContentMetadata)
		}
		for _, term := range terms {
			taxonomyMap[taxKey][term] = append(taxonomyMap[taxKey][term], item)
		}
	}
}

func buildPostPositionMap(posts []models.ContentMetadata) map[string]int {
	postPos := make(map[string]int, len(posts))
	for idx, item := range posts {
		postPos[item.Link] = idx
	}
	return postPos
}

func (service *contentService) buildTaxonomyData(taxonomyMap map[string]map[string][]models.ContentMetadata) map[string]models.TaxonomyData {
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
