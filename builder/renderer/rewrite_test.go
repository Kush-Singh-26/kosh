package renderer

import (
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/assets"
)

func TestRewriteImageRefs(t *testing.T) {
	tests := []struct {
		name      string
		html      string
		converted map[string]string
		expected  string
	}{
		{
			name:      "no converted images (keep original)",
			html:      `<img src="/static/images/test.png">`,
			converted: map[string]string{},
			expected:  `<img src="/static/images/test.png" loading="lazy" decoding="async">`,
		},
		{
			name:      "simple png rewrite",
			html:      `<img src="/static/images/Transformer1.png">`,
			converted: map[string]string{"/static/images/Transformer1.png": "/static/images/Transformer1.webp"},
			expected:  `<img src="/static/images/Transformer1.webp" loading="lazy" decoding="async">`,
		},
		{
			name:      "img with attributes before src",
			html:      `<img alt="test" src="/static/images/Transformer2.png" width="800">`,
			converted: map[string]string{"/static/images/Transformer2.png": "/static/images/Transformer2.webp"},
			expected:  `<img alt="test" src="/static/images/Transformer2.webp" width="800" loading="lazy" decoding="async">`,
		},
		{
			name:      "img with single quotes",
			html:      `<img src='/static/images/Transformer3.png'>`,
			converted: map[string]string{"/static/images/Transformer3.png": "/static/images/Transformer3.webp"},
			expected:  `<img src='/static/images/Transformer3.webp' loading="lazy" decoding="async">`,
		},
		{
			name:      "jpg rewrite",
			html:      `<img src="/static/images/photo.jpg">`,
			converted: map[string]string{"/static/images/photo.jpg": "/static/images/photo.webp"},
			expected:  `<img src="/static/images/photo.webp" loading="lazy" decoding="async">`,
		},
		{
			name:      "jpeg rewrite",
			html:      `<img src="/static/images/photo.jpeg">`,
			converted: map[string]string{"/static/images/photo.jpeg": "/static/images/photo.webp"},
			expected:  `<img src="/static/images/photo.webp" loading="lazy" decoding="async">`,
		},
		{
			name:      "case insensitive PNG",
			html:      `<img src="/static/images/Transformer4.PNG">`,
			converted: map[string]string{"/static/images/Transformer4.PNG": "/static/images/Transformer4.webp"},
			expected:  `<img src="/static/images/Transformer4.webp" loading="lazy" decoding="async">`,
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
			expected:  `<img src="/static/images/logo.webp" loading="lazy" decoding="async">`,
		},
		{
			name:      "no src attribute",
			html:      `<img alt="test" class="my-img">`,
			converted: map[string]string{"/static/images/test.png": "/static/images/test.webp"},
			expected:  `<img alt="test" class="my-img" loading="lazy" decoding="async">`,
		},
		{
			name:      "already has loading and decoding",
			html:      `<img src="/test.png" loading="eager" decoding="sync">`,
			converted: map[string]string{"/test.png": "/test.webp"},
			expected:  `<img src="/test.webp" loading="eager" decoding="sync">`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assets.ResetConvertedImages()
			for k, v := range tt.converted {
				assets.RecordConvertedImage(k, v)
			}
			result := rewriteImageRefs([]byte(tt.html), "test.html")
			if string(result) != tt.expected {
				t.Errorf("\ninput:    %q\ngot:      %q\nwant:     %q", tt.html, string(result), tt.expected)
			}
		})
	}
}

func TestRewriteImageRefs_Specific(t *testing.T) {
	// Minimal case: check if src attribute is found and rewritten
	html := `<img src="/test.png" alt="test">`
	assets.ResetConvertedImages()
	assets.RecordConvertedImage("/test.png", "/test.webp")
	result := rewriteImageRefs([]byte(html), "test.html")
	t.Logf("html=%q result=%q", html, string(result))
	if string(result) != `<img src="/test.webp" alt="test" loading="lazy" decoding="async">` {
		t.Errorf("got %q want %q", string(result), `<img src="/test.webp" alt="test" loading="lazy" decoding="async">`)
	}
}

func TestRewriteImgSrc(t *testing.T) {
	converted := map[string]string{"/test.png": "/test.webp"}
	src := "/test.png"
	got := rewriteImgSrc(src, converted)
	t.Logf("rewriteImgSrc(%q, %v) = %q", src, converted, got)
}
