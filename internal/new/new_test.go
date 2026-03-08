package new

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"Hello World", "hello-world"},
		{"Go: The Best", "go-the-best"},
		{"What? (Maybe)", "what-maybe"},
		{"Multiple---Hyphens", "multiple-hyphens"},
		{"-Trim-", "trim"},
	}

	for _, tt := range tests {
		got := sanitizeSlug(tt.title)
		if got != tt.want {
			t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.title, got, tt.want)
		}
	}
}

func TestRunFs(t *testing.T) {
	fs := afero.NewMemMapFs()
	_ = fs.MkdirAll("content", 0755)

	RunFs(fs, []string{"My New Post"})

	exists, _ := afero.Exists(fs, "content/my-new-post.md")
	if !exists {
		t.Error("New post file was not created")
	}

	content, _ := afero.ReadFile(fs, "content/my-new-post.md")
	if !strings.Contains(string(content), `title: "My New Post"`) {
		t.Error("Post content does not contain title")
	}
}
