package renderer

import (
	"html/template"
	"testing"

	"github.com/Kush-Singh-26/kosh/builder/models"
)

func TestRenderer_ConcurrentRenderPage(t *testing.T) {
	r := setupTestRenderer(t)

	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			data := models.PageData{
				Title:   "Concurrent Test",
				Content: template.HTML("<p>Content</p>"),
			}
			path := "concurrent/" + string(rune('a'+id)) + ".html"
			_ = r.RenderPage(path, data)
			done <- true
		}(i)
	}

	for range numGoroutines {
		<-done
	}

	count := len(r.GetRenderedFiles())
	if count != numGoroutines {
		t.Errorf("Concurrent RenderPage should render %d files, got %d", numGoroutines, count)
	}
}

func TestRenderer_ConcurrentRenderIndex(t *testing.T) {
	r := setupTestRenderer(t)

	const numGoroutines = 5
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			data := models.PageData{
				Title: "Index " + string(rune('a'+id)),
				Posts: []models.PostMetadata{{Title: "Post"}},
			}
			_ = r.RenderIndex("index_"+string(rune('a'+id))+".html", data)
			done <- true
		}(i)
	}

	for range numGoroutines {
		<-done
	}

	count := len(r.GetRenderedFiles())
	if count != numGoroutines {
		t.Errorf("Concurrent RenderIndex should render %d files, got %d", numGoroutines, count)
	}
}

func TestRenderer_BufferPoolReuse(t *testing.T) {
	r := setupTestRenderer(t)

	for i := 0; i < 5; i++ {
		data := models.PageData{
			Title:   "Buffer Test",
			Content: template.HTML("<p>Content " + string(rune('0'+i)) + "</p>"),
		}
		_ = r.RenderPage("buffer"+string(rune('0'+i))+".html", data)
	}

	if errs := r.ConsumeErrors(); errs != nil {
		t.Errorf("Buffer pool reuse should not produce errors, got %v", errs)
	}
}
