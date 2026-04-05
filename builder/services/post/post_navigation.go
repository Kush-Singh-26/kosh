package post

import (
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

type navInfo struct {
	allPosts []models.PostMetadata
	postPos  map[string]int
}

func (s *postService) prepareNavigationInfo(files []models.ScannedFile) navInfo {
	var allPosts []models.PostMetadata
	postPos := make(map[string]int)

	for _, f := range files {
		if f.Draft && !s.cfg.IncludeDrafts {
			continue
		}
		d, _ := time.Parse("2006-01-02", f.Date)
		if d.IsZero() {
			d = f.Info.ModTime()
		}
		post := models.PostMetadata{
			Title: f.Title, Link: f.Link, Weight: f.Weight,
			Pinned: f.Pinned, Draft: f.Draft,
			DateObj: d, Description: f.Description, Tags: f.Tags,
			ReadingTime: f.ReadingTime,
		}
		allPosts = append(allPosts, post)
	}

	timeutil.SortPosts(allPosts)
	for i, p := range allPosts {
		postPos[p.Link] = i
	}

	return navInfo{allPosts: allPosts, postPos: postPos}
}
