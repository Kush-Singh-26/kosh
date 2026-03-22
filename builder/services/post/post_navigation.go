package post

import (
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils/timeutil"
)

type navInfo struct {
	postsByVersion   map[string][]models.PostMetadata
	postPosByVersion map[string]map[string]int
}

func (s *postService) prepareNavigationInfo(files []models.ScannedFile) navInfo {
	postsByVersion := make(map[string][]models.PostMetadata)
	postPosByVersion := make(map[string]map[string]int)

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
			Pinned: f.Pinned, Draft: f.Draft, Version: f.Version,
			DateObj: d, Description: f.Description, Tags: f.Tags,
			ReadingTime: f.ReadingTime,
		}
		postsByVersion[f.Version] = append(postsByVersion[f.Version], post)
	}

	for ver, posts := range postsByVersion {
		timeutil.SortPosts(posts)
		postPosByVersion[ver] = make(map[string]int)
		for i, p := range posts {
			postPosByVersion[ver][p.Link] = i
		}
	}

	return navInfo{postsByVersion: postsByVersion, postPosByVersion: postPosByVersion}
}
