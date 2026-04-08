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
		{"content path", "content/post.md", false},
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
	cachedMeta := &cache.PostMeta{
		ModTime:  1000,
		BodyHash: "hash123",
	}

	info := afero.NewMemMapFs()
	_ = afero.WriteFile(info, "post.md", []byte("content"), 0644)
	stat, _ := info.Stat("post.md")
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
	posts := []models.IndexedPost{
		{SourcePath: "content/post1.md", Record: models.PostRecord{Link: "post1"}},
		{SourcePath: "content/post2.md", Record: models.PostRecord{Link: "post2"}},
		{SourcePath: "content/post1.md", Record: models.PostRecord{Link: "post1"}},
	}

	deduped := dedupeIndexedPosts(posts)
	if len(deduped) != 2 {
		t.Errorf("expected 2 deduped posts, got %d", len(deduped))
	}
}

func indexedPostStableKey(ip models.IndexedPost) string {
	if ip.SourcePath != "" {
		return fspkg.NormalizePath(ip.SourcePath)
	}
	return fspkg.NormalizePath(ip.Record.Link)
}

func dedupeIndexedPosts(posts []models.IndexedPost) []models.IndexedPost {
	if len(posts) < 2 {
		return posts
	}
	seen := make(map[string]int, len(posts))
	result := make([]models.IndexedPost, 0, len(posts))
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
