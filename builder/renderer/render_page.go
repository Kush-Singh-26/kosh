package renderer

import (
	"fmt"
	"io"

	fspkg "github.com/Kush-Singh-26/kosh/builder/fs"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/pools"
)

// executeTemplateAndWrite executes a template, processes HTML, optionally minifies, and writes via sink.
// This is a unified helper to avoid duplication across RenderPage, RenderIndex, RenderGraph, and Render404.
func (r *Renderer) executeTemplateAndWrite(path string, tmpl Executor, data models.PageData, templateName string) error {
	r.PreparePageData(&data)

	buf := pools.SharedBufferPool.Get()
	defer pools.SharedBufferPool.Put(buf)

	if err := tmpl.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute %s template for %s: %w", templateName, path, err)
	}

	finalBytes := buf.Bytes()

	// Optional Minification
	if r.Compress {
		minifier := r.Minifier
		if minifier == nil {
			minifier = fspkg.GetMinifier()
		}
		minified, err := minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	// Final Write
	if err := r.Sink.WriteStream(path, func(w io.Writer) error {
		_, err := w.Write(finalBytes)
		return err
	}); err != nil {
		r.recordError("Failed to write processed HTML", path, err)
		return fmt.Errorf("failed to write processed HTML for %s: %w", path, err)
	}

	r.RegisterFile(path)
	return nil
}

func (r *Renderer) RenderPage(path string, data models.PageData) error {
	r.mu.RLock()
	layout := r.Layout
	r.mu.RUnlock()

	if layout == nil {
		return fmt.Errorf("layout template not loaded for page %s", path)
	}

	return r.executeTemplateAndWrite(path, layout, data, "layout")
}
