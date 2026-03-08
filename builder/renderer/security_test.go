package renderer

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestRenderer_XSSProtection(t *testing.T) {
	// We don't need a full renderer for this, just test how html/template handles our PageData
	tmplStr := `
<html>
<head>
<title>{{ .Title }}</title>
<meta name="description" content="{{ .Description }}">
</head>
<body>
	<div class="content">{{ .Content }}</div>
	<div class="meta">{{ .Meta.author }}</div>
</body>
</html>
`
	tmpl, err := template.New("test").Parse(tmplStr)
	if err != nil {
		t.Fatalf("Failed to parse template: %v", err)
	}

	data := models.PageData{
		Title:       "<script>alert('title')</script>",
		Description: "\"> <script>alert('desc')</script>",
		Content:     template.HTML("<div onclick=\"alert('content')\">Safe HTML</div>"),
		Meta: map[string]interface{}{
			"author": "<b onmouseover=\"alert('author')\">Evil Author</b>",
		},
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("Failed to execute template: %v", err)
	}

	got := buf.String()
	t.Logf("DEBUG HTML: %s", got)

	// Title should be escaped
	if strings.Contains(got, "<script>") {
		t.Error("Title was not escaped!")
	}
	if !strings.Contains(got, "&lt;script&gt;") {
		t.Error("Title start tag not escaped")
	}

	// Description should be escaped
	if strings.Contains(got, "\">") {
		// If it's escaped it should be &#34;&gt; or similar
		t.Log("Checking description escape...")
	}
	if strings.Contains(got, "content=\"\">") {
		t.Error("Description attribute injection successful!")
	}

	// Content should NOT be escaped (it's template.HTML)
	if !strings.Contains(got, "<div onclick=\"alert('content')\">Safe HTML</div>") {
		t.Error("Content was escaped but shouldn't be (as it is template.HTML)")
	}

	// Meta map values should be escaped
	if strings.Contains(got, "Evil Author</b>") {
		t.Error("Meta value tag not escaped")
	}
	if !strings.Contains(got, "Evil Author&lt;/b&gt;") {
		t.Error("Meta value tag end not escaped correctly")
	}
}
