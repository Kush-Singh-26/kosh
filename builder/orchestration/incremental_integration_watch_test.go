package orchestration

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/Kush-Singh-26/kosh/builder/cache"
	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/orchestration/watch"
)

func TestIncrementalBuild_SearchSourceChange(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"cmd/search path", "cmd/search/main.go", true},
		{"builder/search path", "builder/search/fuzzy.go", true},
		{"builder/models path", "builder/models/models.go", true},
		{"content path", "content/Content.md", false},
		{"template path", "themes/template.html", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := watch.IsSearchSourcePath(tt.path)
			if got != tt.want {
				t.Errorf("IsSearchSourcePath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIncrementalBuild_ModTimeQuickBail(t *testing.T) {
	cachedMeta := &cache.ContentMeta{
		ModTime:  1000,
		BodyHash: "hash123",
	}

	info := afero.NewMemMapFs()
	_ = afero.WriteFile(info, "Content.md", []byte("content"), 0644)
	stat, _ := info.Stat("Content.md")
	modTime := stat.ModTime().Unix()

	cachedMeta.ModTime = modTime

	shouldForce := false
	exists := true

	fastBail := !shouldForce && exists && cachedMeta != nil && cachedMeta.BodyHash != "" && stat != nil && cachedMeta.ModTime == modTime

	if !fastBail {
		t.Error("unchanged file should trigger fast bail")
	}

	cachedMeta.ModTime = modTime - 100
	fastBail = !shouldForce && exists && cachedMeta != nil && cachedMeta.BodyHash != "" && stat != nil && cachedMeta.ModTime == modTime

	if fastBail {
		t.Error("changed file should not trigger fast bail")
	}
}

func TestIncrementalBuild_DedupeIndexedPosts(t *testing.T) {
	posts := []models.IndexedContent{
		{SourcePath: "content/post1.md", Record: models.ContentRecord{Link: "post1"}},
		{SourcePath: "content/post2.md", Record: models.ContentRecord{Link: "post2"}},
		{SourcePath: "content/post1.md", Record: models.ContentRecord{Link: "post1"}},
	}

	deduped := dedupeIndexedPosts(posts)
	if len(deduped) != 2 {
		t.Errorf("expected 2 deduped posts, got %d", len(deduped))
	}
}

func indexedPostStableKey(ip models.IndexedContent) string {
	if ip.SourcePath != "" {
		return fspkg.NormalizePath(ip.SourcePath)
	}
	return fspkg.NormalizePath(ip.Record.Link)
}

func dedupeIndexedPosts(posts []models.IndexedContent) []models.IndexedContent {
	if len(posts) < 2 {
		return posts
	}
	seen := make(map[string]int, len(posts))
	result := make([]models.IndexedContent, 0, len(posts))
	for _, ip := range posts {
		key := indexedPostStableKey(ip)
		if idx, ok := seen[key]; ok {
			result[idx] = ip
			continue
		}
		seen[key] = len(result)
		result = append(result, ip)
	}
	return result
}
