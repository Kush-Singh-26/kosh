package renderer

import (
	"html/template"
	"strings"
	"testing"
	"time"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestRenderer_TimezoneHandling(t *testing.T) {
	r := setupTestRenderer(t)

	now := time.Now()
	data := models.PageData{
		Title: "Timezone Test",
		Meta: map[string]any{
			"date": now,
		},
		Content: template.HTML("<p>Content</p>"),
	}

	_ = r.RenderPage("timezone.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Timezone handling should not produce errors, got %v", errs)
	}
}

func TestRenderer_SpecialCharacters(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:       "Test <>&\"'",
		Description: "Special chars: <script>alert('xss')</script>",
		Content:     template.HTML("<p>Content with &amp; special chars</p>"),
		Meta: map[string]any{
			"author": "Author <author@example.com>",
		},
	}

	_ = r.RenderPage("special.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Special characters should not produce errors, got %v", errs)
	}
}

func TestRenderer_EmptyContent(t *testing.T) {
	r := setupTestRenderer(t)

	data := models.PageData{
		Title:   "Empty Content",
		Content: template.HTML(""),
	}

	_ = r.RenderPage("empty.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Empty content should not produce errors, got %v", errs)
	}
}

func TestRenderer_VeryLongContent(t *testing.T) {
	r := setupTestRenderer(t)

	longContent := strings.Repeat("<p>This is a paragraph.</p>", 1000)

	data := models.PageData{
		Title:   "Long Content",
		Content: template.HTML(longContent),
	}

	_ = r.RenderPage("long.html", data)

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Long content should not produce errors, got %v", errs)
	}
}
