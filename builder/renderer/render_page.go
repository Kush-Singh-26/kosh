package renderer

import (
	"bytes"
	"io"
	"path/filepath"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/utils"
)

func (r *Renderer) RenderPage(path string, data models.PageData) {
	if data.Assets == nil {
		data.Assets = r.GetAssets()
	}

	if err := r.Sink.MkdirAll(filepath.Dir(path)); err != nil {
		r.logger.Error("Failed to create directory", "path", path, "error", err)
		return
	}

	r.mu.RLock()
	layout := r.Layout
	r.mu.RUnlock()

	if layout == nil {
		r.logger.Error("Layout template not loaded", "path", path)
		return
	}

	// 1. Execute template to buffer
	var buf bytes.Buffer
	if err := layout.Execute(&buf, data); err != nil {
		r.logger.Error("Failed to render layout", "path", path, "error", err)
		return
	}

	// 2. Process HTML (Fix images and internal paths)
	processedHTML := utils.ProcessHTML(buf.String(), data.BaseURL, data.RelativePrefix, r.Compress)

	// 3. Optional Minification
	finalBytes := []byte(processedHTML)
	if r.Compress {
		minified, err := utils.Minifier.Bytes("text/html", finalBytes)
		if err == nil {
			finalBytes = minified
		}
	}

	// 4. Final Write
	err := r.Sink.WriteStream(path, func(w io.Writer) error {
		_, err := w.Write(finalBytes)
		return err
	})

	if err != nil {
		r.logger.Error("Failed to write processed HTML", "path", path, "error", err)
		r.recordError("Failed to write processed HTML", path, err)
	} else {
		r.RegisterFile(path)
	}
}
