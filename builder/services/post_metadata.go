package services

import (
	"strings"
	"sync"

	"github.com/Kush-Singh-26/kosh/builder/config"
	"github.com/Kush-Singh-26/kosh/builder/models"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
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

	// Pre-calculate latest version name for O(1) lookup
	latestVerName := ""
	if len(cfg.Versions) > 0 {
		for _, v := range cfg.Versions {
			if v.IsLatest {
				latestVerName = v.Name
				break
			}
		}
	}

	allMetadataMap.Range(func(key, value any) bool {
		p := value.(models.PostMetadata)
		postsByVersion[p.Version] = append(postsByVersion[p.Version], p)

		for _, t := range p.Tags {
			key := strings.ToLower(strings.TrimSpace(t))
			tagMap[key] = append(tagMap[key], p)
		}

		// Determine if this post belongs to the main feed:
		// - Unversioned posts for non-versioned sites
		// - Latest version posts for versioned sites
		isLatestOrUnversioned := (p.Version == "" && latestVerName == "") || (p.Version == latestVerName && latestVerName != "")

		if isLatestOrUnversioned {
			if p.Pinned {
				pinnedPosts = append(pinnedPosts, p)
			} else {
				allPosts = append(allPosts, p)
			}
		}
		return true
	})

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
		timeutil.SortPosts(posts)
		siteTrees[ver] = fspkg.BuildSiteTree(posts, "")
	}
	return siteTrees
}
