package services

import (
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// GroupMetadataResult holds the categorized post metadata
type GroupMetadataResult struct {
	AllPosts       []models.PostMetadata
	PinnedPosts    []models.PostMetadata
	TagMap         map[string][]models.PostMetadata
	PostsByVersion map[string][]models.PostMetadata
}

// GroupMetadata categorizes posts into lists, pins, tags, and versions
func GroupMetadata(cfg *config.Config, allMetadataMap *sync.Map) *GroupMetadataResult {
	var (
		allPosts       []models.PostMetadata
		pinnedPosts    []models.PostMetadata
		tagMap         = make(map[string][]models.PostMetadata)
		postsByVersion = make(map[string][]models.PostMetadata)
	)

	type tagEntry struct {
		tag  string
		post models.PostMetadata
	}
	var tagEntries []tagEntry

	allMetadataMap.Range(func(key, value interface{}) bool {
		p := value.(models.PostMetadata)
		postsByVersion[p.Version] = append(postsByVersion[p.Version], p)

		for _, t := range p.Tags {
			key := strings.ToLower(strings.TrimSpace(t))
			tagEntries = append(tagEntries, tagEntry{key, p})
		}

		// Determine if this post belongs to the main feed:
		// - Unversioned posts for non-versioned sites
		// - Latest version posts for versioned sites
		isLatestOrUnversioned := p.Version == ""
		if len(cfg.Versions) > 0 {
			for _, v := range cfg.Versions {
				if v.IsLatest && p.Version == v.Name {
					isLatestOrUnversioned = true
					break
				}
			}
		}

		if isLatestOrUnversioned {
			if p.Pinned {
				pinnedPosts = append(pinnedPosts, p)
			} else {
				allPosts = append(allPosts, p)
			}
		}
		return true
	})

	for _, e := range tagEntries {
		tagMap[e.tag] = append(tagMap[e.tag], e.post)
	}

	return &GroupMetadataResult{
		AllPosts:       allPosts,
		PinnedPosts:    pinnedPosts,
		TagMap:         tagMap,
		PostsByVersion: postsByVersion,
	}
}

// BuildSiteTrees generates navigation trees for each version from their lists of posts
func BuildSiteTrees(postsByVersion map[string][]models.PostMetadata) map[string][]*models.TreeNode {
	siteTrees := make(map[string][]*models.TreeNode)
	for ver, posts := range postsByVersion {
		utils.SortPosts(posts)
		siteTrees[ver] = utils.BuildSiteTree(posts, "")
	}
	return siteTrees
}
