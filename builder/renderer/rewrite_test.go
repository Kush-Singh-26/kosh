package renderer

import (
	"testing"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
)

func TestRewriteImageRefs(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		converted map[string]string
		expected  string
	}{
		{
			name:      "no converted images",
			html:      `<img src="/static/images/test.png">`,
			converted: map[string]string{},
			expected:  `<img src="/static/images/test.png">`,
		},
		{
			name:      "simple png rewrite",
			html:      `<img src="/static/images/Transformer1.png">`,
			converted: map[string]string{"/static/images/Transformer1.png": "/static/images/Transformer1.webp"},
			expected:  `<img src="/static/images/Transformer1.webp">`,
		},
		{
			name:      "img with attributes before src",
			html:      `<img alt="test" src="/static/images/Transformer2.png" width="800">`,
			converted: map[string]string{"/static/images/Transformer2.png": "/static/images/Transformer2.webp"},
			expected:  `<img alt="test" src="/static/images/Transformer2.webp" width="800">`,
		},
		{
			name:      "img with single quotes",
			html:      `<img src='/static/images/Transformer3.png'>`,
			converted: map[string]string{"/static/images/Transformer3.png": "/static/images/Transformer3.webp"},
			expected:  `<img src='/static/images/Transformer3.webp'>`,
		},
		{
			name:      "jpg rewrite",
			html:      `<img src="/static/images/photo.jpg">`,
			converted: map[string]string{"/static/images/photo.jpg": "/static/images/photo.webp"},
			expected:  `<img src="/static/images/photo.webp">`,
		},
		{
			name:      "jpeg rewrite",
			html:      `<img src="/static/images/photo.jpeg">`,
			converted: map[string]string{"/static/images/photo.jpeg": "/static/images/photo.webp"},
			expected:  `<img src="/static/images/photo.webp">`,
		},
		{
			name:      "case insensitive PNG",
			html:      `<img src="/static/images/Transformer4.PNG">`,
			converted: map[string]string{"/static/images/Transformer4.PNG": "/static/images/Transformer4.webp"},
			expected:  `<img src="/static/images/Transformer4.webp">`,
		},
		{
			name:      "no img tags",
			html:      `<div>Hello</div>`,
			converted: map[string]string{"/static/images/test.png": "/static/images/test.webp"},
			expected:  `<div>Hello</div>`,
		},
		{
			name:      "webp not rewritten",
			html:      `<img src="/static/images/logo.webp">`,
			converted: map[string]string{},
			expected:  `<img src="/static/images/logo.webp">`,
		},
		{
			name:      "no src attribute",
			html:      `<img alt="test" class="my-img">`,
			converted: map[string]string{"/static/images/test.png": "/static/images/test.webp"},
			expected:  `<img alt="test" class="my-img">`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fspkg.ResetConvertedImages()
			for k, v := range tt.converted {
				fspkg.RecordConvertedImage(k, v)
			}
			result := rewriteImageRefs([]byte(tt.html))
			if string(result) != tt.expected {
				t.Errorf("\ninput:    %q\ngot:      %q\nwant:     %q", tt.html, string(result), tt.expected)
			}
		})
	}
}

func TestRewriteImageRefs_Specific(t *testing.T) {
	// Minimal case: check if src attribute is found and rewritten
	html := `<img src="/test.png">`
	converted := map[string]string{"/test.png": "/test.webp"}
	result := rewriteImageRefs([]byte(html))
	t.Logf("html=%q converted=%v result=%q", html, converted, string(result))
	if string(result) != `<img src="/test.webp">` {
		t.Errorf("got %q want %q", string(result), `<img src="/test.webp">`)
	}
}

func TestRewriteImgSrc(t *testing.T) {
	converted := map[string]string{"/test.png": "/test.webp"}
	src := "/test.png"
	got := rewriteImgSrc(src, converted)
	t.Logf("rewriteImgSrc(%q, %v) = %q", src, converted, got)
}
