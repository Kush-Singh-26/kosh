package navigation

import "testing"

func TestGetVersionFromPath(t *testing.T) {
	tests := []struct {
		path        string
		wantVersion string
		wantRelPath string
	}{
		{"content/v2.0/getting-started.md", "v2.0", "getting-started.md"},
		{"content/posts/hello.md", "", "posts/hello.md"},
		{"content/v1.10/nested/doc.md", "v1.10", "nested/doc.md"},
		{"v2.0/direct.md", "v2.0", "direct.md"},
		{"content\\v3.0\\windows.md", "v3.0", "windows.md"},
		{"C:\\project\\content\\v4.0\\abs.md", "v4.0", "abs.md"},
	}

	for _, tt := range tests {
		gotVer, gotRel := GetVersionFromPath(tt.path)
		if gotVer != tt.wantVersion || gotRel != tt.wantRelPath {
			t.Errorf("GetVersionFromPath(%q) = (%q, %q), want (%q, %q)",
				tt.path, gotVer, gotRel, tt.wantVersion, tt.wantRelPath)
		}
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		baseURL string
		version string
		relPath string
		want    string
	}{
		{"https://example.com", "v2.0", "docs/intro.html", "https://example.com/v2.0/docs/intro.html"},
		{"https://example.com/", "", "index.html", "https://example.com/index.html"},
		{"http://localhost:8080", "v1.0", "/posts/post.html", "http://localhost:8080/v1.0/posts/post.html"},
	}

	for _, tt := range tests {
		got := BuildURL(tt.baseURL, tt.version, tt.relPath)
		if got != tt.want {
			t.Errorf("BuildURL(%q, %q, %q) = %q, want %q",
				tt.baseURL, tt.version, tt.relPath, got, tt.want)
		}
	}
}
