package post

import (
	"time"

	"github.com/Kush-Singh-26/kosh/builder/generators"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

type navInfo struct {
	allPosts []models.PostMetadata
	postPos  map[string]int
	allTags  []models.TagData
}

func (service *postService) prepareNavigationInfo(files []models.ScannedFile) navInfo {
	var allPosts []models.PostMetadata
	postPos := make(map[string]int)
	tagMap := make(map[string][]models.PostMetadata)

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
			DateObj: date, Description: file.Description, Tags: file.Tags,
			ReadingTime: file.ReadingTime,
		}
		allPosts = append(allPosts, post)

		for _, tag := range file.Tags {
			tagMap[tag] = append(tagMap[tag], post)
		}
	}

	timeutil.SortPosts(allPosts)
	for idx, post := range allPosts {
		postPos[post.Link] = idx
	}

	allTags := generators.BuildAllTags(tagMap)

	return navInfo{allPosts: allPosts, postPos: postPos, allTags: allTags}
}
