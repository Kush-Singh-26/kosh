package renderer

import (
	"fmt"
	"io"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

// executeTemplateAndWrite executes a template, processes HTML, optionally minifies, and writes via sink.
// This is a unified helper to avoid duplication across RenderPage, RenderIndex, RenderGraph, and Render404.
func (r *Renderer) executeTemplateAndWrite(path string, tmpl Executor, data models.PageData, templateName string) error {
	r.PreparePageData(&data)

	buf := utils.SharedBufferPool.Get()
	defer utils.SharedBufferPool.Put(buf)

	if err := tmpl.Execute(buf, data); err != nil {
		return fmt.Errorf("failed to execute %s template for %s: %w", templateName, path, err)
	}

	// Process HTML (Fix images and internal paths) - Legacy regex post-pass
	var finalBytes []byte
	if r.EnableLegacyProcessHTML {
		finalBytes = utils.ProcessHTMLBytes(buf.Bytes(), data.BaseURL, data.RelativePrefix, r.Compress)
	} else {
		finalBytes = buf.Bytes()
	}

	// Optional Minification
	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
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
